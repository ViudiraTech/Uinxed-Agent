package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

// supercode used to be an effort level that forced max reasoning. As a toggle it
// stopped moving the request, so turning it on did nothing the existing level had
// not already done — the complaint that it "still feels like plain xhigh". The
// floor is what makes the mode mean what its panel says.
func TestEffectiveEffortRaisesSupercodeToXHigh(t *testing.T) {
	cases := []struct {
		name              string
		meta              map[string]any
		fallbackEffort    string
		fallbackSupercode bool
		want              string
	}{
		{"off keeps the level", map[string]any{"effort": "low"}, "high", false, "low"},
		{"off defaults to high", map[string]any{}, "high", false, "high"},
		{"on lifts low", map[string]any{"effort": "low", "supercode": true}, "high", false, "xhigh"},
		{"on lifts medium", map[string]any{"effort": "medium", "supercode": true}, "high", false, "xhigh"},
		{"on lifts high", map[string]any{"effort": "high", "supercode": true}, "high", false, "xhigh"},
		{"on keeps xhigh", map[string]any{"effort": "xhigh", "supercode": true}, "high", false, "xhigh"},
		{"on never downgrades max", map[string]any{"effort": "max", "supercode": true}, "high", false, "max"},
		{"on with no level", map[string]any{"supercode": true}, "high", false, "xhigh"},
		// The reported bug: a session with no "supercode" key at all (created
		// before the default was copied in) must still honour the config.
		{"config default turns it on", map[string]any{"effort": "low"}, "high", true, "xhigh"},
		{"session override wins over config", map[string]any{"supercode": false}, "high", true, "high"},
		{"no level falls back to config", map[string]any{}, "max", false, "max"},
	}
	for _, tc := range cases {
		if got := effectiveEffort(tc.meta, tc.fallbackEffort, tc.fallbackSupercode); got != tc.want {
			t.Errorf("%s: effectiveEffort = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// effortOrder duplicates the scale on purpose (internal/agent does not import
// config), so this pins the two together.
func TestEffortOrderMatchesConfig(t *testing.T) {
	levels := config.EffortLevels()
	if len(levels) != len(effortOrder) {
		t.Fatalf("effortOrder has %d levels, config has %d", len(effortOrder), len(levels))
	}
	for i := range levels {
		if levels[i] != effortOrder[i] {
			t.Fatalf("level %d: agent=%q config=%q", i, effortOrder[i], levels[i])
		}
	}
}

func TestSupercodePromptDemandsConcurrentDelegation(t *testing.T) {
	prompt := SystemPromptMode(Get("build"), "test-model", "", true, "auto-edit")
	for _, want := range []string{"Supercode 模式", "并发", "delegate", "汇总"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("supercode prompt missing %q", want)
		}
	}
}

// Plan mode is read-only and its registry has no delegate tool, so promising the
// model that it can fan work out would only make it call a tool it cannot see.
func TestSupercodePromptIsSkippedInPlanMode(t *testing.T) {
	prompt := SystemPromptMode(Get("plan"), "test-model", "", true, "plan")
	if strings.Contains(prompt, "Supercode 模式") {
		t.Fatal("plan mode must not carry the orchestration instructions")
	}
}

// supercodeChildProvider delegates once, then answers. The child round is round
// two; the parent's closing round is the third.
type supercodeChildProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *supercodeChildProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		emit := func(e provider.Event) {
			select {
			case ch <- e:
			case <-ctx.Done():
			}
		}
		if n == 1 {
			emit(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: "c-delegate", Type: "function",
				Function: domain.ToolCallFunction{Name: "delegate", Arguments: `{"agent":"explorer","task":"look around"}`},
			}}})
			emit(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		emit(provider.Event{Kind: provider.EventContent, Text: "done"})
		emit(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*supercodeChildProvider) Models(context.Context) ([]string, error) {
	return []string{"test"}, nil
}
func (*supercodeChildProvider) CheckKey(context.Context, string) error { return nil }

// The child inherits most of the parent's metadata, but not supercode: a child
// told it is in orchestration mode would delegate its own task again, and the
// fan-out would recurse instead of terminating.
func TestDelegateChildDoesNotInheritSupercode(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir(),
		Metadata: map[string]any{"mode": "full-auto", "supercode": true, "effort": "low"}}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return &supercodeChildProvider{}, nil })
	t.Cleanup(r.Close)

	if _, err := r.StartTurn(context.Background(), "s", "go delegate"); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, r, "s")

	list, err := st.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var child *domain.Session
	for i := range list {
		if list[i].ParentID == "s" {
			child = &list[i]
		}
	}
	if child == nil {
		t.Fatal("no child session was persisted")
	}
	if _, ok := child.Metadata["supercode"]; ok {
		t.Fatalf("child inherited supercode: %v", child.Metadata)
	}
	if child.Metadata["effort"] != "low" {
		t.Fatalf("child lost the parent's effort level: %v", child.Metadata)
	}
}
