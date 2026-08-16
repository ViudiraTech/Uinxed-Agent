package storage

import (
	"context"
	"errors"
	"sync"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

type Hybrid struct {
	mu   sync.RWMutex
	db   *SQLite
	js   *JSONStore
	mode func() string
}

func NewHybrid(db *SQLite, js *JSONStore, mode func() string) *Hybrid {
	return &Hybrid{db: db, js: js, mode: mode}
}
func (h *Hybrid) SetDB(db *SQLite) { h.mu.Lock(); h.db = db; h.mu.Unlock() }
func (h *Hybrid) DB() *SQLite      { h.mu.RLock(); defer h.mu.RUnlock(); return h.db }
func (h *Hybrid) target() (Store, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.mode() != "config" {
		if h.db == nil {
			return nil, errors.New("sqlite store is not open")
		}
		return h.db, nil
	}
	if h.js == nil {
		return nil, errors.New("config store unavailable")
	}
	return h.js, nil
}
func (h *Hybrid) ListSessions(ctx context.Context) ([]domain.Session, error) {
	s, e := h.target()
	if e != nil {
		return nil, e
	}
	return s.ListSessions(ctx)
}
func (h *Hybrid) SearchSessions(ctx context.Context, q string, l int) ([]domain.Session, error) {
	s, e := h.target()
	if e != nil {
		return nil, e
	}
	return s.SearchSessions(ctx, q, l)
}
func (h *Hybrid) LoadSession(ctx context.Context, id string) (domain.Session, error) {
	s, e := h.target()
	if e != nil {
		return domain.Session{}, e
	}
	return s.LoadSession(ctx, id)
}
func (h *Hybrid) SaveSession(ctx context.Context, v domain.Session) error {
	s, e := h.target()
	if e != nil {
		return e
	}
	return s.SaveSession(ctx, v)
}
func (h *Hybrid) DeleteSession(ctx context.Context, id string) error {
	s, e := h.target()
	if e != nil {
		return e
	}
	return s.DeleteSession(ctx, id)
}
func (h *Hybrid) DeleteAllSessions(ctx context.Context) error {
	s, e := h.target()
	if e != nil {
		return e
	}
	return s.DeleteAllSessions(ctx)
}
func (h *Hybrid) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.db != nil {
		return h.db.Close()
	}
	return nil
}
