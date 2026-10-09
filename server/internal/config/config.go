// Package config reads server settings from environment variables.
package config

import (
	"os"
	"time"
)

type Config struct {
	Addr          string // HTTP listen address of the Matrix server
	DataDir       string // directory for the SQLite database
	WebDir        string // directory with the built web client
	RecordingsDir string // MediaMTX recordings directory (used for disk statistics)
	AdminPassword string // initial password of the "admin" user (only used on first start)
	SessionTTL    time.Duration
	SecureCookies bool // set when Matrix is served over HTTPS

	MediaMTXAPI      string
	MediaMTXWebRTC   string
	MediaMTXHLS      string
	MediaMTXPlayback string
}

func Load() Config {
	return Config{
		Addr:          env("MATRIX_ADDR", ":8080"),
		DataDir:       env("MATRIX_DATA_DIR", "./data"),
		WebDir:        env("MATRIX_WEB_DIR", "./web"),
		RecordingsDir: env("MATRIX_RECORDINGS_DIR", "./recordings"),
		AdminPassword: os.Getenv("MATRIX_ADMIN_PASSWORD"),
		SessionTTL:    duration("MATRIX_SESSION_TTL", 12*time.Hour),
		SecureCookies: os.Getenv("MATRIX_SECURE_COOKIES") == "true",

		MediaMTXAPI:      env("MATRIX_MEDIAMTX_API", "http://127.0.0.1:9997"),
		MediaMTXWebRTC:   env("MATRIX_MEDIAMTX_WEBRTC", "http://127.0.0.1:8889"),
		MediaMTXHLS:      env("MATRIX_MEDIAMTX_HLS", "http://127.0.0.1:8888"),
		MediaMTXPlayback: env("MATRIX_MEDIAMTX_PLAYBACK", "http://127.0.0.1:9996"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func duration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
