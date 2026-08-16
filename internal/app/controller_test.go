package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

func TestControllerConfigModeBootsWhenSQLiteUnavailable(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Update(func(c *config.Config) error {
		c.Storage = "config"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Force sqlite open/configure to fail without touching config.json.
	if err := os.Mkdir(filepath.Join(dir, "ux-agent.db"), 0o700); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctrl, err := New(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("config mode should tolerate unavailable sqlite: %v", err)
	}
	defer ctrl.Close()
	if ctrl.DB != nil {
		t.Fatal("expected no sqlite handle")
	}
	s, err := ctrl.EnsureSession(context.Background())
	if err != nil {
		t.Fatalf("JSON compatibility store should remain usable: %v", err)
	}
	if s.ID == "" {
		t.Fatal("expected a persisted session")
	}
}

func TestControllerDBModeDoesNotSilentlyHideUnavailableDatabase(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Update(func(c *config.Config) error {
		c.Storage = "db"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "ux-agent.db"), 0o700); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if ctrl, err := New(context.Background(), cfg, logger); err == nil {
		ctrl.Close()
		t.Fatal("db mode must fail closed instead of showing an empty config store")
	}
}
