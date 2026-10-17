package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

// loopProvider keeps asking for a tool until it has done `rounds` rounds, then
// answers. It exists to prove the runtime no longer imposes a round budget.
type loopProvider struct {
	mu     sync.Mutex
	calls  int
	rounds int
}

func (p *loopProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		emit := func(e provider.Event) bool {
			select {
			case ch <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if n <= p.rounds {
			emit(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: fmt.Sprintf("c%d", n), Type: "function",
				Function: domain.ToolCallFunction{Name: "calc", Arguments: `{"expr":"1+1"}`},
			}}})
			emit(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		emit(provider.Event{Kind: provider.EventContent, Text: "done"})
		emit(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}

func (*loopProvider) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*loopProvider) CheckKey(context.Context, string) error   { return nil }

// A turn ends when the model stops asking for tools, not when a counter runs out.
// The old runtime stopped at 32 rounds, which cut long investigations in half and
// made the model believe it had been interrupted.
func TestTurnHasNoToolRoundCap(t *testing.T) {
	const rounds = 40 // above the 32 the runtime used to allow

	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(), ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir()}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	fp := &loopProvider{rounds: rounds}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return fp, nil })
	defer r.Close()

	if _, err := r.StartTurn(context.Background(), "s", "keep going"); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, r, "s")

	fp.mu.Lock()
	calls := fp.calls
	fp.mu.Unlock()
	if calls != rounds+1 {
		t.Fatalf("provider was called %d times, want %d (one per round plus the answer)", calls, rounds+1)
	}

	got, err := st.LoadSession(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	answered := false
	for _, m := range got.Messages {
		if m.Role == domain.RoleAssistant && m.Content == "done" {
			answered = true
		}
	}
	if !answered {
		t.Fatal("the turn never reached its final answer")
	}
}
