package tools

import (
	"context"
	"fmt"
	"sync"
)

type Category string

const (
	CategoryRead     Category = "read"
	CategoryWrite    Category = "write"
	CategoryShell    Category = "shell"
	CategoryNetwork  Category = "network"
	CategoryDelegate Category = "delegate"
	CategoryState    Category = "state"
)

type Limits map[Category]int

func DefaultLimits() Limits {
	return Limits{CategoryRead: 8, CategoryWrite: 1, CategoryShell: 2, CategoryNetwork: 6, CategoryDelegate: 4, CategoryState: 4}
}

type Scheduler struct {
	fs   sync.RWMutex
	sems map[Category]chan struct{}
}

func NewScheduler(l Limits) *Scheduler {
	s := &Scheduler{sems: map[Category]chan struct{}{}}
	for c, n := range l {
		if n < 1 {
			n = 1
		}
		s.sems[c] = make(chan struct{}, n)
	}
	return s
}
func (s *Scheduler) Do(ctx context.Context, c Category, fn func(context.Context) error) error {
	sem := s.sems[c]
	if sem == nil {
		return fmt.Errorf("unknown scheduler category %s", c)
	}
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-sem }()
	switch c {
	case CategoryRead:
		s.fs.RLock()
		defer s.fs.RUnlock()
	case CategoryWrite, CategoryShell:
		s.fs.Lock()
		defer s.fs.Unlock()
	}
	return fn(ctx)
}
