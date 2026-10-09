package api

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"

	"matrix/internal/store"
)

// MediaMTX is never exposed directly. Browsers reach it through these proxies,
// which check the session cookie and only allow reading camera streams.
//
//	/media/webrtc/cam1_sub/whep   -> WebRTC (WHEP) live view
//	/media/hls/cam1/index.m3u8    -> HLS live view (fallback)
//	/media/playback/get?path=cam1 -> recorded video
var (
	webrtcPath   = regexp.MustCompile(`^/cam[0-9]+(_sub)?/whep(/[A-Za-z0-9-]+)?$`)
	hlsPath      = regexp.MustCompile(`^/cam[0-9]+(_sub)?/[A-Za-z0-9_.-]+$`)
	playbackPath = regexp.MustCompile(`^/(list|get)$`)
	cameraPath   = regexp.MustCompile(`^cam[0-9]+$`)
)

func (s *Server) mountMedia(mux *http.ServeMux) {
	mux.Handle("/media/webrtc/", s.mediaProxy("/media/webrtc", s.cfg.MediaMTXWebRTC, store.RoleViewer,
		func(r *http.Request) bool { return webrtcPath.MatchString(r.URL.Path) }))
	mux.Handle("/media/hls/", s.mediaProxy("/media/hls", s.cfg.MediaMTXHLS, store.RoleViewer,
		func(r *http.Request) bool { return r.Method == http.MethodGet && hlsPath.MatchString(r.URL.Path) }))
	mux.Handle("/media/playback/", s.mediaProxy("/media/playback", s.cfg.MediaMTXPlayback, store.RoleOperator,
		func(r *http.Request) bool {
			return r.Method == http.MethodGet && playbackPath.MatchString(r.URL.Path) &&
				cameraPath.MatchString(r.URL.Query().Get("path"))
		}))
}

func (s *Server) mediaProxy(prefix, target string, min store.Role, allowed func(*http.Request) bool) http.Handler {
	u, err := url.Parse(target)
	if err != nil {
		panic("invalid media server URL " + target)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(u)
			pr.Out.Host = u.Host
			pr.Out.Header.Del("Cookie") // the session never leaves Matrix
			pr.Out.Header.Del("Authorization")
		},
		ModifyResponse: func(resp *http.Response) error {
			// WHEP returns the session URL as an absolute path on the media server.
			if loc := resp.Header.Get("Location"); strings.HasPrefix(loc, "/") {
				resp.Header.Set("Location", prefix+loc)
			}
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Credentials")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("media proxy", "path", r.URL.Path, "err", err)
			writeError(w, http.StatusBadGateway, "медиасервер недоступен")
		},
		FlushInterval: -1, // stream video without buffering
	}
	return s.require(min, func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		r.URL.RawPath = ""
		if !allowed(r) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
