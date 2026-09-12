package app

import (
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// TestCoalesceEventsForwardsWithoutMerging pins that approval events bypass the
// batcher entirely: they must not merge, not be delayed by an open batch
// window, and must arrive in order relative to each other. The response to a
// prompt travels by direct method call, but the prompt itself is an event — a
// held-back prompt would stall the whole tool round.
func TestCoalesceEventsForwardsWithoutMerging(t *testing.T) {
	in := make(chan domain.Event, 16)
	out := CoalesceEvents(t.Context(), in, 50*time.Millisecond)

	req1 := domain.Event{Kind: domain.EventApprovalRequested, SessionID: "s", Data: domain.ApprovalRequest{ID: "r1", ToolName: "bash"}}
	req2 := domain.Event{Kind: domain.EventApprovalRequested, SessionID: "s2", Data: domain.ApprovalRequest{ID: "r2", ToolName: "write_file"}}
	res1 := domain.Event{Kind: domain.EventApprovalResolved, SessionID: "s", Data: domain.ApprovalResolved{ID: "r1", Allowed: true}}

	// Open a batch window with a stream delta, then inject approval events
	// while it is still pending: a batching implementation would hold them.
	in <- domain.Event{Kind: domain.EventStreamDelta, SessionID: "s", Data: domain.StreamDelta{MessageID: "m1", Text: "hi"}}
	in <- req1
	in <- req2
	in <- res1
	close(in)

	deadline := time.After(3 * time.Second)
	var got []domain.Event
	for len(got) < 4 {
		select {
		case e, ok := <-out:
			if !ok {
				t.Fatalf("output closed after %d events", len(got))
			}
			got = append(got, e)
		case <-deadline:
			t.Fatalf("timed out; got %d events: %#v", len(got), got)
		}
	}
	// First event must be the flushed delta batch, then the approval events in
	// injection order, each intact and unmerged.
	if got[0].Kind != domain.EventStreamDelta {
		t.Fatalf("first event = %s, want the flushed stream batch", got[0].Kind)
	}
	for i, want := range []domain.Event{req1, req2, res1} {
		if got[i+1].Kind != want.Kind {
			t.Fatalf("event %d = %s, want %s", i+1, got[i+1].Kind, want.Kind)
		}
		// Data carries slices, so compare by identity fields instead of ==.
		switch w := want.Data.(type) {
		case domain.ApprovalRequest:
			g, ok := got[i+1].Data.(domain.ApprovalRequest)
			if !ok || g.ID != w.ID || g.ToolName != w.ToolName || g.SessionID != w.SessionID {
				t.Fatalf("event %d data = %#v, want %#v (merged or altered)", i+1, got[i+1].Data, w)
			}
		case domain.ApprovalResolved:
			g, ok := got[i+1].Data.(domain.ApprovalResolved)
			if !ok || g.ID != w.ID || g.Allowed != w.Allowed {
				t.Fatalf("event %d data = %#v, want %#v (merged or altered)", i+1, got[i+1].Data, w)
			}
		}
	}
}
