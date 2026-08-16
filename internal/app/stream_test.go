package app

import (
	"context"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"testing"
	"time"
)

func TestCoalesceEventsBatchesDeltasAndPreservesBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 16)
	out := CoalesceEvents(ctx, in, 8*time.Millisecond)
	for _, s := range []string{"a", "b", "c"} {
		in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: s}}
	}
	in <- domain.Event{Kind: domain.EventToolStarted, SessionID: "s", RunID: "r", Data: domain.ToolEvent{}}
	close(in)
	var got []domain.Event
	for e := range out {
		got = append(got, e)
	}
	if len(got) != 2 {
		t.Fatalf("events=%d %#v", len(got), got)
	}
	d, ok := got[0].Data.(domain.StreamDelta)
	if !ok || d.Text != "abc" {
		t.Fatalf("delta=%#v", got[0])
	}
	if got[1].Kind != domain.EventToolStarted {
		t.Fatalf("boundary=%s", got[1].Kind)
	}
}

func TestCoalesceEventsSeparatesReasoningAndContent(t *testing.T) {
	in := make(chan domain.Event, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := CoalesceEvents(ctx, in, 8*time.Millisecond)
	in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", RunID: "r", Data: domain.StreamDelta{MessageID: "m", Text: "answer"}}
	in <- domain.Event{Kind: domain.EventReasoningDelta, SessionID: "s", RunID: "r", Data: domain.ReasoningDelta{MessageID: "m", Text: "think"}}
	close(in)
	n := 0
	for range out {
		n++
	}
	if n != 2 {
		t.Fatalf("got %d events", n)
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
