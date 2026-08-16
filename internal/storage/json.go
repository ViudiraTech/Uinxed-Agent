package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

type JSONStore struct {
	cfg *config.Store
	mu  sync.Mutex
}

func NewJSONStore(cfg *config.Store) *JSONStore { return &JSONStore{cfg: cfg} }

func (j *JSONStore) all() ([]domain.Session, string, error) {
	snap, err := config.ReadLegacySessions(j.cfg.File())
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	cfg := j.cfg.Snapshot()
	for i := range snap.Sessions {
		if snap.Sessions[i].ProviderID == "" {
			snap.Sessions[i].ProviderID = cfg.ActiveProvider
		}
		if snap.Sessions[i].Model == "" {
			snap.Sessions[i].Model = cfg.Model
		}
		if snap.Sessions[i].Metadata == nil {
			snap.Sessions[i].Metadata = map[string]any{"effort": cfg.Effort, "thinking": cfg.Thinking}
		}
	}
	return snap.Sessions, snap.ActiveSessionID, nil
}

func (j *JSONStore) ListSessions(ctx context.Context) ([]domain.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	ss, _, err := j.all()
	if err != nil {
		return nil, err
	}
	sort.Slice(ss, func(i, k int) bool { return ss[i].UpdatedAt.After(ss[k].UpdatedAt) })
	return ss, nil
}

func (j *JSONStore) SearchSessions(ctx context.Context, q string, limit int) ([]domain.Session, error) {
	ss, err := j.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var out []domain.Session
	for _, s := range ss {
		if q == "" || strings.Contains(strings.ToLower(s.Name), q) {
			out = append(out, s)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (j *JSONStore) LoadSession(ctx context.Context, id string) (domain.Session, error) {
	if err := ctx.Err(); err != nil {
		return domain.Session{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	ss, _, err := j.all()
	if err != nil {
		return domain.Session{}, err
	}
	for _, s := range ss {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Session{}, sql.ErrNoRows
}

func (j *JSONStore) SaveSession(ctx context.Context, s domain.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	ss, active, err := j.all()
	if err != nil {
		return err
	}
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	found := false
	for i := range ss {
		if ss[i].ID == s.ID {
			ss[i] = s
			found = true
			break
		}
	}
	if !found {
		ss = append(ss, s)
	}
	if active == "" {
		active = s.ID
	}
	return j.cfg.SaveLegacySessions(ss, active)
}

func (j *JSONStore) DeleteSession(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	ss, active, err := j.all()
	if err != nil {
		return err
	}
	out := make([]domain.Session, 0, len(ss))
	for _, s := range ss {
		if s.ID != id {
			out = append(out, s)
		}
	}
	if active == id {
		active = ""
		if len(out) > 0 {
			active = out[0].ID
		}
	}
	return j.cfg.SaveLegacySessions(out, active)
}

func (j *JSONStore) DeleteAllSessions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cfg.SaveLegacySessions(nil, "")
}

func (j *JSONStore) Close() error { return nil }
