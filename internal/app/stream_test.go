package app

import (
	"context"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// TestConversationDeltasAreBatchedWithinTheFrameBudget pins the contract that
// makes streaming cheap: many provider deltas reach the UI as one event, with
// every byte preserved. View() runs per message in Bubble Tea, so forwarding
// each delta individually costs a full transcript rebuild per token.
func TestConversationDeltasAreBatchedWithinTheFrameBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 8)
	out := CoalesceEvents(ctx, in, 20*time.Millisecond)

	for _, text := range []string{"a", "b", "c"} {
		in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: text}}
	}

	select {
	case e := <-out:
		d, ok := e.Data.(domain.StreamDelta)
		if !ok || d.Text != "abc" {
			t.Fatalf("batched delta = %#v, want %q", e.Data, "abc")
		}
	case <-time.After(time.Second):
		t.Fatal("batched delta never arrived")
	}
	// The batch must be delivered once, not once per input delta.
	select {
	case e := <-out:
		t.Fatalf("expected a single batched event, got a second: %#v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestReasoningAndContentBatchesStaySeparate keeps the two streams independent
// so collapsing reasoning cannot swallow answer text.
func TestReasoningAndContentBatchesStaySeparate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 4)
	out := CoalesceEvents(ctx, in, 20*time.Millisecond)

	in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: "answer"}}
	in <- domain.Event{Kind: domain.EventReasoningDelta, SessionID: "s", RunID: "r", Data: domain.ReasoningDelta{MessageID: "m", Text: "think"}}

	first := <-out
	second := <-out
	if first.Kind != domain.EventStreamDelta || second.Kind != domain.EventReasoningDelta {
		t.Fatalf("unexpected order: %s then %s", first.Kind, second.Kind)
	}
	firstDelta, _ := first.Data.(domain.StreamDelta)
	secondDelta, _ := second.Data.(domain.ReasoningDelta)
	if firstDelta.Text != "answer" || secondDelta.Text != "think" {
		t.Fatalf("batches crossed streams: %q / %q", firstDelta.Text, secondDelta.Text)
	}
}

// TestLifecycleBoundaryFlushesPendingText guards ordering: a turn must not be
// reported finished before the text it produced.
func TestLifecycleBoundaryFlushesPendingText(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 4)
	// Interval far longer than the test: only the boundary may release this.
	out := CoalesceEvents(ctx, in, time.Hour)

	in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: "partial"}}
	in <- domain.Event{Kind: domain.EventAgentFinished, SessionID: "s", RunID: "r", Data: domain.AgentEvent{}}

	first := <-out
	second := <-out
	if first.Kind != domain.EventStreamDelta {
		t.Fatalf("text must precede the lifecycle event, got %s first", first.Kind)
	}
	if d, _ := first.Data.(domain.StreamDelta); d.Text != "partial" {
		t.Fatalf("pending text was dropped at the boundary: %#v", first.Data)
	}
	if second.Kind != domain.EventAgentFinished {
		t.Fatalf("second event = %s, want %s", second.Kind, domain.EventAgentFinished)
	}
}

func TestCoalesceToolOutputKeepsLatestSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 8)
	out := CoalesceEvents(ctx, in, 8*time.Millisecond)
	base := domain.ToolActivity{ID: "tool-1", CallID: "call-1", Name: "bash", State: "running"}
	for _, text := range []string{"a", "ab", "abc"} {
		a := base
		a.Output = text
		in <- domain.Event{Kind: domain.EventToolOutput, SessionID: "s", RunID: "r", Data: domain.ToolEvent{Activity: a}}
	}
	close(in)
	var got []domain.Event
	for e := range out {
		got = append(got, e)
	}
	if len(got) != 1 {
		t.Fatalf("events=%d %#v", len(got), got)
	}
	tv, ok := got[0].Data.(domain.ToolEvent)
	if !ok || tv.Activity.Output != "abc" {
		t.Fatalf("tool output=%#v", got[0].Data)
	}
}

func TestPendingToolOutputFlushesBeforeLifecycleBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 4)
	out := CoalesceEvents(ctx, in, time.Second)
	activity := domain.ToolActivity{ID: "tool-1", CallID: "call-1", Name: "bash", Output: "latest", State: "running"}
	in <- domain.Event{Kind: domain.EventToolOutput, SessionID: "s", RunID: "r", Data: domain.ToolEvent{Activity: activity}}
	in <- domain.Event{Kind: domain.EventToolFinished, SessionID: "s", RunID: "r", Data: domain.ToolEvent{Activity: activity}}
	close(in)

	first := <-out
	second := <-out
	if first.Kind != domain.EventToolOutput || second.Kind != domain.EventToolFinished {
		t.Fatalf("unexpected order: %s then %s", first.Kind, second.Kind)
	}
}
