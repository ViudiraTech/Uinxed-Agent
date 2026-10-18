package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// A burst larger than the batch cap must arrive whole: the cap used to drop the
// deltas that followed it, so the transcript lost text under a fast stream.
func TestCoalescerKeepsEveryByte(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan domain.Event, 32)
	// A long interval forces one batch window, which is what hits the cap.
	out := CoalesceEvents(ctx, in, time.Hour)

	chunk := strings.Repeat("x", 64<<10)
	const n = 8 // 512 KiB, well over the 256 KiB cap
	go func() {
		for i := 0; i < n; i++ {
			in <- domain.NewEvent(domain.EventStreamDelta, "s", domain.StreamDelta{MessageID: "m", Text: chunk})
		}
		close(in)
	}()

	var got strings.Builder
	for ev := range out {
		if d, ok := ev.Data.(domain.StreamDelta); ok {
			got.WriteString(d.Text)
		}
	}
	if got.Len() != n*len(chunk) {
		t.Fatalf("coalescer delivered %d of %d bytes", got.Len(), n*len(chunk))
	}
}
