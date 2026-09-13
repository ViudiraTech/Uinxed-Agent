package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/storage"
	"github.com/ViudiraTech/Uinxed-Agent/internal/tools"
)

// Decision is one answer to a pending approval prompt.
type Decision struct {
	Allow bool
	// Always records a per-session grant for this tool name, so the same tool
	// stops prompting for the rest of the session.
	Always bool
	Reason string
}

// request is one pending tool call. resp is buffered with capacity 1 so the
// resolver never blocks on a goroutine that has already given up.
type request struct {
	ID        string
	SessionID string
	RunID     string
	CallID    string
	Tool      string
	Args      json.RawMessage
	Category  tools.Category
	Summary   string
	resp      chan Decision
}

// Broker serialises approval prompts: exactly one request is presented at a
// time, in registration order, and every waiting tool goroutine is released
// either by a user decision or by cancellation.
//
// It lives in the agent package rather than the TUI because the blocking
// happens inside tool execution. The UI is a consumer of events and a caller of
// Resolve; it is never on the critical path for correctness.
type Broker struct {
	mu      sync.Mutex
	queue   []*request
	current *request
	byID    map[string]*request
	// grants maps a root session ID to the set of tool names the user allowed
	// for the rest of that session. In-memory only, never persisted.
	grants map[string]map[string]struct{}
	// emit publishes approval events. It must be safe to call with no lock
	// held; see the comment on register.
	emit func(domain.Event)
}

func NewBroker(emit func(domain.Event)) *Broker {
	if emit == nil {
		emit = func(domain.Event) {}
	}
	return &Broker{byID: map[string]*request{}, grants: map[string]map[string]struct{}{}, emit: emit}
}

// register enqueues a request and returns its ID.
//
// Constraint: registration happens on the caller's goroutine, before the
// goroutine that executes the tool is started, and executeCalls registers in
// call-index order. Launching the goroutines first and registering inside them
// would order the prompts by scheduler luck instead of by call order, which is
// exactly the non-determinism this design removes.
func (b *Broker) register(req *request) string {
	if req.resp == nil {
		req.resp = make(chan Decision, 1)
	}
	b.mu.Lock()
	b.byID[req.ID] = req
	b.queue = append(b.queue, req)
	_, ev := b.advanceLocked()
	b.mu.Unlock()
	// Emitting outside the lock is mandatory, not stylistic: the event channel
	// has a fixed capacity, and holding b.mu while blocked on it would stall
	// every other goroutine's register and forget behind the same mutex.
	if ev != nil {
		b.emit(*ev)
	}
	return req.ID
}

// advanceLocked promotes the next queued request to current when nothing is
// being presented. It returns the event describing the new prompt, if any.
// Callers must hold b.mu and must emit the returned event after unlocking.
func (b *Broker) advanceLocked() (*request, *domain.Event) {
	if b.current != nil || len(b.queue) == 0 {
		return nil, nil
	}
	req := b.queue[0]
	b.queue = b.queue[1:]
	b.current = req
	ev := domain.Event{
		Kind:      domain.EventApprovalRequested,
		SessionID: req.SessionID,
		RunID:     req.RunID,
		Data: domain.ApprovalRequest{
			ID:        req.ID,
			ToolName:  req.Tool,
			Category:  string(req.Category),
			Summary:   req.Summary,
			Arguments: req.Args,
			SessionID: req.SessionID,
			RunID:     req.RunID,
		},
	}
	return req, &ev
}

// wait blocks until the request is answered or ctx is cancelled. Cancellation
// (Esc / Ctrl+C tearing down the errgroup context) releases the goroutine
// without a decision, and the deferred forget cleans up the queue.
func (b *Broker) wait(ctx context.Context, req *request) (Decision, bool) {
	select {
	case d := <-req.resp:
		return d, true
	case <-ctx.Done():
		return Decision{}, false
	}
}

// forget removes a request from the broker and advances to the next prompt.
// It is idempotent: a resolved request was already removed from byID, and a
// request cancelled while still queued is dropped from the queue.
func (b *Broker) forget(id string) {
	if id == "" {
		return
	}
	b.mu.Lock()
	if _, ok := b.byID[id]; !ok {
		b.mu.Unlock()
		return
	}
	delete(b.byID, id)
	advanced := false
	if b.current != nil && b.current.ID == id {
		b.current = nil
		advanced = true
	}
	for i, q := range b.queue {
		if q.ID == id {
			b.queue = append(b.queue[:i], b.queue[i+1:]...)
			break
		}
	}
	var ev *domain.Event
	if advanced {
		_, ev = b.advanceLocked()
	}
	b.mu.Unlock()
	if ev != nil {
		b.emit(*ev)
	}
}

// Resolve answers a pending request. It reports whether the ID was still
// pending, so a UI answering a stale prompt can be ignored.
//
// The grant is recorded under the root session (see rootSessionID) so a
// delegate child inherits what its parent was already allowed to do.
func (b *Broker) Resolve(rootSessionID, reqID string, d Decision) bool {
	b.mu.Lock()
	req, ok := b.byID[reqID]
	if !ok {
		b.mu.Unlock()
		return false
	}
	if d.Allow && d.Always {
		if b.grants[rootSessionID] == nil {
			b.grants[rootSessionID] = map[string]struct{}{}
		}
		b.grants[rootSessionID][req.Tool] = struct{}{}
	}
	delete(b.byID, reqID)
	advanced := false
	if b.current != nil && b.current.ID == reqID {
		b.current = nil
		advanced = true
	}
	for i, q := range b.queue {
		if q.ID == reqID {
			b.queue = append(b.queue[:i], b.queue[i+1:]...)
			break
		}
	}
	var ev *domain.Event
	if advanced {
		_, ev = b.advanceLocked()
	}
	b.mu.Unlock()

	// Buffered with capacity 1: the waiter may already have abandoned this
	// request on cancellation, and that must not block the resolver.
	select {
	case req.resp <- d:
	default:
	}
	if ev != nil {
		b.emit(*ev)
	}
	b.emit(domain.Event{
		Kind:      domain.EventApprovalResolved,
		SessionID: req.SessionID,
		RunID:     req.RunID,
		Data:      domain.ApprovalResolved{ID: reqID, Allowed: d.Allow, Reason: d.Reason},
	})
	return true
}

// GrantsFor returns the tool names already allowed for a root session. The
// returned set is a copy and is safe to read without holding any lock.
func (b *Broker) GrantsFor(rootSessionID string) map[string]struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	src := b.grants[rootSessionID]
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(src))
	for k := range src {
		out[k] = struct{}{}
	}
	return out
}

// DropSession discards the grants for one root session. Called when a session
// is cleared or deleted so a reused ID cannot inherit stale authority.
func (b *Broker) DropSession(rootSessionID string) {
	b.mu.Lock()
	delete(b.grants, rootSessionID)
	b.mu.Unlock()
}

// pendingCount reports queued plus in-flight requests. Test and diagnostics
// only; it takes the lock from the ordinary path.
func (b *Broker) pendingCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(b.queue)
	if b.current != nil {
		n++
	}
	return n
}

// rootSessionID walks ParentID up to the topmost session.
//
// Authorization is stored at the root because delegates create child sessions:
// keying grants by the child would re-prompt for every tool the parent was
// already granted, every time a subagent is spawned. The walk is bounded so a
// cyclic ParentID (corrupt data) cannot hang the approval path.
func (r *Runtime) rootSessionID(ctx context.Context, sessionID string) string {
	const maxDepth = 32
	id := sessionID
	for i := 0; i < maxDepth; i++ {
		sess, err := r.store.LoadSession(ctx, id)
		if err != nil || sess.ParentID == "" {
			break
		}
		id = sess.ParentID
	}
	return id
}

// ResolveApproval answers a pending prompt for a run.
//
// It is the runtime half of the direct-call response path: the TUI calls it
// through Controller.ResolveApproval. Resolvability is decided by the broker's
// byID table alone. Checking the runs map here would look stricter but is
// wrong: delegate-tool children run their loop inline inside the parent's
// goroutine, so a child session has no runs entry and its prompts would be
// permanently unanswerable.
func (r *Runtime) ResolveApproval(runID, reqID string, d Decision) bool {
	if r.broker == nil {
		return false
	}
	_ = runID // accepted for interface symmetry; the request carries its own run
	root := r.broker.rootOf(reqID, r.store)
	if root == "" {
		return false
	}
	return r.broker.Resolve(root, reqID, d)
}

// rootOf reports the root session of the session that raised a request, or ""
// when the request is unknown. It is called with no broker lock held because
// the session walk performs store I/O.
func (b *Broker) rootOf(reqID string, store storage.Store) string {
	sessionID := b.sessionOf(reqID)
	if sessionID == "" {
		return ""
	}
	const maxDepth = 32
	id := sessionID
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for i := 0; i < maxDepth; i++ {
		sess, err := store.LoadSession(ctx, id)
		if err != nil || sess.ParentID == "" {
			break
		}
		id = sess.ParentID
	}
	return id
}

// sessionOf reports which session raised a request, or "" when the request is
// unknown. The broker cannot resolve a grant correctly without it.
func (b *Broker) sessionOf(reqID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	req, ok := b.byID[reqID]
	if !ok {
		return ""
	}
	return req.SessionID
}

// summarizeApproval renders a one-line preview of a tool call for the prompt.
// It is derived from the arguments rather than from the tool, so a new tool
// needs no rendering support to be approvable.
func summarizeApproval(tool string, args json.RawMessage) string {
	if tool == "switch_mode" {
		var a struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal(args, &a) == nil && a.Mode != "" {
			return "switch mode to " + a.Mode
		}
		return "switch mode"
	}
	var m map[string]any
	if len(args) == 0 || json.Unmarshal(args, &m) != nil {
		return ""
	}
	for _, key := range []string{"cmd", "path", "file_path", "query", "pattern", "task", "expr", "url"} {
		v, ok := m[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		if key == "path" || key == "file_path" {
			if extra, ok := m["edits"].([]any); ok && len(extra) > 0 {
				return fmt.Sprintf("%s (%d edits)", s, len(extra))
			}
		}
		return s
	}
	return ""
}
