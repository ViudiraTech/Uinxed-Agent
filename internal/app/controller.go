package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/agent"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	gitutil "github.com/ViudiraTech/Uinxed-Agent/internal/git"
	"github.com/ViudiraTech/Uinxed-Agent/internal/indexer"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
	"github.com/ViudiraTech/Uinxed-Agent/internal/storage"
	"github.com/ViudiraTech/Uinxed-Agent/internal/tools"
)

type Controller struct {
	Config    *config.Store
	Store     storage.Store
	DB        *storage.SQLite
	JSON      *storage.JSONStore
	Hybrid    *storage.Hybrid
	Runtime   *agent.Runtime
	Index     *indexer.FileIndex
	log       *slog.Logger
	mu        sync.Mutex
	providers map[string]*provider.OpenAICompatible
	events    <-chan domain.Event
	cancel    context.CancelFunc
}

func New(ctx context.Context, cfg *config.Store, log *slog.Logger) (*Controller, error) {
	if cfg == nil {
		return nil, errors.New("config store required")
	}
	snapBeforeDB := cfg.Snapshot()
	db, dbErr := storage.OpenSQLite(ctx, filepath.Join(cfg.Dir(), "ux-agent.db"))
	if dbErr != nil {
		// Explicit config.json compatibility mode must remain usable even if
		// SQLite is unavailable or corrupt. In db mode, failing closed avoids
		// silently presenting an empty JSON store as if database history vanished.
		if snapBeforeDB.Storage != "config" {
			return nil, fmt.Errorf("open sqlite store: %w", dbErr)
		}
		if log != nil {
			log.Warn("sqlite unavailable; continuing in config storage mode", "error", dbErr)
		}
	}
	if db != nil {
		report, migrateErr := storage.MigrateLegacyConfig(ctx, cfg, db)
		if migrateErr != nil {
			// A legacy config remains the source of truth until migration commits.
			// Let explicit config mode boot so the user's original data stays usable.
			if cfg.Snapshot().Storage != "config" {
				db.Close()
				return nil, fmt.Errorf("migrate legacy config: %w", migrateErr)
			}
			if log != nil {
				log.Warn("legacy sqlite migration failed; keeping config storage", "error", migrateErr, "backup", report.Backup)
			}
		} else if log != nil && report.Migrated > 0 {
			log.Info("legacy migration complete", "detected", report.Detected, "migrated", report.Migrated, "verified", report.Verified, "backup", report.Backup)
		}
	}
	js := storage.NewJSONStore(cfg)
	hybrid := storage.NewHybrid(db, js, func() string { return cfg.Snapshot().Storage })
	c := &Controller{Config: cfg, Store: hybrid, DB: db, JSON: js, Hybrid: hybrid, log: log, providers: map[string]*provider.OpenAICompatible{}}
	reg := tools.DefaultRegistry()
	c.Runtime = agent.NewRuntime(hybrid, reg, c.resolveProvider)
	c.Runtime.SetLogger(log)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	c.cancel = streamCancel
	interval := time.Duration(cfg.Snapshot().StreamRenderIntervalMS) * time.Millisecond
	c.events = CoalesceEvents(streamCtx, c.Runtime.Events(), interval)
	snap := cfg.Snapshot()
	cwd := snap.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	c.Index = indexer.New(cwd)
	bctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_ = c.Index.Build(bctx)
	cancel()
	return c, nil
}
func (c *Controller) Close() error {
	// Keep the coalescer draining while runtime goroutines wind down. Terminal
	// lifecycle delivery is bounded, so shutdown cannot deadlock on UI events.
	c.Runtime.Close()
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.Index.Close()
	return c.Hybrid.Close()
}
func (c *Controller) Events() <-chan domain.Event { return c.events }

func (c *Controller) resolveProvider(id string) (provider.Provider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snap := c.Config.Snapshot()
	var pc *config.Provider
	for i := range snap.Providers {
		if snap.Providers[i].ID == id {
			p := snap.Providers[i]
			pc = &p
			break
		}
	}
	if pc == nil {
		return nil, fmt.Errorf("provider %q not configured", id)
	}
	if p := c.providers[id]; p != nil {
		p.Update(*pc)
		return p, nil
	}
	p := provider.NewOpenAICompatible(*pc, func() (string, error) { return c.Config.ProviderKey(id) })
	c.providers[id] = p
	return p, nil
}

func (c *Controller) ListSessions(ctx context.Context) ([]domain.Session, error) {
	return c.Store.ListSessions(ctx)
}
func (c *Controller) LoadSession(ctx context.Context, id string) (domain.Session, error) {
	return c.Store.LoadSession(ctx, id)
}

func (c *Controller) EnsureSession(ctx context.Context) (domain.Session, error) {
	cfg := c.Config.Snapshot()
	if cfg.ActiveSessionID != "" {
		if s, err := c.Store.LoadSession(ctx, cfg.ActiveSessionID); err == nil {
			return s, nil
		}
	}
	list, err := c.Store.ListSessions(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if len(list) > 0 {
		_ = c.Config.Update(func(x *config.Config) error { x.ActiveSessionID = list[0].ID; return nil })
		return c.Store.LoadSession(ctx, list[0].ID)
	}
	return c.NewSession(ctx, "会话 1")
}

func (c *Controller) NewSession(ctx context.Context, name string) (domain.Session, error) {
	cfg := c.Config.Snapshot()
	if strings.TrimSpace(name) == "" {
		name = "新会话"
	}
	cwd := cfg.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	now := time.Now()
	s := domain.Session{ID: fmt.Sprintf("s-%d", now.UnixNano()), Name: name, CreatedAt: now, UpdatedAt: now, ProviderID: cfg.ActiveProvider, Model: cfg.Model, AgentID: "build", CWD: cwd, Metadata: map[string]any{"effort": cfg.Effort, "thinking": cfg.Thinking}}
	if err := c.Store.SaveSession(ctx, s); err != nil {
		return s, err
	}
	if err := c.SwitchSession(ctx, s.ID); err != nil {
		return s, err
	}
	return s, nil
}
func (c *Controller) SwitchSession(ctx context.Context, id string) error {
	if _, err := c.Store.LoadSession(ctx, id); err != nil {
		return err
	}
	return c.Config.Update(func(x *config.Config) error { x.ActiveSessionID = id; return nil })
}
func (c *Controller) DeleteSession(ctx context.Context, id string) error {
	if _, err := c.Runtime.CancelAndWait(ctx, id); err != nil {
		return fmt.Errorf("cancel active session before delete: %w", err)
	}
	if err := c.Store.DeleteSession(ctx, id); err != nil {
		return err
	}
	cfg := c.Config.Snapshot()
	if cfg.ActiveSessionID == id {
		list, _ := c.Store.ListSessions(ctx)
		next := ""
		if len(list) > 0 {
			next = list[0].ID
		}
		return c.Config.Update(func(x *config.Config) error { x.ActiveSessionID = next; return nil })
	}
	return nil
}

func (c *Controller) Submit(ctx context.Context, sessionID, text string) (string, error) {
	return c.Runtime.StartTurn(ctx, sessionID, text)
}
func (c *Controller) Cancel(sessionID string) bool { return c.Runtime.Cancel(sessionID) }

func (c *Controller) SetAgent(ctx context.Context, sessionID, id string) error {
	def := agent.Get(id)
	if !def.CanPrimary() {
		return fmt.Errorf("%s is not a primary agent", id)
	}
	s, err := c.Store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	s.AgentID = id
	s.UpdatedAt = time.Now()
	return c.Store.SaveSession(ctx, s)
}
func (c *Controller) SetModel(ctx context.Context, sessionID, model string) error {
	s, err := c.Store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	s.Model = model
	s.UpdatedAt = time.Now()
	if err := c.Store.SaveSession(ctx, s); err != nil {
		return err
	}
	return c.Config.Update(func(x *config.Config) error { x.Model = model; return nil })
}
func (c *Controller) SetProvider(ctx context.Context, sessionID, id string) error {
	if err := c.Config.SetActiveProvider(id); err != nil {
		return err
	}
	cfg := c.Config.Snapshot()
	s, err := c.Store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	s.ProviderID = id
	s.Model = cfg.Model
	s.UpdatedAt = time.Now()
	return c.Store.SaveSession(ctx, s)
}
func (c *Controller) SetEffort(ctx context.Context, sessionID, effort string) error {
	err := c.Config.Update(func(x *config.Config) error { x.Effort = effort; return nil })
	if err != nil {
		return err
	}
	s, err := c.Store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if s.Metadata == nil {
		s.Metadata = map[string]any{}
	}
	s.Metadata["effort"] = effort
	s.UpdatedAt = time.Now()
	return c.Store.SaveSession(ctx, s)
}
func (c *Controller) SetThinking(ctx context.Context, sessionID string, on bool) error {
	if err := c.Config.Update(func(x *config.Config) error { x.Thinking = on; return nil }); err != nil {
		return err
	}
	s, err := c.Store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if s.Metadata == nil {
		s.Metadata = map[string]any{}
	}
	s.Metadata["thinking"] = on
	s.UpdatedAt = time.Now()
	return c.Store.SaveSession(ctx, s)
}
func (c *Controller) ChangeCWD(ctx context.Context, sessionID, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}
	s, err := c.Store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	s.CWD = abs
	s.UpdatedAt = time.Now()
	if err := c.Store.SaveSession(ctx, s); err != nil {
		return err
	}
	_ = c.Config.Update(func(x *config.Config) error { x.CWD = abs; return nil })
	if c.Index != nil {
		_ = c.Index.Close()
	}
	c.Index = indexer.New(abs)
	bctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Index.Build(bctx)
}
func (c *Controller) Diff(ctx context.Context, cwd string) (gitutil.Snapshot, error) {
	return gitutil.Diff(ctx, cwd)
}
func (c *Controller) FileDiff(ctx context.Context, cwd, path string) (string, error) {
	return gitutil.FileDiff(ctx, cwd, path)
}
func (c *Controller) Models(ctx context.Context, providerID string) ([]string, error) {
	p, err := c.resolveProvider(providerID)
	if err != nil {
		return nil, err
	}
	return p.Models(ctx)
}
func (c *Controller) CheckKey(ctx context.Context, providerID, key string) error {
	p, err := c.resolveProvider(providerID)
	if err != nil {
		return err
	}
	return p.CheckKey(ctx, key)
}
func (c *Controller) SetKey(providerID, key string) error {
	return c.Config.SetProviderKey(providerID, key)
}
func (c *Controller) Reset(ctx context.Context) error {
	c.Runtime.Close()
	_ = c.Store.DeleteAllSessions(ctx)
	_ = c.Store.Close()
	return c.Config.Reset()
}
func (c *Controller) Log() *slog.Logger { return c.log }

func (c *Controller) ClearSession(ctx context.Context, id string) error {
	s, err := c.Store.LoadSession(ctx, id)
	if err != nil {
		return err
	}
	s.Messages = nil
	s.Todos = nil
	s.ToolActivities = nil
	s.UpdatedAt = time.Now()
	return c.Store.SaveSession(ctx, s)
}

func (c *Controller) RenameSession(ctx context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("session name cannot be empty")
	}
	s, err := c.Store.LoadSession(ctx, id)
	if err != nil {
		return err
	}
	s.Name = name
	s.UpdatedAt = time.Now()
	return c.Store.SaveSession(ctx, s)
}

func (c *Controller) Compact(ctx context.Context, id string) error {
	return c.Runtime.Compact(ctx, id)
}

func (c *Controller) Profile(ctx context.Context, providerID string) (map[string]any, error) {
	p, err := c.resolveProvider(providerID)
	if err != nil {
		return nil, err
	}
	o, ok := p.(*provider.OpenAICompatible)
	if !ok {
		return nil, errors.New("provider does not expose profile")
	}
	return o.Profile(ctx)
}

func (c *Controller) SwitchStorage(ctx context.Context, target string) (int, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target != "db" && target != "config" {
		return 0, errors.New("storage must be db or config")
	}
	if c.Runtime.IsRunningAny() {
		return 0, errors.New("cannot switch storage while an agent is running")
	}
	current := c.Config.Snapshot().Storage
	if current == target {
		ss, err := c.Store.ListSessions(ctx)
		return len(ss), err
	}
	if target == "config" {
		ss, err := c.Store.ListSessions(ctx)
		if err != nil {
			return 0, err
		}
		active := c.Config.Snapshot().ActiveSessionID
		if err := c.Config.SaveLegacySessions(ss, active); err != nil {
			return 0, err
		}
		if err := c.Config.Update(func(x *config.Config) error { x.Storage = "config"; return nil }); err != nil {
			return 0, err
		}
		db := c.Hybrid.DB()
		c.Hybrid.SetDB(nil)
		c.DB = nil
		if db != nil {
			_ = db.Close()
			path := db.Path()
			for _, f := range []string{path, path + "-wal", path + "-shm"} {
				_ = os.Remove(f)
			}
		}
		return len(ss), nil
	}
	ss, err := c.Store.ListSessions(ctx)
	if err != nil {
		return 0, err
	}
	path := filepath.Join(c.Config.Dir(), "ux-agent.db")
	db, err := storage.OpenSQLite(ctx, path)
	if err != nil {
		return 0, err
	}
	if err := db.ImportSessions(ctx, ss); err != nil {
		db.Close()
		return 0, fmt.Errorf("atomic storage migration: %w", err)
	}
	for _, sess := range ss {
		got, err := db.LoadSession(ctx, sess.ID)
		if err != nil || len(got.Messages) != len(sess.Messages) {
			db.Close()
			if err != nil {
				return 0, fmt.Errorf("verify %s: %w", sess.ID, err)
			}
			return 0, fmt.Errorf("verify %s: message count mismatch", sess.ID)
		}
	}
	c.Hybrid.SetDB(db)
	c.DB = db
	if err := c.Config.Update(func(x *config.Config) error { x.Storage = "db"; return nil }); err != nil {
		db.Close()
		c.Hybrid.SetDB(nil)
		c.DB = nil
		return 0, err
	}
	return len(ss), nil
}

func (c *Controller) StartSubagent(ctx context.Context, parentSessionID, agentID, task string) (domain.Session, string, error) {
	return c.Runtime.StartSubagent(ctx, parentSessionID, agentID, task)
}
