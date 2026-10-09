// Package api is the HTTP interface of Matrix: the JSON API under /api,
// authenticated media proxies under /media and the web client.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"matrix/internal/config"
	"matrix/internal/media"
	"matrix/internal/monitor"
	"matrix/internal/store"
)

const Version = "0.1.0"

type Server struct {
	cfg     config.Config
	store   *store.Store
	media   *media.Client
	monitor *monitor.Monitor
	limiter *loginLimiter
	started time.Time
}

func New(cfg config.Config, s *store.Store, m *media.Client, mon *monitor.Monitor) *Server {
	return &Server{cfg: cfg, store: s, media: m, monitor: mon, limiter: newLoginLimiter(), started: time.Now()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.Handle("GET /api/auth/me", s.require(store.RoleViewer, s.me))
	mux.Handle("POST /api/auth/password", s.require(store.RoleViewer, s.changeOwnPassword))

	mux.Handle("GET /api/system", s.require(store.RoleViewer, s.system))

	mux.Handle("GET /api/cameras", s.require(store.RoleViewer, s.listCameras))
	mux.Handle("POST /api/cameras", s.require(store.RoleAdmin, s.createCamera))
	mux.Handle("GET /api/cameras/{id}", s.require(store.RoleViewer, s.getCamera))
	mux.Handle("PUT /api/cameras/{id}", s.require(store.RoleAdmin, s.updateCamera))
	mux.Handle("DELETE /api/cameras/{id}", s.require(store.RoleAdmin, s.deleteCamera))
	mux.Handle("GET /api/cameras/{id}/recordings", s.require(store.RoleOperator, s.recordings))
	mux.Handle("POST /api/cameras/{id}/ptz/move", s.require(store.RoleOperator, s.ptzMove))
	mux.Handle("POST /api/cameras/{id}/ptz/stop", s.require(store.RoleOperator, s.ptzStop))
	mux.Handle("GET /api/cameras/{id}/ptz/presets", s.require(store.RoleOperator, s.ptzPresets))
	mux.Handle("POST /api/cameras/{id}/ptz/presets/{token}", s.require(store.RoleOperator, s.ptzGotoPreset))

	mux.Handle("POST /api/onvif/discover", s.require(store.RoleAdmin, s.onvifDiscover))
	mux.Handle("POST /api/onvif/probe", s.require(store.RoleAdmin, s.onvifProbe))

	mux.Handle("GET /api/events", s.require(store.RoleOperator, s.listEvents))
	mux.Handle("GET /api/events/stream", s.require(store.RoleViewer, s.eventStream))

	mux.Handle("GET /api/users", s.require(store.RoleAdmin, s.listUsers))
	mux.Handle("POST /api/users", s.require(store.RoleAdmin, s.createUser))
	mux.Handle("PUT /api/users/{id}", s.require(store.RoleAdmin, s.updateUser))
	mux.Handle("DELETE /api/users/{id}", s.require(store.RoleAdmin, s.deleteUser))

	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "unknown API endpoint")
	}))

	s.mountMedia(mux)
	mux.Handle("/", s.webClient())

	return securityHeaders(logRequests(mux))
}

// webClient serves the single-page web client, falling back to index.html
// so client-side routes survive a page reload.
func (s *Server) webClient() http.Handler {
	files := http.FileServer(http.Dir(s.cfg.WebDir))
	index := filepath.Join(s.cfg.WebDir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(s.cfg.WebDir, filepath.Clean("/"+r.URL.Path))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "Web client is not built. Run: cd web && npm install && npm run build", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

// --- helpers ---

type ctxKey struct{}

func currentUser(r *http.Request) store.User {
	u, _ := r.Context().Value(ctxKey{}).(store.User)
	return u
}

// require wraps a handler so it only runs for signed-in users with at least the given role.
func (s *Server) require(min store.Role, h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "требуется вход")
			return
		}
		if !u.Role.AtLeast(min) {
			writeError(w, http.StatusForbidden, "недостаточно прав")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeStoreError maps storage errors to HTTP responses.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case strings.Contains(err.Error(), "UNIQUE constraint"):
		writeError(w, http.StatusConflict, "уже существует")
	default:
		slog.Error("store", "err", err)
		writeError(w, http.StatusInternalServerError, "внутренняя ошибка")
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush keeps streaming responses (SSE, media) working through the recorder.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Debug("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "took", time.Since(start))
	})
}
