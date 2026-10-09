package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"matrix/internal/store"
)

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.EventFilter{}
	f.BeforeID, _ = strconv.ParseInt(q.Get("before"), 10, 64)
	f.CameraID, _ = strconv.ParseInt(q.Get("camera_id"), 10, 64)
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if t := q.Get("types"); t != "" {
		for _, typ := range strings.Split(t, ",") {
			f.Types = append(f.Types, store.EventType(typ))
		}
	}
	events, err := s.store.ListEvents(f)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// eventStream pushes new events to the client with Server-Sent Events.
func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc.Flush()

	// Viewers only learn about camera status; the full log is for operators.
	everything := currentUser(r).Role.AtLeast(store.RoleOperator)

	events, stop := s.monitor.Subscribe()
	defer stop()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-events:
			if !everything && e.Type != store.EventCameraOnline && e.Type != store.EventCameraOffline {
				continue
			}
			data, _ := json.Marshal(e)
			if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.ID, data); err != nil {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
		}
		if rc.Flush() != nil {
			return
		}
	}
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	cams, err := s.store.ListCameras()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var enabled, online, recording, viewers int
	for _, c := range cams {
		if !c.Enabled {
			continue
		}
		enabled++
		if st, ok := s.monitor.Status(c.ID); ok {
			if st.Online {
				online++
				if c.Record {
					recording++
				}
			}
			viewers += st.Readers
		}
	}
	res := map[string]any{
		"version":         Version,
		"uptime_seconds":  int(time.Since(s.started).Seconds()),
		"media_online":    s.monitor.MediaOnline(),
		"cameras_total":   len(cams),
		"cameras_enabled": enabled,
		"cameras_online":  online,
		"recording":       recording,
		"viewers":         viewers,
	}
	var st syscall.Statfs_t
	if syscall.Statfs(s.cfg.RecordingsDir, &st) == nil {
		res["disk_total_bytes"] = st.Blocks * uint64(st.Bsize)
		res["disk_free_bytes"] = st.Bavail * uint64(st.Bsize)
	}
	writeJSON(w, http.StatusOK, res)
}
