package store

import (
	"strings"
	"time"
)

type EventType string

const (
	EventCameraOnline  EventType = "camera_online"
	EventCameraOffline EventType = "camera_offline"
	EventCameraAdded   EventType = "camera_added"
	EventCameraChanged EventType = "camera_changed"
	EventCameraDeleted EventType = "camera_deleted"
	EventLogin         EventType = "login"
	EventLoginFailed   EventType = "login_failed"
	EventUserChanged   EventType = "user_changed"
	EventSystem        EventType = "system"
)

type Event struct {
	ID       int64     `json:"id"`
	Time     time.Time `json:"time"`
	Type     EventType `json:"type"`
	CameraID *int64    `json:"camera_id,omitempty"`
	Message  string    `json:"message"`
}

func (s *Store) AddEvent(e Event) (Event, error) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	res, err := s.db.Exec(`INSERT INTO events (ts, type, camera_id, message) VALUES (?, ?, ?, ?)`,
		e.Time.UnixMilli(), e.Type, e.CameraID, e.Message)
	if err != nil {
		return Event{}, err
	}
	e.ID, _ = res.LastInsertId()
	return e, nil
}

type EventFilter struct {
	BeforeID int64       // only events with a smaller ID (for paging back in time)
	CameraID int64       // 0 means any camera
	Types    []EventType // empty means any type
	Limit    int
}

func (s *Store) ListEvents(f EventFilter) ([]Event, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.BeforeID > 0 {
		where = append(where, "id < ?")
		args = append(args, f.BeforeID)
	}
	if f.CameraID > 0 {
		where = append(where, "camera_id = ?")
		args = append(args, f.CameraID)
	}
	if len(f.Types) > 0 {
		where = append(where, "type IN (?"+strings.Repeat(", ?", len(f.Types)-1)+")")
		for _, t := range f.Types {
			args = append(args, t)
		}
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	args = append(args, f.Limit)
	rows, err := s.db.Query(`SELECT id, ts, type, camera_id, message FROM events WHERE `+
		strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Type, &e.CameraID, &e.Message); err != nil {
			return nil, err
		}
		e.Time = time.UnixMilli(ts)
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) DeleteEventsBefore(t time.Time) error {
	_, err := s.db.Exec(`DELETE FROM events WHERE ts < ?`, t.UnixMilli())
	return err
}
