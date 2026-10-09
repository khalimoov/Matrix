package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"matrix/internal/config"
	"matrix/internal/media"
	"matrix/internal/monitor"
	"matrix/internal/store"
)

// fakeMediaMTX records the path configuration Matrix pushes to it.
type fakeMediaMTX struct {
	mu    sync.Mutex
	paths map[string]map[string]any
}

func (f *fakeMediaMTX) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	switch {
	case strings.HasPrefix(r.URL.Path, "/v3/config/paths/get/"):
		conf, ok := f.paths[name]
		if !ok {
			http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(conf)
	case strings.HasPrefix(r.URL.Path, "/v3/config/paths/add/"),
		strings.HasPrefix(r.URL.Path, "/v3/config/paths/replace/"):
		var conf map[string]any
		json.NewDecoder(r.Body).Decode(&conf)
		f.paths[name] = conf
	case strings.HasPrefix(r.URL.Path, "/v3/config/paths/delete/"):
		if _, ok := f.paths[name]; !ok {
			http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
			return
		}
		delete(f.paths, name)
	case r.URL.Path == "/v3/config/paths/list", r.URL.Path == "/v3/paths/list":
		w.Write([]byte(`{"items":[]}`))
	case strings.HasSuffix(r.URL.Path, "/whip"), strings.HasSuffix(r.URL.Path, "/whep"):
		w.Header().Set("Location", r.URL.Path+"/0f8b1c2e-1111-2222-3333-444455556666")
		w.WriteHeader(http.StatusCreated)
	default:
		http.NotFound(w, r)
	}
}

type testEnv struct {
	url  string
	mtx  *fakeMediaMTX
	stor *store.Store
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	mtx := &fakeMediaMTX{paths: map[string]map[string]any{}}
	mtxSrv := httptest.NewServer(mtx)
	t.Cleanup(mtxSrv.Close)

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, u := range []struct {
		name string
		role store.Role
	}{{"admin", store.RoleAdmin}, {"viewer", store.RoleViewer}} {
		hash, _ := HashPassword("password123")
		if _, err := st.CreateUser(u.name, hash, u.role); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.Config{
		WebDir: t.TempDir(), RecordingsDir: t.TempDir(), SessionTTL: 3600e9,
		MediaMTXAPI: mtxSrv.URL, MediaMTXWebRTC: mtxSrv.URL, MediaMTXHLS: mtxSrv.URL, MediaMTXPlayback: mtxSrv.URL,
	}
	mc := media.New(cfg.MediaMTXAPI, cfg.MediaMTXPlayback)
	srv := httptest.NewServer(New(cfg, st, mc, monitor.New(st, mc)).Handler())
	t.Cleanup(srv.Close)
	return &testEnv{url: srv.URL, mtx: mtx, stor: st}
}

func (e *testEnv) client(t *testing.T, user string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if user != "" {
		res := do(t, c, "POST", e.url+"/api/auth/login", `{"username":"`+user+`","password":"password123"}`)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("login %s: %d", user, res.StatusCode)
		}
	}
	return c
}

func do(t *testing.T, c *http.Client, method, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res
}

const dahuaCamera = `{"name":"Gate","vendor":"dahua","host":"10.0.0.5","username":"admin","password":"secret",
	"record":true,"retention_days":3,"enabled":true}`

func TestAuthAndRoles(t *testing.T) {
	env := newTestEnv(t)
	anon := env.client(t, "")
	if res := do(t, anon, "GET", env.url+"/api/cameras", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous camera list: %d", res.StatusCode)
	}
	if res := do(t, anon, "POST", env.url+"/api/auth/login", `{"username":"admin","password":"nope"}`); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad password: %d", res.StatusCode)
	}

	viewer := env.client(t, "viewer")
	if res := do(t, viewer, "GET", env.url+"/api/cameras", ""); res.StatusCode != http.StatusOK {
		t.Errorf("viewer camera list: %d", res.StatusCode)
	}
	for _, path := range []string{"/api/cameras", "/api/users"} {
		if res := do(t, viewer, "POST", env.url+path, `{}`); res.StatusCode != http.StatusForbidden {
			t.Errorf("viewer POST %s: %d", path, res.StatusCode)
		}
	}
	if res := do(t, viewer, "GET", env.url+"/media/playback/list?path=cam1", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer playback: %d", res.StatusCode)
	}

	admin := env.client(t, "admin")
	if res := do(t, admin, "POST", env.url+"/api/auth/logout", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("logout: %d", res.StatusCode)
	}
	if res := do(t, admin, "GET", env.url+"/api/cameras", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("after logout: %d", res.StatusCode)
	}
}

func TestLoginLockout(t *testing.T) {
	env := newTestEnv(t)
	c := env.client(t, "")
	for i := 0; i < maxLoginFailures; i++ {
		do(t, c, "POST", env.url+"/api/auth/login", `{"username":"admin","password":"wrong"}`)
	}
	res := do(t, c, "POST", env.url+"/api/auth/login", `{"username":"admin","password":"password123"}`)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected lockout, got %d", res.StatusCode)
	}
}

func TestCameraLifecycleSyncsMediaServer(t *testing.T) {
	env := newTestEnv(t)
	admin := env.client(t, "admin")

	if res := do(t, admin, "POST", env.url+"/api/cameras", dahuaCamera); res.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", res.StatusCode)
	}
	main, sub := env.mtx.paths["cam1"], env.mtx.paths["cam1_sub"]
	if main == nil || sub == nil {
		t.Fatalf("paths not pushed: %v", env.mtx.paths)
	}
	if main["source"] != "rtsp://admin:secret@10.0.0.5:554/cam/realmonitor?channel=1&subtype=0" || main["record"] != true {
		t.Errorf("main path: %v", main)
	}
	if sub["sourceOnDemand"] != true || sub["record"] != false {
		t.Errorf("sub path: %v", sub)
	}

	// Updating without a password keeps the stored one; disabling removes the paths.
	upd := strings.Replace(strings.Replace(dahuaCamera, `"password":"secret",`, "", 1), `"enabled":true`, `"enabled":false`, 1)
	if res := do(t, admin, "PUT", env.url+"/api/cameras/1", upd); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d", res.StatusCode)
	}
	if c, _ := env.stor.Camera(1); c.Password != "secret" {
		t.Errorf("password lost on update")
	}
	if len(env.mtx.paths) != 0 {
		t.Errorf("disabled camera still configured: %v", env.mtx.paths)
	}

	if res := do(t, admin, "DELETE", env.url+"/api/cameras/1", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("delete: %d", res.StatusCode)
	}
}

func TestCameraResponseHidesPassword(t *testing.T) {
	env := newTestEnv(t)
	admin := env.client(t, "admin")
	do(t, admin, "POST", env.url+"/api/cameras", dahuaCamera)
	res, err := admin.Get(env.url + "/api/cameras")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(body), "secret") {
		t.Errorf("camera password leaked: %s", body)
	}
}

func TestMediaProxyOnlyAllowsReading(t *testing.T) {
	env := newTestEnv(t)
	viewer := env.client(t, "viewer")

	res := do(t, viewer, "POST", env.url+"/media/webrtc/cam1_sub/whep", "v=0")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("whep: %d", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, "/media/webrtc/cam1_sub/whep/") {
		t.Errorf("session location not rewritten: %q", loc)
	}
	blocked := []string{
		"/media/webrtc/cam1/whip",     // publishing
		"/media/webrtc/other/whep",    // not a camera path
		"/media/webrtc/cam1/",         // MediaMTX's own reader page
		"/media/hls/../v3/paths/list", // path traversal to the API
	}
	for _, p := range blocked {
		if res := do(t, viewer, "POST", env.url+p, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", p, res.StatusCode)
		}
	}
}
