// Package store keeps users, sessions, cameras and events in SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(dataDir, "matrix.db") +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows a single writer; one connection avoids "database is locked".
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

var migrations = []string{
	`CREATE TABLE users (
		id            INTEGER PRIMARY KEY,
		username      TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		role          TEXT NOT NULL,
		created_at    INTEGER NOT NULL
	);
	CREATE TABLE sessions (
		token_hash TEXT PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at INTEGER NOT NULL
	);
	CREATE TABLE cameras (
		id             INTEGER PRIMARY KEY,
		name           TEXT NOT NULL,
		grp            TEXT NOT NULL DEFAULT '',
		vendor         TEXT NOT NULL,
		host           TEXT NOT NULL DEFAULT '',
		rtsp_port      INTEGER NOT NULL DEFAULT 554,
		username       TEXT NOT NULL DEFAULT '',
		password       TEXT NOT NULL DEFAULT '',
		channel        INTEGER NOT NULL DEFAULT 1,
		main_url       TEXT NOT NULL DEFAULT '',
		sub_url        TEXT NOT NULL DEFAULT '',
		onvif_url      TEXT NOT NULL DEFAULT '',
		onvif_profile  TEXT NOT NULL DEFAULT '',
		record         INTEGER NOT NULL DEFAULT 1,
		retention_days INTEGER NOT NULL DEFAULT 7,
		enabled        INTEGER NOT NULL DEFAULT 1,
		created_at     INTEGER NOT NULL
	);
	CREATE TABLE events (
		id        INTEGER PRIMARY KEY,
		ts        INTEGER NOT NULL,
		type      TEXT NOT NULL,
		camera_id INTEGER,
		message   TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX events_ts ON events(ts);`,
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	err := s.db.QueryRow(`SELECT version FROM schema_version`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`UPDATE schema_version SET version = ?`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
