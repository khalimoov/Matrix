package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"matrix/internal/store"
)

const sessionCookie = "matrix_session"

func (s *Server) authenticate(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.User{}, false
	}
	u, err := s.store.SessionUser(c.Value)
	return u, err == nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	key := strings.ToLower(strings.TrimSpace(req.Username)) + "|" + clientIP(r)
	if wait := s.limiter.blockedFor(key); wait > 0 {
		writeError(w, http.StatusTooManyRequests, "слишком много неудачных попыток, повторите через "+wait.Round(time.Second).String())
		return
	}
	u, err := s.store.UserByName(req.Username)
	if err == nil {
		err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password))
	} else {
		// Spend the same time as a real check so usernames cannot be probed.
		bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
	}
	if err != nil {
		s.limiter.fail(key)
		s.monitor.Emit(store.EventLoginFailed, 0, "Неудачная попытка входа под именем "+strings.TrimSpace(req.Username)+" с адреса "+clientIP(r))
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	s.limiter.reset(key)

	token := randomToken()
	if err := s.store.CreateSession(token, u.ID, s.cfg.SessionTTL); err != nil {
		writeStoreError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	s.monitor.Emit(store.EventLogin, 0, "Пользователь "+u.Username+" вошёл с адреса "+clientIP(r))
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

func (s *Server) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	u := currentUser(r)
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)) != nil {
		writeError(w, http.StatusBadRequest, "текущий пароль указан неверно")
		return
	}
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateUserPassword(u.ID, hash); err != nil {
		writeStoreError(w, err)
		return
	}
	s.monitor.Emit(store.EventUserChanged, 0, "Пользователь "+u.Username+" сменил пароль")
	w.WriteHeader(http.StatusNoContent)
}

func HashPassword(pw string) (string, error) {
	if len(pw) < 8 {
		return "", errors.New("пароль должен быть не короче 8 символов")
	}
	if len(pw) > 72 {
		return "", errors.New("пароль должен быть не длиннее 72 символов")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("matrix-dummy-password"), bcrypt.DefaultCost)

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// RandomPassword generates a readable initial password.
func RandomPassword() string {
	return randomToken()[:16]
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginLimiter locks a username+IP pair for a while after repeated failures.
type loginLimiter struct {
	mu    sync.Mutex
	fails map[string]*failure
}

type failure struct {
	count int       // failures since first
	first time.Time // start of the counting window
	until time.Time // locked until this time
}

const (
	maxLoginFailures = 5
	failureWindow    = 15 * time.Minute
	loginLockout     = time.Minute
)

func newLoginLimiter() *loginLimiter { return &loginLimiter{fails: map[string]*failure{}} }

func (l *loginLimiter) blockedFor(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f, ok := l.fails[key]; ok {
		return max(time.Until(f.until), 0)
	}
	return 0
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, f := range l.fails { // forget stale entries to keep the map small
		if now.After(f.until) && now.Sub(f.first) > failureWindow {
			delete(l.fails, k)
		}
	}
	f, ok := l.fails[key]
	if !ok {
		f = &failure{first: now}
		l.fails[key] = f
	}
	f.count++
	if f.count >= maxLoginFailures {
		f.until = now.Add(loginLockout)
		f.count, f.first = 0, now
	}
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.fails, key)
	l.mu.Unlock()
}
