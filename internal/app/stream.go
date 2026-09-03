package app

import (
	"context"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// CoalesceEvents keeps conversational deltas truly live while rate-limiting
// only noisy cumulative tool-output snapshots.
//
// Provider content/reasoning events are forwarded immediately. They are the
// latency-sensitive path users actually watch while a model is answering, so
// batching them here makes fast models look chunked even though the upstream
// connection is SSE. Tool output is different: ToolEvent.Output is a
// cumulative snapshot, so retaining only the newest snapshot in a short
// interval removes redundant redraws without hiding any bytes from the user.
func CoalesceEvents(ctx context.Context, in <-chan domain.Event, interval time.Duration) <-chan domain.Event {
	if interval < 8*time.Millisecond {
		interval = 8 * time.Millisecond
	}
	out := make(chan domain.Event, 64)
	go func() {
		defer close(out)

		pending := make(map[string]domain.Event)
		order := make([]string, 0, 8)
		timer := time.NewTimer(interval)
		if !timer.Stop() {
			<-timer.C
		}
		active := false

		flushTools := func() bool {
			for _, key := range order {
				e, ok := pending[key]
				if !ok {
					continue
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
					flushTools()
					return
				}

				// Content and reasoning are never coalesced. Forward each provider
				// delta immediately so time-to-first-visible-token is not tied to a
				// UI batching interval.
				if e.Kind == domain.EventStreamDelta || e.Kind == domain.EventReasoningDelta {
					// Keep global event order if a tool snapshot arrived just before
					// this delta. Flushing one cumulative snapshot does not introduce
					// a timer wait; the conversational delta is still forwarded in the
					// same receive iteration.
					if len(order) > 0 && !flushTools() {
						return
					}
					if !forward(e) {
						return
					}
					continue
				}

				// Tool output snapshots can be extremely noisy (for example compiler
				// output). They are cumulative, so only the newest snapshot per
				// activity is needed inside the small render interval.
				if e.Kind == domain.EventToolOutput {
					if d, ok := e.Data.(domain.ToolEvent); ok {
						key := e.SessionID + "\x00" + e.RunID + "\x00" + d.Activity.ID
						if _, exists := pending[key]; !exists {
							order = append(order, key)
						}
						pending[key] = e
						if !active {
							timer.Reset(interval)
							active = true
						}
						continue
					}
				}

				// Preserve event ordering around tool lifecycle boundaries: flush a
				// pending snapshot before forwarding a non-coalesced event.
				if len(order) > 0 && !flushTools() {
					return
				}
				if !forward(e) {
					return
				}
			case <-timer.C:
				if !flushTools() {
					return
				}
			}
		}
	}()
	return out
}
