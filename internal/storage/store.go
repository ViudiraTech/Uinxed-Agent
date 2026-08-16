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
	DeleteSession(ctx context.Context, id string) error
	DeleteAllSessions(ctx context.Context) error
	Close() error
}
