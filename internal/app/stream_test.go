package app

import (
	"context"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func TestConversationDeltasAreForwardedImmediatelyAndIndividually(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 8)
	out := CoalesceEvents(ctx, in, 5*time.Second)

	in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: "a"}}
	in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: "b"}}

	for i, want := range []string{"a", "b"} {
		select {
		case e := <-out:
			d, ok := e.Data.(domain.StreamDelta)
			if !ok || d.Text != want {
				t.Fatalf("event %d = %#v, want delta %q", i, e, want)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("conversation delta %d was delayed by coalescing interval", i)
		}
	}
}

func TestReasoningDeltaIsNotBatchedBehindContent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 4)
	out := CoalesceEvents(ctx, in, 5*time.Second)

	in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: "answer"}}
	in <- domain.Event{Kind: domain.EventReasoningDelta, SessionID: "s", RunID: "r", Data: domain.ReasoningDelta{MessageID: "m", Text: "think"}}

	first := <-out
	second := <-out
	if first.Kind != domain.EventStreamDelta || second.Kind != domain.EventReasoningDelta {
		t.Fatalf("unexpected order: %s then %s", first.Kind, second.Kind)
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
