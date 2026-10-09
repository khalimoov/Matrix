// Package monitor watches camera health, records events and fans them out to
// connected clients.
package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"matrix/internal/media"
	"matrix/internal/store"
)

type Monitor struct {
	store *store.Store
	media *media.Client

	mu          sync.RWMutex
	status      map[int64]media.PathState // last known state per camera
	mediaOnline bool
	quietUntil  time.Time // no status events while cameras reconnect after a sync
	subs        map[chan store.Event]struct{}
}

func New(s *store.Store, m *media.Client) *Monitor {
	return &Monitor{
		store:  s,
		media:  m,
		status: map[int64]media.PathState{},
		subs:   map[chan store.Event]struct{}{},
	}
}

// Emit stores an event and delivers it to every subscriber.
func (m *Monitor) Emit(typ store.EventType, cameraID int64, message string) {
	e := store.Event{Type: typ, Message: message}
	if cameraID > 0 {
		e.CameraID = &cameraID
	}
	e, err := m.store.AddEvent(e)
	if err != nil {
		slog.Error("store event", "err", err)
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch := range m.subs {
		select {
		case ch <- e:
		default: // slow client: drop rather than block everyone
		}
	}
}

// Subscribe returns a channel of new events; call the returned func to stop.
func (m *Monitor) Subscribe() (<-chan store.Event, func()) {
	ch := make(chan store.Event, 64)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		delete(m.subs, ch)
		m.mu.Unlock()
	}
}

// Status returns the last known stream state of a camera.
func (m *Monitor) Status(cameraID int64) (media.PathState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, ok := m.status[cameraID]
	return st, ok
}

func (m *Monitor) MediaOnline() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mediaOnline
}

// Forget drops the state of a deleted camera.
func (m *Monitor) Forget(cameraID int64) {
	m.mu.Lock()
	delete(m.status, cameraID)
	m.mu.Unlock()
}

// Run polls MediaMTX until ctx is cancelled. On every (re)connection to MediaMTX
// it pushes the full camera configuration, so a restarted media server recovers.
func (m *Monitor) Run(ctx context.Context) {
	poll := time.NewTicker(5 * time.Second)
	defer poll.Stop()
	cleanup := time.NewTicker(time.Hour)
	defer cleanup.Stop()
	m.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			m.tick(ctx)
		case <-cleanup.C:
			m.store.DeleteExpiredSessions()
			m.store.DeleteEventsBefore(time.Now().AddDate(0, 0, -90))
		}
	}
}

func (m *Monitor) tick(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	states, err := m.media.PathStates(ctx)
	m.mu.Lock()
	wasOnline := m.mediaOnline
	m.mediaOnline = err == nil
	m.mu.Unlock()
	if err != nil {
		if wasOnline {
			slog.Error("media server unreachable", "err", err)
			m.Emit(store.EventSystem, 0, "Медиасервер недоступен")
		}
		return
	}
	if !wasOnline {
		slog.Info("media server connected, syncing cameras")
		m.quietUntil = time.Now().Add(20 * time.Second)
		if err := m.SyncAll(ctx); err != nil {
			slog.Error("sync cameras", "err", err)
		}
		if states, err = m.media.PathStates(ctx); err != nil {
			return
		}
	}

	cams, err := m.store.ListCameras()
	if err != nil {
		slog.Error("list cameras", "err", err)
		return
	}
	for _, c := range cams {
		st := states[c.PathName()]
		if sub, ok := states[c.SubPathName()]; ok {
			st.Readers += sub.Readers
		}
		m.mu.Lock()
		prev, known := m.status[c.ID]
		m.status[c.ID] = st
		m.mu.Unlock()
		if !c.Enabled || !known || prev.Online == st.Online || time.Now().Before(m.quietUntil) {
			continue
		}
		if st.Online {
			m.Emit(store.EventCameraOnline, c.ID, "Камера «"+c.Name+"» в сети")
		} else {
			m.Emit(store.EventCameraOffline, c.ID, "Камера «"+c.Name+"»: потеря сигнала")
		}
	}
}

// SyncAll pushes every camera to MediaMTX and removes paths it does not know.
func (m *Monitor) SyncAll(ctx context.Context) error {
	cams, err := m.store.ListCameras()
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, c := range cams {
		want[c.PathName()] = true
		want[c.SubPathName()] = true
		if err := m.media.SyncCamera(ctx, c); err != nil {
			slog.Error("sync camera", "camera", c.ID, "err", err)
		}
	}
	paths, err := m.media.ConfiguredPaths(ctx)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if !want[p] && isCameraPath(p) {
			m.media.RemoveCamera(ctx, store.Camera{ID: cameraIDFromPath(p)})
		}
	}
	return nil
}

func isCameraPath(p string) bool { return cameraIDFromPath(p) > 0 }

func cameraIDFromPath(p string) int64 {
	if len(p) < 4 || p[:3] != "cam" {
		return 0
	}
	var id int64
	for _, r := range p[3:] {
		if r == '_' {
			break
		}
		if r < '0' || r > '9' {
			return 0
		}
		id = id*10 + int64(r-'0')
	}
	return id
}
