package app

import (
	"context"
	"strings"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// CoalesceEvents bounds how often the UI rebuilds its view.
//
// Bubble Tea calls Model.View() once per message, not once per frame — its FPS
// cap only throttles writes to the terminal. A provider streaming 100+ deltas a
// second therefore rebuilds the whole transcript 100+ times a second, and since
// the event loop is serialized, that work also delays keystroke echo and
// cancellation. Batching to a frame budget removes the cost without changing
// what the user sees: text still lands at the terminal's refresh rate, and the
// first token of a burst waits at most one interval.
//
// Two kinds of event merge, with different rules:
//   - content and reasoning deltas are appended; each carries only its own text;
//   - tool output snapshots are cumulative, so the newest replaces the last.
//
// Everything else is forwarded untouched, after flushing pending batches, so a
// lifecycle boundary can never overtake the text that belongs before it.
func CoalesceEvents(ctx context.Context, in <-chan domain.Event, interval time.Duration) <-chan domain.Event {
	if interval < 8*time.Millisecond {
		interval = 8 * time.Millisecond
	}
	out := make(chan domain.Event, 64)
	go func() {
		defer close(out)

		pending := make(map[string]*pendingEvent)
		order := make([]string, 0, 8)
		timer := time.NewTimer(interval)
		if !timer.Stop() {
			<-timer.C
		}
		active := false

		flush := func() bool {
			for _, key := range order {
				p, ok := pending[key]
				if !ok {
					continue
				}
				ev := p.materialize()
				select {
				case out <- ev:
				case <-ctx.Done():
					return false
				}
			}
			clear(pending)
			order = order[:0]
			active = false
			return true
		}

		forward := func(e domain.Event) bool {
			select {
			case out <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-in:
				if !ok {
					flush()
					return
				}

				key, mergeable := batchKey(e)
				if !mergeable {
					// Preserve ordering around boundaries: anything queued
					// belongs before this event.
					if len(order) > 0 && !flush() {
						return
					}
					if !forward(e) {
						return
					}
					continue
				}

				if p, exists := pending[key]; exists {
					p.merge(e)
				} else {
					pending[key] = newPending(e)
					order = append(order, key)
				}
				if !active {
					timer.Reset(interval)
					active = true
				}
			case <-timer.C:
				if !flush() {
					return
				}
			}
		}
	}()
	return out
}

// pendingEvent holds one merge batch. text is nil for cumulative events, where
// the newest value is the whole truth.
type pendingEvent struct {
	ev   domain.Event
	text *strings.Builder
}

func newPending(e domain.Event) *pendingEvent {
	p := &pendingEvent{ev: e}
	if isAppendMerged(e.Kind) {
		p.text = &strings.Builder{}
		p.text.WriteString(deltaText(e))
	}
	return p
}

func (p *pendingEvent) merge(e domain.Event) {
	if p.text == nil {
		p.ev = e
		return
	}
	if p.text.Len() < maxBatchBytes {
		p.text.WriteString(deltaText(e))
	}
}

// materialize rebuilds the event around the accumulated text.
func (p *pendingEvent) materialize() domain.Event {
	if p.text == nil {
		return p.ev
	}
	switch p.ev.Kind {
	case domain.EventStreamDelta:
		d, _ := p.ev.Data.(domain.StreamDelta)
		d.Text = p.text.String()
		p.ev.Data = d
	case domain.EventReasoningDelta:
		d, _ := p.ev.Data.(domain.ReasoningDelta)
		d.Text = p.text.String()
		p.ev.Data = d
	}
	return p.ev
}

// maxBatchBytes caps a single batch so a runaway stream cannot grow one event
// without bound; the remainder arrives in the next flush.
const maxBatchBytes = 256 << 10

func isAppendMerged(k domain.EventKind) bool {
	return k == domain.EventStreamDelta || k == domain.EventReasoningDelta
}

func deltaText(e domain.Event) string {
	switch e.Kind {
	case domain.EventStreamDelta:
		if d, ok := e.Data.(domain.StreamDelta); ok {
			return d.Text
		}
	case domain.EventReasoningDelta:
		if d, ok := e.Data.(domain.ReasoningDelta); ok {
			return d.Text
		}
	}
	return ""
}

// batchKey returns the merge identity of an event, or false if it must be
// forwarded immediately. Events that share a key are interchangeable within one
// flush window.
func batchKey(e domain.Event) (string, bool) {
	switch e.Kind {
	case domain.EventStreamDelta:
		d, ok := e.Data.(domain.StreamDelta)
		if !ok {
			return "", false
		}
		return "text\x00" + e.SessionID + "\x00" + d.MessageID, true
	case domain.EventReasoningDelta:
		d, ok := e.Data.(domain.ReasoningDelta)
		if !ok {
			return "", false
		}
		return "reason\x00" + e.SessionID + "\x00" + d.MessageID, true
	case domain.EventToolOutput:
		d, ok := e.Data.(domain.ToolEvent)
		if !ok {
			return "", false
		}
		return "tool\x00" + e.SessionID + "\x00" + e.RunID + "\x00" + d.Activity.ID, true
	}
	return "", false
}
