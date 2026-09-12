package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/tools"
)

// newTestBroker wires a broker to a channel of the approval events it emits, so
// tests can observe presentation order without a TUI.
func newTestBroker(buffer int) (*Broker, <-chan domain.Event) {
	ch := make(chan domain.Event, buffer)
	b := NewBroker(func(e domain.Event) { ch <- e })
	return b, ch
}

func mkReq(id, tool string) *request {
	return &request{
		ID: id, SessionID: "s-root", RunID: "run-1", CallID: "call-" + id,
		Tool: tool, Args: []byte(`{}`), Category: tools.CategoryShell,
		resp: make(chan Decision, 1),
	}
}

// nextRequestEvent reads events until the next ApprovalRequest, skipping
// ApprovalResolved notifications that trail every resolution.
func nextRequestEvent(t *testing.T, events <-chan domain.Event) domain.ApprovalRequest {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-events:
			if req, ok := e.Data.(domain.ApprovalRequest); ok {
				return req
			}
		case <-deadline:
			t.Fatal("timed out waiting for an approval request event")
			return domain.ApprovalRequest{}
		}
	}
}

// TestBrokerPresentsRequestsInRegistrationOrder pins constraint 1: the prompt
// queue follows register order, one at a time, and each resolution promotes the
// next request.
func TestBrokerPresentsRequestsInRegistrationOrder(t *testing.T) {
	b, events := newTestBroker(8)
	b.register(mkReq("a1", "bash"))
	b.register(mkReq("a2", "bash"))
	b.register(mkReq("a3", "write_file"))

	if got := nextRequestEvent(t, events).ID; got != "a1" {
		t.Fatalf("first prompt = %q, want a1", got)
	}
	if !b.Resolve("s-root", "a1", Decision{Allow: true}) {
		t.Fatal("resolve a1 reported unknown")
	}
	if got := nextRequestEvent(t, events).ID; got != "a2" {
		t.Fatalf("second prompt = %q, want a2", got)
	}
	if !b.Resolve("s-root", "a2", Decision{Allow: false, Reason: "no"}) {
		t.Fatal("resolve a2 reported unknown")
	}
	if got := nextRequestEvent(t, events).ID; got != "a3" {
		t.Fatalf("third prompt = %q, want a3", got)
	}
	b.Resolve("s-root", "a3", Decision{Allow: true})
	if got := b.pendingCount(); got != 0 {
		t.Fatalf("pending=%d after resolving everything", got)
	}
}

// TestBrokerConcurrentRegistrationIsSafe exercises register from many goroutines
// at once. Presentation order is defined by register order, and register calls
// that race still leave the broker consistent; -race is the real assertion here.
func TestBrokerConcurrentRegistrationIsSafe(t *testing.T) {
	b, events := newTestBroker(64)
	const n = 12
	ids := make([]string, n)
	for i := range n {
		ids[i] = "cc" + string(rune('a'+i))
	}
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.register(mkReq(ids[i], "bash"))
		}()
	}
	wg.Wait()
	if got := b.pendingCount(); got != n {
		t.Fatalf("pending=%d, want %d", got, n)
	}
	// Only the head of the queue is ever current, so exactly one prompt event
	// exists no matter how many requests raced to register.
	if got := nextRequestEvent(t, events).ID; got == "" {
		t.Fatal("no prompt presented")
	}
	for _, id := range ids {
		b.Resolve("s-root", id, Decision{Allow: true})
	}
	if got := b.pendingCount(); got != 0 {
		t.Fatalf("pending=%d after resolving all", got)
	}
}

// TestBrokerCancelReleasesWaiterAndPromotesNext pins the cleanup path: a waiter
// abandoned by context cancellation must return, and forget must hand the queue
// to the next request. Run under -race for the memory-model half.
func TestBrokerCancelReleasesWaiterAndPromotesNext(t *testing.T) {
	b, events := newTestBroker(8)
	b.register(mkReq("c1", "bash"))
	b.register(mkReq("c2", "bash"))
	if got := nextRequestEvent(t, events).ID; got != "c1" {
		t.Fatalf("first prompt = %q, want c1", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, decided := b.wait(ctx, firstReq(b, "c1")); decided {
			t.Error("cancelled wait returned a decision")
		}
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not return after cancellation")
	}
	b.forget("c1")
	b.forget("c1") // idempotent
	// The second request must now be presented.
	if got := nextRequestEvent(t, events).ID; got != "c2" {
		t.Fatalf("after cancel, presented %q, want c2", got)
	}
	if got := b.pendingCount(); got != 1 {
		t.Fatalf("pending=%d, want 1", got)
	}
}

// firstReq recovers a registered request by ID. wait needs the *request, and in
// this test the goroutine that registered it is the test itself.
func firstReq(b *Broker, id string) *request {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.byID[id]
}

// TestBrokerDoesNotDeadlockWhenEmitBlocks pins constraint 3: emit happens
// outside broker.mu, so a wedged consumer must not stop other goroutines from
// registering, forgetting, or reading grants. Guarded with timeouts so a
// regression fails the test instead of hanging it.
func TestBrokerDoesNotDeadlockWhenEmitBlocks(t *testing.T) {
	emitEntered := make(chan struct{})
	release := make(chan struct{})
	once := sync.Once{}
	b := NewBroker(func(domain.Event) {
		// Simulate a saturated consumer: the first emit parks until released.
		once.Do(func() { close(emitEntered) })
		<-release
	})
	registerDone := make(chan struct{})
	go func() {
		defer close(registerDone)
		b.register(mkReq("d1", "bash"))
	}()
	select {
	case <-emitEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first register never reached the blocked emit")
	}
	// None of these may block on broker.mu while emit is parked.
	probes := make(chan string, 3)
	go func() { b.register(mkReq("d2", "bash")); probes <- "register" }()
	go func() { b.forget("d2"); probes <- "forget" }()
	go func() { b.GrantsFor("s-root"); probes <- "grants" }()
	for range 3 {
		select {
		case <-probes:
		case <-time.After(2 * time.Second):
			t.Fatal("broker mutex is pinned by a blocked emit: deadlock")
		}
	}
	close(release)
	select {
	case <-registerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first register never completed")
	}
}

// TestGrantsStoredByRootSession pins constraint 4: an "always allow" answer is
// recorded under the root session and is invisible to unrelated roots.
func TestGrantsStoredByRootSession(t *testing.T) {
	b, _ := newTestBroker(8)
	b.register(mkReq("g1", "bash"))
	if !b.Resolve("root-A", "g1", Decision{Allow: true, Always: true}) {
		t.Fatal("resolve reported unknown")
	}
	if _, ok := b.GrantsFor("root-A")["bash"]; !ok {
		t.Fatal("grant missing for root-A")
	}
	if g := b.GrantsFor("root-B"); g != nil {
		t.Fatalf("unrelated root inherited grants: %v", g)
	}
	b.DropSession("root-A")
	if g := b.GrantsFor("root-A"); g != nil {
		t.Fatalf("grants survived DropSession: %v", g)
	}
}

// TestGrantDoesNotApplyWithoutAlways checks that a one-shot allow records no
// grant, so the next call of the same tool asks again.
func TestGrantDoesNotApplyWithoutAlways(t *testing.T) {
	b, _ := newTestBroker(8)
	b.register(mkReq("h1", "bash"))
	b.Resolve("root", "h1", Decision{Allow: true})
	if g := b.GrantsFor("root"); len(g) != 0 {
		t.Fatalf("one-shot allow leaked a grant: %v", g)
	}
}

// TestResolveUnknownIDIsRejected covers stale prompts: an answer aimed at an
// already-gone request must be dropped, not delivered to whoever waits next.
func TestResolveUnknownIDIsRejected(t *testing.T) {
	b, _ := newTestBroker(8)
	if b.Resolve("root", "missing", Decision{Allow: true}) {
		t.Fatal("resolving an unknown ID reported success")
	}
	b.register(mkReq("k1", "bash"))
	b.Resolve("root", "k1", Decision{Allow: true})
	if b.Resolve("root", "k1", Decision{Allow: true}) {
		t.Fatal("double resolve reported success")
	}
}

// TestWaitReceivesDecision covers the happy path through wait.
func TestWaitReceivesDecision(t *testing.T) {
	b, _ := newTestBroker(8)
	req := mkReq("w1", "bash")
	b.register(req)
	go b.Resolve("root", "w1", Decision{Allow: true, Reason: "ok"})
	d, decided := b.wait(context.Background(), req)
	if !decided || !d.Allow || d.Reason != "ok" {
		t.Fatalf("wait = (%#v, %v)", d, decided)
	}
}
