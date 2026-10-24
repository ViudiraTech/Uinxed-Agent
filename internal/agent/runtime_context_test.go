package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

// warnModel is a model name the window parser reads as 10k, which makes a real
// session small enough to cross the warning threshold inside a test.
const warnModel = "10k"

func testRuntime(t *testing.T) *Runtime {
	t.Helper()
	r := NewRuntime(newMemStore(), nil, func(string) (provider.Provider, error) { return &scriptedProvider{}, nil })
	t.Cleanup(r.Close)
	return r
}

// nextWarning returns the next context warning, failing if another event arrives
// first — the notice has to stand on its own, not hide behind a stream delta.
func nextWarning(t *testing.T, r *Runtime) domain.ContextWarning {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e := <-r.Events():
			if e.Kind != domain.EventContextWarning {
				t.Fatalf("event %s arrived before the warning", e.Kind)
			}
			w, ok := e.Data.(domain.ContextWarning)
			if !ok {
				t.Fatalf("payload = %T, want domain.ContextWarning", e.Data)
			}
			return w
		case <-timer.C:
			t.Fatal("timed out waiting for the context warning")
		}
	}
}

// The warning is a notice, not a heartbeat: once per fill-up. It re-arms only
// when a compaction empties the context, because that is the only thing that
// makes a second crossing worth mentioning.
func TestContextPressureWarnsOncePerFillUp(t *testing.T) {
	r := testRuntime(t)
	full := []domain.Message{{Role: domain.RoleUser, Content: strings.Repeat("a", 40_000)}}

	r.noteContextPressure(context.Background(), "s", warnModel, full, "run-1")
	w := nextWarning(t, r)
	if w.Percent < 50 {
		t.Fatalf("percent = %d, want at least the 50%% warning line", w.Percent)
	}
	if w.CompactPercent != 62 {
		t.Fatalf("compact percent = %d, want 62", w.CompactPercent)
	}
	if w.WarnAt != 5_000 || w.CompactAt != 6_200 {
		t.Fatalf("thresholds = %d/%d, want 5000/6200", w.WarnAt, w.CompactAt)
	}

	r.noteContextPressure(context.Background(), "s", warnModel, full, "run-1")
	select {
	case e := <-r.Events():
		t.Fatalf("a second warning for the same fill: %s", e.Kind)
	default:
	}

	r.rearmContextWarning("s")
	r.noteContextPressure(context.Background(), "s", warnModel, full, "run-1")
	nextWarning(t, r)
}

// A session with room says nothing; warning there would train the user to ignore
// the one that matters.
func TestContextPressureStaysQuietBelowTheThreshold(t *testing.T) {
	r := testRuntime(t)
	small := []domain.Message{{Role: domain.RoleUser, Content: "read the makefile"}}
	r.noteContextPressure(context.Background(), "s", warnModel, small, "run-1")
	select {
	case e := <-r.Events():
		t.Fatalf("warning for a session with room: %s", e.Kind)
	default:
	}
}

// The warning has to reach the UI from a real turn, before the model has
// answered: it is the only thing that explains why a compaction happened.
func TestTurnWarnsWhileTheContextIsFillingUp(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{
		ID: "s", Name: "s", ProviderID: "p", Model: warnModel, AgentID: "build", CWD: t.TempDir(),
		// 17000 latin characters estimate to roughly 5400 tokens: above the 5000
		// warning line, below the 6200 compaction one, so this turn warns without
		// compacting.
		Messages: []domain.Message{{Role: domain.RoleUser, Content: strings.Repeat("a", 17_000)}},
	}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return &scriptedProvider{}, nil })
	defer r.Close()
	if _, err := r.StartTurn(context.Background(), "s", "next"); err != nil {
		t.Fatal(err)
	}

	warnings := 0
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for done := false; !done; {
		select {
		case e := <-r.Events():
			switch e.Kind {
			case domain.EventContextWarning:
				warnings++
			case domain.EventCompaction:
				t.Fatalf("compaction ran at a size that should only warn: %v", e.Data)
			case domain.EventAgentFinished:
				done = true
			case domain.EventError:
				if d, ok := e.Data.(domain.ErrorData); ok {
					t.Fatalf("runtime error: %s", d.Message)
				}
			}
		case <-timer.C:
			t.Fatal("timed out waiting for the turn")
		}
	}
	if warnings != 1 {
		t.Fatalf("warnings = %d in one turn, want exactly 1", warnings)
	}
}
