package storage

import (
	"context"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

type Store interface {
	ListSessions(ctx context.Context) ([]domain.Session, error)
	SearchSessions(ctx context.Context, query string, limit int) ([]domain.Session, error)
	LoadSession(ctx context.Context, id string) (domain.Session, error)
	SaveSession(ctx context.Context, session domain.Session) error
	// SessionMeta returns the session's mutable settings without loading its
	// transcript. A running turn reads the approval mode through this on every
	// round, so a change made while the turn runs is visible to it.
	SessionMeta(ctx context.Context, id string) (map[string]any, error)
	// UpdateSettings applies fn to a freshly loaded copy of the session and writes
	// back only the settings it can change (name, cwd, agent, model, metadata).
	// The transcript is never rewritten, so a settings change cannot drop messages
	// that a running turn just saved — the bug that a load/mutate/SaveSession pair
	// had, because SaveSession replaces every message row.
	UpdateSettings(ctx context.Context, id string, fn func(*domain.Session)) error
	DeleteSession(ctx context.Context, id string) error
	DeleteAllSessions(ctx context.Context) error
	Close() error
}
