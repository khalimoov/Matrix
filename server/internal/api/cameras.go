package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"matrix/internal/media"
	"matrix/internal/onvif"
	"matrix/internal/store"
)

// cameraView is a camera as the web client sees it: without the password,
// with its live state and the media paths to play.
type cameraView struct {
	store.Camera
	HasPassword bool             `json:"has_password"`
	Status      *media.PathState `json:"status"`
	LivePath    string           `json:"live_path"` // low-resolution stream for grids
	MainPath    string           `json:"main_path"` // full-resolution stream
	PTZ         bool             `json:"ptz"`       // PTZ control is configured
	SyncError   string           `json:"sync_error,omitempty"`
}

func (s *Server) view(c store.Camera) cameraView {
	v := cameraView{
		Camera:      c,
		HasPassword: c.Password != "",
		LivePath:    c.PathName(),
		MainPath:    c.PathName(),
		PTZ:         c.ONVIFURL != "",
	}
	if _, sub, err := c.StreamURLs(); err == nil && sub != "" {
		v.LivePath = c.SubPathName()
	}
	if st, ok := s.monitor.Status(c.ID); ok {
		v.Status = &st
	}
	return v
}

// cameraInput is what the client sends to create or update a camera.
// A nil password keeps the stored one.
type cameraInput struct {
	Name          string       `json:"name"`
	Group         string       `json:"group"`
	Vendor        store.Vendor `json:"vendor"`
	Host          string       `json:"host"`
	RTSPPort      int          `json:"rtsp_port"`
	Username      string       `json:"username"`
	Password      *string      `json:"password"`
	Channel       int          `json:"channel"`
	MainURL       string       `json:"main_url"`
	SubURL        string       `json:"sub_url"`
	ONVIFURL      string       `json:"onvif_url"`
	ONVIFProfile  string       `json:"onvif_profile"`
	Record        bool         `json:"record"`
	RetentionDays int          `json:"retention_days"`
	Enabled       bool         `json:"enabled"`
}

func (in cameraInput) apply(c *store.Camera) {
	c.Name, c.Group, c.Vendor = in.Name, in.Group, in.Vendor
	c.Host, c.RTSPPort, c.Username, c.Channel = in.Host, in.RTSPPort, in.Username, in.Channel
	if in.Password != nil {
		c.Password = *in.Password
	}
	c.MainURL, c.SubURL = in.MainURL, in.SubURL
	c.ONVIFURL, c.ONVIFProfile = in.ONVIFURL, in.ONVIFProfile
	c.Record, c.RetentionDays, c.Enabled = in.Record, in.RetentionDays, in.Enabled
}

func (s *Server) listCameras(w http.ResponseWriter, r *http.Request) {
	cams, err := s.store.ListCameras()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	views := make([]cameraView, len(cams))
	for i, c := range cams {
		views[i] = s.view(c)
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) getCamera(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCamera(w, r)
	if ok {
		writeJSON(w, http.StatusOK, s.view(c))
	}
}

func (s *Server) createCamera(w http.ResponseWriter, r *http.Request) {
	var in cameraInput
	if !readJSON(w, r, &in) {
		return
	}
	var c store.Camera
	in.apply(&c)
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := s.store.CreateCamera(c)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	v := s.view(c)
	v.SyncError = s.syncCamera(r.Context(), c)
	s.monitor.Emit(store.EventCameraAdded, c.ID, "Пользователь "+currentUser(r).Username+" добавил камеру «"+c.Name+"»")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateCamera(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCamera(w, r)
	if !ok {
		return
	}
	var in cameraInput
	if !readJSON(w, r, &in) {
		return
	}
	in.apply(&c)
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateCamera(c); err != nil {
		writeStoreError(w, err)
		return
	}
	v := s.view(c)
	v.SyncError = s.syncCamera(r.Context(), c)
	s.monitor.Emit(store.EventCameraChanged, c.ID, "Пользователь "+currentUser(r).Username+" изменил камеру «"+c.Name+"»")
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteCamera(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCamera(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteCamera(c.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	if err := s.media.RemoveCamera(r.Context(), c); err != nil {
		slog.Warn("remove camera from media server", "camera", c.ID, "err", err)
	}
	s.monitor.Forget(c.ID)
	s.monitor.Emit(store.EventCameraDeleted, 0, "Пользователь "+currentUser(r).Username+" удалил камеру «"+c.Name+"»")
	w.WriteHeader(http.StatusNoContent)
}

// syncCamera pushes a camera to the media server. A failure is reported to the
// client but does not fail the request: the monitor re-syncs on reconnect.
func (s *Server) syncCamera(ctx context.Context, c store.Camera) string {
	if err := s.media.SyncCamera(ctx, c); err != nil {
		slog.Warn("sync camera", "camera", c.ID, "err", err)
		return err.Error()
	}
	return ""
}

func (s *Server) loadCamera(w http.ResponseWriter, r *http.Request) (store.Camera, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return store.Camera{}, false
	}
	c, err := s.store.Camera(id)
	if err != nil {
		writeStoreError(w, err)
		return store.Camera{}, false
	}
	return c, true
}

// recordings lists recorded spans of a camera. Query: start, end (RFC 3339),
// defaulting to the last 24 hours.
func (s *Server) recordings(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCamera(w, r)
	if !ok {
		return
	}
	end := time.Now()
	start := end.Add(-24 * time.Hour)
	var err error
	if v := r.URL.Query().Get("start"); v != "" {
		if start, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid start time")
			return
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if end, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid end time")
			return
		}
	}
	recs, err := s.media.Recordings(r.Context(), c.PathName(), start, end)
	if err != nil {
		writeError(w, http.StatusBadGateway, "медиасервер: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": c.PathName(), "recordings": recs})
}

// --- PTZ ---

func (s *Server) ptzClient(w http.ResponseWriter, r *http.Request) (*onvif.Client, string, bool) {
	c, ok := s.loadCamera(w, r)
	if !ok {
		return nil, "", false
	}
	if c.ONVIFURL == "" {
		writeError(w, http.StatusBadRequest, "для PTZ укажите адрес ONVIF камеры")
		return nil, "", false
	}
	cl := &onvif.Client{DeviceURL: c.ONVIFURL, Username: c.Username, Password: c.Password}
	profile := c.ONVIFProfile
	if profile == "" {
		profiles, err := cl.Profiles(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return nil, "", false
		}
		for _, p := range profiles {
			if p.HasPTZ {
				profile = p.Token
				break
			}
		}
		if profile == "" {
			writeError(w, http.StatusBadRequest, "у камеры нет PTZ-профиля")
			return nil, "", false
		}
		c.ONVIFProfile = profile
		s.store.UpdateCamera(c) // remember it to skip this lookup next time
	}
	return cl, profile, true
}

func (s *Server) ptzMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pan  float64 `json:"pan"`
		Tilt float64 `json:"tilt"`
		Zoom float64 `json:"zoom"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	for _, v := range []float64{req.Pan, req.Tilt, req.Zoom} {
		if v < -1 || v > 1 {
			writeError(w, http.StatusBadRequest, "скорость должна быть от -1 до 1")
			return
		}
	}
	cl, profile, ok := s.ptzClient(w, r)
	if !ok {
		return
	}
	s.ptzResult(w, cl.ContinuousMove(r.Context(), profile, req.Pan, req.Tilt, req.Zoom))
}

func (s *Server) ptzStop(w http.ResponseWriter, r *http.Request) {
	cl, profile, ok := s.ptzClient(w, r)
	if ok {
		s.ptzResult(w, cl.Stop(r.Context(), profile))
	}
}

func (s *Server) ptzPresets(w http.ResponseWriter, r *http.Request) {
	cl, profile, ok := s.ptzClient(w, r)
	if !ok {
		return
	}
	presets, err := cl.Presets(r.Context(), profile)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, presets)
}

func (s *Server) ptzGotoPreset(w http.ResponseWriter, r *http.Request) {
	cl, profile, ok := s.ptzClient(w, r)
	if ok {
		s.ptzResult(w, cl.GotoPreset(r.Context(), profile, r.PathValue("token")))
	}
}

func (s *Server) ptzResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- ONVIF ---

func (s *Server) onvifDiscover(w http.ResponseWriter, r *http.Request) {
	devices, err := onvif.Discover(r.Context(), 3*time.Second)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка поиска: "+err.Error())
		return
	}
	// Mark devices that are already added.
	cams, _ := s.store.ListCameras()
	added := map[string]bool{}
	for _, c := range cams {
		added[c.ONVIFURL] = true
		added[c.Host] = true
	}
	type item struct {
		onvif.Discovered
		Added bool `json:"added"`
	}
	out := make([]item, len(devices))
	for i, d := range devices {
		out[i] = item{d, added[d.DeviceURL] || added[d.Address]}
	}
	writeJSON(w, http.StatusOK, out)
}

// onvifProbe connects to an ONVIF device and returns its information and stream profiles.
// With camera_id and no password, the camera's stored password is used.
func (s *Server) onvifProbe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceURL string `json:"device_url"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		CameraID  int64  `json:"camera_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Password == "" && req.CameraID > 0 {
		if c, err := s.store.Camera(req.CameraID); err == nil {
			req.Password = c.Password
		}
	}
	if req.DeviceURL == "" {
		writeError(w, http.StatusBadRequest, "укажите адрес ONVIF")
		return
	}
	cl := &onvif.Client{DeviceURL: req.DeviceURL, Username: req.Username, Password: req.Password}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	info, err := cl.DeviceInformation(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("device information: %v", err))
		return
	}
	profiles, err := cl.Profiles(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("profiles: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": info, "profiles": profiles})
}
