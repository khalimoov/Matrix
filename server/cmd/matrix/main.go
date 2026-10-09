// Command matrix runs the Matrix video management server.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"matrix/internal/api"
	"matrix/internal/config"
	"matrix/internal/media"
	"matrix/internal/monitor"
	"matrix/internal/store"
)

func main() {
	level := slog.LevelInfo
	if os.Getenv("MATRIX_DEBUG") == "true" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := createFirstAdmin(db, cfg.AdminPassword); err != nil {
		return err
	}

	mediaClient := media.New(cfg.MediaMTXAPI, cfg.MediaMTXPlayback)
	mon := monitor.New(db, mediaClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go mon.Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.New(cfg, db, mediaClient, mon).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No write timeout: live events and video are long-running responses.
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("Matrix server started", "addr", cfg.Addr, "version", api.Version)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// createFirstAdmin creates the "admin" account on an empty database.
func createFirstAdmin(db *store.Store, password string) error {
	n, err := db.CountUsers()
	if err != nil || n > 0 {
		return err
	}
	generated := password == ""
	if generated {
		password = api.RandomPassword()
	}
	hash, err := api.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := db.CreateUser("admin", hash, store.RoleAdmin); err != nil {
		return err
	}
	if generated {
		slog.Warn("created administrator account; change this password after signing in",
			"username", "admin", "password", password)
	} else {
		slog.Info("created administrator account", "username", "admin")
	}
	return nil
}
