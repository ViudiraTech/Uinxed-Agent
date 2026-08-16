package app

import (
	"context"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

type deltaKey struct {
	kind              domain.EventKind
	session, run, msg string
}
type pendingDelta struct {
	event domain.Event
	text  string
}

func CoalesceEvents(ctx context.Context, in <-chan domain.Event, interval time.Duration) <-chan domain.Event {
	if interval < 8*time.Millisecond {
		interval = 8 * time.Millisecond
	}
	out := make(chan domain.Event, 64)
	go func() {
		defer close(out)
		pending := map[deltaKey]*pendingDelta{}
		order := make([]deltaKey, 0, 16)
		timer := time.NewTimer(interval)
		if !timer.Stop() {
			<-timer.C
		}
		active := false
		flush := func() bool {
			for _, k := range order {
				p := pending[k]
				if p == nil {
					continue
				}
				e := p.event
				if e.Kind == domain.EventStreamDelta {
					d := e.Data.(domain.StreamDelta)
					d.Text = p.text
					e.Data = d
				} else if e.Kind == domain.EventReasoningDelta {
					d := e.Data.(domain.ReasoningDelta)
					d.Text = p.text
					e.Data = d
				}
				select {
				case out <- e:
				case <-ctx.Done():
					return false
				}
			}
			clear(pending)
			order = order[:0]
			active = false
			return true
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
				var k deltaKey
				merge := false
				switch d := e.Data.(type) {
				case domain.StreamDelta:
					if e.Kind == domain.EventStreamDelta {
						k = deltaKey{e.Kind, e.SessionID, e.RunID, d.MessageID}
						merge = true
					}
				case domain.ReasoningDelta:
					if e.Kind == domain.EventReasoningDelta {
						k = deltaKey{e.Kind, e.SessionID, e.RunID, d.MessageID}
						merge = true
					}
				case domain.ToolEvent:
					if e.Kind == domain.EventToolOutput {
						k = deltaKey{e.Kind, e.SessionID, e.RunID, d.Activity.ID}
						merge = true
					}
				}
				if merge {
					text := ""
					appendText := false
					if d, ok := e.Data.(domain.StreamDelta); ok {
						text = d.Text
						appendText = true
					}
					if d, ok := e.Data.(domain.ReasoningDelta); ok {
						text = d.Text
						appendText = true
					}
					if p := pending[k]; p != nil {
						if appendText {
							p.text += text
						} else {
							// Tool output events carry cumulative snapshots; keeping only the
							// newest one coalesces noisy stdout without losing content.
							p.event = e
						}
						p.event.At = e.At
					} else {
						pending[k] = &pendingDelta{event: e, text: text}
						order = append(order, k)
					}
					if !active {
						timer.Reset(interval)
						active = true
					}
					continue
				}
				if len(order) > 0 && !flush() {
					return
				}
				select {
				case out <- e:
				case <-ctx.Done():
					return
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
