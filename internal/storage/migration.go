package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

type MigrationReport struct {
	Detected      int
	Migrated      int
	Verified      int
	ActiveSession string
	Backup        string
}

func MigrateLegacyConfig(ctx context.Context, cfg *config.Store, dst *SQLite) (MigrationReport, error) {
	var report MigrationReport
	snapshot, err := config.ReadLegacySessions(cfg.File())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return report, nil
		}
		return report, err
	}
	report.Detected = len(snapshot.Sessions)
	report.ActiveSession = snapshot.ActiveSessionID
	if report.Detected == 0 {
		return report, nil
	}

	raw, err := os.ReadFile(cfg.File())
	if err != nil {
		return report, err
	}
	backup := filepath.Join(cfg.Dir(), "config.pre-go-migration."+time.Now().UTC().Format("20060102T150405Z")+".json")
	if err := os.WriteFile(backup, raw, 0o600); err != nil {
		return report, fmt.Errorf("backup legacy config: %w", err)
	}
	report.Backup = backup

	if err := dst.ImportSessions(ctx, snapshot.Sessions); err != nil {
		return report, fmt.Errorf("atomic session import: %w", err)
	}
	report.Migrated = len(snapshot.Sessions)
	for _, s := range snapshot.Sessions {
		got, err := dst.LoadSession(ctx, s.ID)
		if err != nil {
			return report, fmt.Errorf("verify session %s: %w", s.ID, err)
		}
		if len(got.Messages) != len(s.Messages) {
			return report, fmt.Errorf("verify session %s: message count %d != %d", s.ID, len(got.Messages), len(s.Messages))
		}
		report.Verified++
	}
	if report.Migrated != report.Detected || report.Verified != report.Detected {
		return report, fmt.Errorf("migration incomplete: %+v", report)
	}
	err = cfg.Update(func(c *config.Config) error {
		c.Storage = "db"
		if snapshot.ActiveSessionID != "" {
			c.ActiveSessionID = snapshot.ActiveSessionID
		}
		return nil
	})
	return report, err
}
