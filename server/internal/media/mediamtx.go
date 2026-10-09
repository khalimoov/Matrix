// Package media controls the MediaMTX media server through its HTTP API.
//
// MediaMTX pulls RTSP streams from cameras, records them and serves them to
// browsers over WebRTC and HLS. Matrix configures one MediaMTX path per stream.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"matrix/internal/store"
)

type Client struct {
	api      string
	playback string
	http     *http.Client
}

func New(apiURL, playbackURL string) *Client {
	return &Client{
		api:      strings.TrimRight(apiURL, "/"),
		playback: strings.TrimRight(playbackURL, "/"),
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// pathConf is the subset of MediaMTX path settings Matrix manages.
type pathConf struct {
	Source            string `json:"source"`
	SourceOnDemand    bool   `json:"sourceOnDemand"`
	RTSPTransport     string `json:"rtspTransport"`
	Record            bool   `json:"record"`
	RecordDeleteAfter string `json:"recordDeleteAfter,omitempty"`
}

// SyncCamera makes MediaMTX match the camera's settings.
func (c *Client) SyncCamera(ctx context.Context, cam store.Camera) error {
	if !cam.Enabled {
		return c.RemoveCamera(ctx, cam)
	}
	mainURL, subURL, err := cam.StreamURLs()
	if err != nil {
		return err
	}
	// The main stream is pulled all the time so it can be recorded and the
	// camera's status is known; the sub stream is only pulled while watched.
	main := pathConf{
		Source:            mainURL,
		SourceOnDemand:    false,
		RTSPTransport:     "tcp",
		Record:            cam.Record,
		RecordDeleteAfter: strconv.Itoa(cam.RetentionDays*24) + "h",
	}
	if err := c.putPath(ctx, cam.PathName(), main); err != nil {
		return err
	}
	if subURL == "" {
		return c.deletePath(ctx, cam.SubPathName())
	}
	sub := pathConf{Source: subURL, SourceOnDemand: true, RTSPTransport: "tcp"}
	return c.putPath(ctx, cam.SubPathName(), sub)
}

func (c *Client) RemoveCamera(ctx context.Context, cam store.Camera) error {
	if err := c.deletePath(ctx, cam.PathName()); err != nil {
		return err
	}
	return c.deletePath(ctx, cam.SubPathName())
}

// ConfiguredPaths lists the names of paths configured in MediaMTX.
func (c *Client) ConfiguredPaths(ctx context.Context) ([]string, error) {
	var res struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/v3/config/paths/list?itemsPerPage=10000", nil, &res); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		names = append(names, it.Name)
	}
	return names, nil
}

type PathState struct {
	Online       bool     `json:"online"`
	Readers      int      `json:"readers"`
	InboundBytes uint64   `json:"inbound_bytes"`
	Codecs       []string `json:"codecs"`
}

// PathStates returns the live state of every active path, keyed by name.
func (c *Client) PathStates(ctx context.Context) (map[string]PathState, error) {
	var res struct {
		Items []struct {
			Name          string            `json:"name"`
			Available     bool              `json:"available"` // stream is being received
			Ready         bool              `json:"ready"`     // same, older MediaMTX versions
			Readers       []json.RawMessage `json:"readers"`
			InboundBytes  uint64            `json:"inboundBytes"`
			BytesReceived uint64            `json:"bytesReceived"`
			Tracks        []string          `json:"tracks"`
		} `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/v3/paths/list?itemsPerPage=10000", nil, &res); err != nil {
		return nil, err
	}
	states := make(map[string]PathState, len(res.Items))
	for _, it := range res.Items {
		st := PathState{
			Online:       it.Available || it.Ready,
			Readers:      len(it.Readers),
			InboundBytes: max(it.InboundBytes, it.BytesReceived),
			Codecs:       it.Tracks,
		}
		states[it.Name] = st
	}
	return states, nil
}

type Recording struct {
	Start    time.Time `json:"start"`
	Duration float64   `json:"duration"` // seconds
}

// Recordings lists recorded time spans of a path between start and end.
func (c *Client) Recordings(ctx context.Context, path string, start, end time.Time) ([]Recording, error) {
	q := url.Values{
		"path":  {path},
		"start": {start.Format(time.RFC3339)},
		"end":   {end.Format(time.RFC3339)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.playback+"/list?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return []Recording{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		err := apiError(resp)
		// The recordings folder of a path is created with its first segment.
		if strings.Contains(err.Error(), "no such file or directory") {
			return []Recording{}, nil
		}
		return nil, err
	}
	recs := []Recording{}
	return recs, json.NewDecoder(resp.Body).Decode(&recs)
}

// putPath creates or replaces a path. An unchanged path is left alone so its
// stream and recording are not interrupted.
func (c *Client) putPath(ctx context.Context, name string, conf pathConf) error {
	var current pathConf
	err := c.do(ctx, http.MethodGet, "/v3/config/paths/get/"+name, nil, &current)
	switch {
	case err == nil && current.matches(conf):
		return nil
	case err == nil:
		return c.do(ctx, http.MethodPost, "/v3/config/paths/replace/"+name, conf, nil)
	case isNotFound(err):
		return c.do(ctx, http.MethodPost, "/v3/config/paths/add/"+name, conf, nil)
	default:
		return err
	}
}

// matches reports whether the path already has the wanted settings.
// Durations are compared by value since MediaMTX may print "168h" as "7d".
func (p pathConf) matches(want pathConf) bool {
	return p.Source == want.Source && p.SourceOnDemand == want.SourceOnDemand &&
		p.RTSPTransport == want.RTSPTransport && p.Record == want.Record &&
		(want.RecordDeleteAfter == "" || parseDuration(p.RecordDeleteAfter) == parseDuration(want.RecordDeleteAfter))
}

func parseDuration(s string) time.Duration {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, _ := strconv.Atoi(days)
		return time.Duration(n) * 24 * time.Hour
	}
	d, _ := time.ParseDuration(s)
	return d
}

func (c *Client) deletePath(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/v3/config/paths/delete/"+name, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return fmt.Sprintf("mediamtx: %d %s", e.code, e.msg) }

func isNotFound(err error) bool {
	se, ok := err.(*statusError)
	return ok && se.code == http.StatusNotFound
}

func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Error string `json:"error"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		msg = e.Error
	}
	return &statusError{code: resp.StatusCode, msg: msg}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.api+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return apiError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
