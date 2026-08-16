package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	contextmgr "github.com/ViudiraTech/Uinxed-Agent/internal/context"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
	"github.com/ViudiraTech/Uinxed-Agent/internal/skills"
	"github.com/ViudiraTech/Uinxed-Agent/internal/storage"
	"github.com/ViudiraTech/Uinxed-Agent/internal/tools"
	"golang.org/x/sync/errgroup"
)

type ProviderResolver func(providerID string) (provider.Provider, error)

type Runtime struct {
	store         storage.Store
	registry      *tools.Registry
	resolve       ProviderResolver
	events        chan domain.Event
	mu            sync.Mutex
	runs          map[string]*runHandle
	seq           atomic.Uint64
	maxToolRounds int
	wg            sync.WaitGroup
	closed        bool
	log           *slog.Logger
}

type runHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func NewRuntime(store storage.Store, registry *tools.Registry, resolver ProviderResolver) *Runtime {
	if registry == nil {
		registry = tools.DefaultRegistry()
	}
	return &Runtime{store: store, registry: registry, resolve: resolver, events: make(chan domain.Event, 256), runs: map[string]*runHandle{}, maxToolRounds: 32}
}
func (r *Runtime) Events() <-chan domain.Event { return r.events }
func (r *Runtime) SetLogger(log *slog.Logger)  { r.mu.Lock(); r.log = log; r.mu.Unlock() }
func (r *Runtime) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		r.wg.Wait()
		return
	}
	r.closed = true
	cancels := make([]context.CancelFunc, 0, len(r.runs))
	for _, h := range r.runs {
		cancels = append(cancels, h.cancel)
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	r.wg.Wait()
	r.mu.Lock()
	r.runs = map[string]*runHandle{}
	r.mu.Unlock()
}

func (r *Runtime) StartTurn(parent context.Context, sessionID, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("empty prompt")
	}
	// Validate the session before reporting a successful start, and capture the
	// agent identity so lifecycle events can be emitted even if provider setup
	// fails before runTurn reaches the model loop.
	sess, err := r.store.LoadSession(parent, sessionID)
	if err != nil {
		return "", err
	}
	agentID := sess.AgentID
	if agentID == "" {
		agentID = "build"
	}
	runID := r.id("run")
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return "", errors.New("agent runtime is closed")
	}
	if old := r.runs[sessionID]; old != nil {
		r.mu.Unlock()
		cancel()
		return "", errors.New("session already has a running operation")
	}
	handle := &runHandle{cancel: cancel, done: make(chan struct{})}
	r.runs[sessionID] = handle
	r.wg.Add(1)
	log := r.log
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		defer close(handle.done)
		run := domain.AgentRun{ID: runID, SessionID: sessionID, AgentID: agentID, State: "running", StartedAt: time.Now()}
		r.emit(ctx, domain.Event{Kind: domain.EventAgentStarted, SessionID: sessionID, RunID: runID, At: time.Now(), Data: domain.AgentEvent{Run: run}})
		if log != nil {
			log.Info("agent started", "run_id", runID, "session_id", sessionID, "agent", agentID)
		}

		err := r.runTurn(ctx, sessionID, text, runID)

		// Release the per-session run slot before publishing terminal events so a
		// slow UI consumer cannot keep cancellation/state artificially locked.
		r.mu.Lock()
		delete(r.runs, sessionID)
		r.mu.Unlock()
		cancel()

		if err != nil && !errors.Is(err, context.Canceled) {
			r.emitTerminal(domain.Event{Kind: domain.EventError, SessionID: sessionID, RunID: runID, At: time.Now(), Data: domain.ErrorData{Op: "agent", Message: err.Error()}})
		}
		run.FinishedAt = time.Now()
		switch {
		case errors.Is(err, context.Canceled):
			run.State = "cancelled"
		case err != nil:
			run.State = "failed"
		default:
			run.State = "done"
		}
		r.emitTerminal(domain.Event{Kind: domain.EventAgentFinished, SessionID: sessionID, RunID: runID, At: time.Now(), Data: domain.AgentEvent{Run: run}})
		if log != nil {
			attrs := []any{"run_id", runID, "session_id", sessionID, "agent", agentID, "state", run.State, "duration_ms", run.FinishedAt.Sub(run.StartedAt).Milliseconds()}
			if err != nil {
				attrs = append(attrs, "error", err.Error())
			}
			log.Info("agent finished", attrs...)
		}
	}()
	return runID, nil
}

func (r *Runtime) Cancel(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h := r.runs[sessionID]; h != nil {
		h.cancel()
		return true
	}
	return false
}

// CancelAndWait cancels the active turn for one session and waits until that
// run has fully exited. It is used before destructive session operations so a
// cancelled goroutine cannot persist the session again after deletion.
func (r *Runtime) CancelAndWait(ctx context.Context, sessionID string) (bool, error) {
	r.mu.Lock()
	h := r.runs[sessionID]
	if h != nil {
		h.cancel()
	}
	r.mu.Unlock()
	if h == nil {
		return false, nil
	}
	select {
	case <-h.done:
		return true, nil
	case <-ctx.Done():
		return true, ctx.Err()
	}
}

type turnState struct {
	mu               sync.Mutex
	s                domain.Session
	referenceContext string
}

func (r *Runtime) runTurn(ctx context.Context, sessionID, text, runID string) error {
	sess, err := r.store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.AgentID == "" {
		sess.AgentID = "build"
	}
	p, err := r.resolve(sess.ProviderID)
	if err != nil {
		return err
	}
	if contextmgr.EstimateMessages(sess.Messages) >= contextmgr.CompactThreshold(sess.Model) {
		if err := r.compact(ctx, p, &sess, runID); err != nil {
			return err
		}
	}
	now := time.Now()
	referenceContext := r.expandFileReferences(ctx, sess, text)
	sess.Messages = append(sess.Messages, domain.Message{ID: r.id("msg"), Role: domain.RoleUser, Content: text, CreatedAt: now})
	sess.UpdatedAt = now
	if err := r.store.SaveSession(ctx, sess); err != nil {
		return err
	}
	st := &turnState{s: sess, referenceContext: referenceContext}
	err = r.loop(ctx, p, st, runID)
	st.mu.Lock()
	final := st.s.Clone()
	st.mu.Unlock()
	final.UpdatedAt = time.Now()
	saveErr := r.saveFinal(final)
	if err == nil && saveErr != nil {
		return fmt.Errorf("save final session: %w", saveErr)
	}
	return err
}

func (r *Runtime) loop(ctx context.Context, p provider.Provider, st *turnState, runID string) error {
	for round := 0; round < r.maxToolRounds; round++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		st.mu.Lock()
		sess := st.s.Clone()
		st.mu.Unlock()
		adef := Get(sess.AgentID)
		sys := SystemPrompt(adef, sess.Model, skills.PromptBlock(sess.CWD), effortFromMetadata(sess.Metadata))
		history := contextmgr.FitMessages(sess.Messages, contextmgr.HistoryBudget(sess.Model))
		msgs := make([]domain.Message, 0, len(history)+1)
		msgs = append(msgs, domain.Message{Role: domain.RoleSystem, Content: sys})
		msgs = append(msgs, history...)
		if st.referenceContext != "" {
			msgs = append(msgs, domain.Message{Role: domain.RoleUser, Content: st.referenceContext})
		}
		defs := r.registry.ProviderDefinitions(adef.ToolAllowed)
		req := provider.Request{Model: sess.Model, Messages: msgs, Tools: defs, Effort: effortFromMetadata(sess.Metadata), Thinking: thinkingFromMetadata(sess.Metadata)}
		stream, err := p.Stream(ctx, req)
		if err != nil {
			return err
		}
		message := domain.Message{ID: r.id("msg"), Role: domain.RoleAssistant, CreatedAt: time.Now()}
		var usage domain.Usage
		var acc toolAccumulator
		done := false
		for ev := range stream {
			switch ev.Kind {
			case provider.EventContent:
				message.Content += ev.Text
				r.emit(ctx, domain.Event{Kind: domain.EventStreamDelta, SessionID: sess.ID, RunID: runID, At: time.Now(), Data: domain.StreamDelta{MessageID: message.ID, Text: ev.Text}})
			case provider.EventReasoning:
				message.ReasoningContent += ev.Text
				r.emit(ctx, domain.Event{Kind: domain.EventReasoningDelta, SessionID: sess.ID, RunID: runID, At: time.Now(), Data: domain.ReasoningDelta{MessageID: message.ID, Text: ev.Text}})
			case provider.EventToolCall:
				acc.Add(ev.ToolCalls)
			case provider.EventUsage:
				usage = mergeUsage(usage, ev.Usage)
				r.emit(ctx, domain.NewEvent(domain.EventUsageChanged, sess.ID, usage))
			case provider.EventDone:
				done = true
			case provider.EventError:
				if ev.Err != nil {
					return ev.Err
				}
			}
		}
		message.ToolCalls = acc.Calls()
		if message.Content != "" || message.ReasoningContent != "" || len(message.ToolCalls) > 0 {
			st.mu.Lock()
			st.s.Messages = append(st.s.Messages, message)
			st.s.UpdatedAt = time.Now()
			st.mu.Unlock()
		}
		if len(message.ToolCalls) == 0 {
			if !done && message.Content == "" {
				return errors.New("provider stream ended without content or completion")
			}
			return nil
		}
		if err := r.executeCalls(ctx, st, runID, message.ToolCalls, adef); err != nil {
			return err
		}
		st.mu.Lock()
		snap := st.s.Clone()
		st.mu.Unlock()
		if err := r.store.SaveSession(ctx, snap); err != nil {
			return err
		}
	}
	return fmt.Errorf("tool round limit (%d) exceeded", r.maxToolRounds)
}

type toolAccumulator struct{ calls []domain.ToolCall }

func (a *toolAccumulator) Add(delta []domain.ToolCall) {
	for i, d := range delta {
		idx := -1
		if d.Index >= 0 && d.Index < len(a.calls) {
			idx = d.Index
		}
		if d.ID != "" {
			for j := range a.calls {
				if a.calls[j].ID == d.ID {
					idx = j
					break
				}
			}
		}
		if idx < 0 && i < len(a.calls) && d.ID == "" {
			idx = i
		}
		if idx < 0 {
			target := d.Index
			if target < 0 {
				target = len(a.calls)
			}
			for len(a.calls) <= target {
				a.calls = append(a.calls, domain.ToolCall{Index: len(a.calls), Type: "function"})
			}
			idx = target
		}
		if d.ID != "" {
			a.calls[idx].ID = d.ID
		}
		if d.Function.Name != "" {
			a.calls[idx].Function.Name += d.Function.Name
		}
		if d.Function.Arguments != "" {
			a.calls[idx].Function.Arguments += d.Function.Arguments
		}
	}
}
func (a *toolAccumulator) Calls() []domain.ToolCall {
	return append([]domain.ToolCall(nil), a.calls...)
}

func (r *Runtime) executeCalls(ctx context.Context, st *turnState, runID string, calls []domain.ToolCall, adef domain.AgentDefinition) error {
	type outcome struct {
		res tools.Result
		err error
		act domain.ToolActivity
	}
	results := make([]outcome, len(calls))
	g, gctx := errgroup.WithContext(ctx)
	for i, call := range calls {
		i, call := i, call
		g.Go(func() error {
			if !adef.ToolAllowed(call.Function.Name) {
				results[i].err = fmt.Errorf("agent %s is not allowed to use %s", adef.ID, call.Function.Name)
				return nil
			}
			args := json.RawMessage(call.Function.Arguments)
			if len(args) == 0 {
				args = []byte("{}")
			}
			act := domain.ToolActivity{ID: r.id("tool"), CallID: call.ID, Name: call.Function.Name, Arguments: append([]byte(nil), args...), State: "running", StartedAt: time.Now()}
			results[i].act = act
			r.emit(gctx, domain.Event{Kind: domain.EventToolStarted, SessionID: st.s.ID, RunID: runID, At: time.Now(), Data: domain.ToolEvent{Activity: act}})
			env := tools.ExecutionContext{}
			st.mu.Lock()
			env.CWD = st.s.CWD
			st.mu.Unlock()
			var liveMu sync.Mutex
			var live strings.Builder
			env.OnOutput = func(stream, text string) {
				liveMu.Lock()
				if live.Len() < 128<<10 {
					remain := (128 << 10) - live.Len()
					if len(text) > remain {
						live.WriteString(text[:remain])
						live.WriteString("\n…[live preview truncated]")
					} else {
						live.WriteString(text)
					}
				}
				a := act
				a.Output = live.String()
				liveMu.Unlock()
				r.emit(gctx, domain.Event{Kind: domain.EventToolOutput, SessionID: st.s.ID, RunID: runID, At: time.Now(), Data: domain.ToolEvent{Activity: a}})
			}
			env.Callbacks = tools.RuntimeCallbacks{
				TodoWrite: func(c context.Context, raw json.RawMessage) (tools.Result, error) {
					return r.todoWrite(c, st, runID, raw)
				},
				TodoUpdate: func(c context.Context, raw json.RawMessage) (tools.Result, error) {
					return r.todoUpdate(c, st, runID, raw)
				},
				Delegate: func(c context.Context, raw json.RawMessage) (tools.Result, error) {
					return r.delegate(c, st, runID, raw)
				},
			}
			if r.log != nil {
				r.log.Debug("tool started", "run_id", runID, "session_id", st.s.ID, "tool", call.Function.Name, "call_id", call.ID)
			}
			res, err := r.registry.Execute(gctx, call.Function.Name, args, env)
			act.EndedAt = time.Now()
			act.State = "success"
			if err != nil {
				act.State = "failed"
				act.Error = err.Error()
			}
			act.Output = res.Content
			act.ExitCode = res.ExitCode
			results[i] = outcome{res: res, err: err, act: act}
			r.emitTerminal(domain.Event{Kind: domain.EventToolFinished, SessionID: st.s.ID, RunID: runID, At: time.Now(), Data: domain.ToolEvent{Activity: act}})
			if r.log != nil {
				attrs := []any{"run_id", runID, "session_id", st.s.ID, "tool", call.Function.Name, "call_id", call.ID, "state", act.State, "duration_ms", act.EndedAt.Sub(act.StartedAt).Milliseconds()}
				if err != nil {
					attrs = append(attrs, "error", err.Error())
				}
				r.log.Debug("tool finished", attrs...)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, o := range results {
		st.s.ToolActivities = append(st.s.ToolActivities, o.act)
		content := o.res.JSON()
		if o.err != nil {
			content = `{"error":` + quote(o.err.Error()) + `}`
		}
		st.s.Messages = append(st.s.Messages, domain.Message{ID: r.id("msg"), Role: domain.RoleTool, ToolCallID: calls[i].ID, Name: calls[i].Function.Name, Content: content, CreatedAt: time.Now()})
	}
	st.s.UpdatedAt = time.Now()
	return nil
}

func (r *Runtime) todoWrite(ctx context.Context, st *turnState, runID string, raw json.RawMessage) (tools.Result, error) {
	var a struct {
		Todos []struct {
			Subject string            `json:"subject"`
			Status  domain.TodoStatus `json:"status"`
		} `json:"todos"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return tools.Result{}, err
	}
	now := time.Now()
	var todos []domain.Todo
	for _, t := range a.Todos {
		if strings.TrimSpace(t.Subject) == "" {
			continue
		}
		if t.Status == "" {
			t.Status = domain.TodoPending
		}
		todos = append(todos, domain.Todo{ID: r.id("todo"), Subject: t.Subject, Status: t.Status, UpdatedAt: now})
	}
	st.mu.Lock()
	st.s.Todos = todos
	sid := st.s.ID
	st.mu.Unlock()
	r.emit(ctx, domain.Event{Kind: domain.EventTodoChanged, SessionID: sid, RunID: runID, At: now, Data: append([]domain.Todo(nil), todos...)})
	return tools.Result{Content: fmt.Sprintf("%d todos", len(todos))}, nil
}
func (r *Runtime) todoUpdate(ctx context.Context, st *turnState, runID string, raw json.RawMessage) (tools.Result, error) {
	var a struct {
		Index   int               `json:"index"`
		Subject string            `json:"subject"`
		Status  domain.TodoStatus `json:"status"`
		Reason  string            `json:"reason"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return tools.Result{}, err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	idx := -1
	if a.Index > 0 && a.Index <= len(st.s.Todos) {
		idx = a.Index - 1
	} else if a.Subject != "" {
		for i, t := range st.s.Todos {
			if t.Subject == a.Subject {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return tools.Result{}, errors.New("todo not found")
	}
	st.s.Todos[idx].Status = a.Status
	st.s.Todos[idx].Reason = a.Reason
	st.s.Todos[idx].UpdatedAt = time.Now()
	copyTodos := append([]domain.Todo(nil), st.s.Todos...)
	sid := st.s.ID
	r.emit(ctx, domain.Event{Kind: domain.EventTodoChanged, SessionID: sid, RunID: runID, At: time.Now(), Data: copyTodos})
	return tools.Result{Content: "todo updated"}, nil
}

func (r *Runtime) delegate(ctx context.Context, parent *turnState, parentRunID string, raw json.RawMessage) (tools.Result, error) {
	var a struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return tools.Result{}, err
	}
	def := Get(a.Agent)
	if !def.CanSubagent() {
		return tools.Result{}, fmt.Errorf("%s is not a subagent", a.Agent)
	}
	parent.mu.Lock()
	ps := parent.s.Clone()
	parent.mu.Unlock()
	now := time.Now()
	child := domain.Session{ID: r.id("sub"), Name: a.Agent + ": " + truncateName(a.Task), CreatedAt: now, UpdatedAt: now, ProviderID: ps.ProviderID, Model: ps.Model, AgentID: a.Agent, CWD: ps.CWD, ParentID: ps.ID, Metadata: cloneMeta(ps.Metadata)}
	if err := r.store.SaveSession(ctx, child); err != nil {
		return tools.Result{}, err
	}
	p, err := r.resolve(child.ProviderID)
	if err != nil {
		return tools.Result{}, err
	}
	child.Messages = append(child.Messages, domain.Message{ID: r.id("msg"), Role: domain.RoleUser, Content: a.Task, CreatedAt: now})
	st := &turnState{s: child}
	subRun := r.id("run")
	run := domain.AgentRun{ID: subRun, ParentID: parentRunID, SessionID: child.ID, AgentID: a.Agent, Task: a.Task, State: "running", StartedAt: now}
	r.emit(ctx, domain.Event{Kind: domain.EventAgentStarted, SessionID: child.ID, RunID: subRun, At: now, Data: domain.AgentEvent{Run: run}})
	err = r.loop(ctx, p, st, subRun)
	st.mu.Lock()
	fin := st.s.Clone()
	st.mu.Unlock()
	fin.UpdatedAt = time.Now()
	saveErr := r.saveFinal(fin)
	if err == nil && saveErr != nil {
		err = fmt.Errorf("save child session: %w", saveErr)
	}
	run.FinishedAt = time.Now()
	switch {
	case errors.Is(err, context.Canceled):
		run.State = "cancelled"
	case err != nil:
		run.State = "failed"
	default:
		run.State = "done"
	}
	r.emitTerminal(domain.Event{Kind: domain.EventAgentFinished, SessionID: child.ID, RunID: subRun, At: time.Now(), Data: domain.AgentEvent{Run: run}})
	if err != nil {
		return tools.Result{}, err
	}
	answer := ""
	for i := len(fin.Messages) - 1; i >= 0; i-- {
		if fin.Messages[i].Role == domain.RoleAssistant && fin.Messages[i].Content != "" {
			answer = fin.Messages[i].Content
			break
		}
	}
	return tools.Result{Content: answer, Metadata: map[string]any{"session_id": child.ID, "agent": a.Agent}}, nil
}

func (r *Runtime) compact(ctx context.Context, p provider.Provider, sess *domain.Session, runID string) error {
	started := time.Now()
	if r.log != nil {
		r.log.Info("context compaction started", "run_id", runID, "session_id", sess.ID, "messages", len(sess.Messages))
	}
	r.emit(ctx, domain.Event{Kind: domain.EventCompaction, SessionID: sess.ID, RunID: runID, At: time.Now(), Data: "started"})
	req := provider.Request{Model: sess.Model, Messages: contextmgr.CompactionConversation(sess.Messages)}
	ch, err := p.Stream(ctx, req)
	if err != nil {
		return err
	}
	var summary strings.Builder
	for ev := range ch {
		switch ev.Kind {
		case provider.EventContent:
			summary.WriteString(ev.Text)
		case provider.EventError:
			if ev.Err != nil {
				return ev.Err
			}
		}
	}
	if strings.TrimSpace(summary.String()) == "" {
		return errors.New("compaction returned empty summary")
	}
	sess.Messages = contextmgr.ReplaceWithSummary(summary.String())
	sess.UpdatedAt = time.Now()
	if err := r.store.SaveSession(ctx, *sess); err != nil {
		return err
	}
	r.emit(ctx, domain.Event{Kind: domain.EventCompaction, SessionID: sess.ID, RunID: runID, At: time.Now(), Data: "completed"})
	if r.log != nil {
		r.log.Info("context compaction finished", "run_id", runID, "session_id", sess.ID, "duration_ms", time.Since(started).Milliseconds(), "messages", len(sess.Messages))
	}
	return nil
}

func (r *Runtime) emit(ctx context.Context, e domain.Event) {
	select {
	case r.events <- e:
	case <-ctx.Done():
	}
}

func (r *Runtime) emitTerminal(e domain.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	r.emit(ctx, e)
}

func (r *Runtime) saveFinal(sess domain.Session) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := r.store.SaveSession(ctx, sess)
	if err != nil && r.log != nil {
		r.log.Warn("final session persistence failed", "session_id", sess.ID, "error", err)
	}
	return err
}
func (r *Runtime) id(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixMilli(), r.seq.Add(1))
}
func mergeUsage(a, b domain.Usage) domain.Usage {
	if b.InputTokens != 0 {
		a.InputTokens = b.InputTokens
	}
	if b.OutputTokens != 0 {
		a.OutputTokens = b.OutputTokens
	}
	if b.TotalTokens != 0 {
		a.TotalTokens = b.TotalTokens
	} else {
		a.TotalTokens = a.InputTokens + a.OutputTokens
	}
	return a
}
func quote(s string) string { b, _ := json.Marshal(s); return string(b) }
func truncateName(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 48 {
		return string(r[:48]) + "…"
	}
	return s
}
func cloneMeta(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func effortFromMetadata(m map[string]any) string {
	if v, ok := m["effort"].(string); ok && v != "" {
		return v
	}
	return "high"
}
func thinkingFromMetadata(m map[string]any) bool {
	if v, ok := m["thinking"].(bool); ok {
		return v
	}
	return true
}

func (r *Runtime) Compact(ctx context.Context, sessionID string) error {
	r.mu.Lock()
	if r.runs[sessionID] != nil {
		r.mu.Unlock()
		return errors.New("cannot compact while generation is running")
	}
	r.mu.Unlock()
	sess, err := r.store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	p, err := r.resolve(sess.ProviderID)
	if err != nil {
		return err
	}
	return r.compact(ctx, p, &sess, r.id("compact"))
}

func (r *Runtime) IsRunningAny() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs) > 0
}

// StartSubagent creates an isolated child session and starts a direct @agent task.
// It is the interactive counterpart of the delegate tool: the caller can switch
// into the returned child session immediately while it runs.
func (r *Runtime) StartSubagent(parent context.Context, parentSessionID, agentID, task string) (domain.Session, string, error) {
	def := Get(agentID)
	if !def.CanSubagent() {
		return domain.Session{}, "", fmt.Errorf("%s is not a subagent", agentID)
	}
	if strings.TrimSpace(task) == "" {
		return domain.Session{}, "", errors.New("subagent task is empty")
	}
	ps, err := r.store.LoadSession(parent, parentSessionID)
	if err != nil {
		return domain.Session{}, "", err
	}
	now := time.Now()
	child := domain.Session{
		ID: r.id("sub"), Name: agentID + ": " + truncateName(task), CreatedAt: now, UpdatedAt: now,
		ProviderID: ps.ProviderID, Model: ps.Model, AgentID: agentID, CWD: ps.CWD,
		ParentID: ps.ID, Metadata: cloneMeta(ps.Metadata),
	}
	if err := r.store.SaveSession(parent, child); err != nil {
		return domain.Session{}, "", err
	}
	runID, err := r.StartTurn(parent, child.ID, task)
	if err != nil {
		_ = r.store.DeleteSession(parent, child.ID)
		return domain.Session{}, "", err
	}
	return child, runID, nil
}

// expandFileReferences turns @path mentions into bounded ephemeral context.
// The returned context is sent to the provider for every tool round in the turn,
// but is deliberately not persisted or shown as the user's message. It ignores
// @agent and @skill: mentions. Reads still pass through the sandboxed read_file tool.
func (r *Runtime) expandFileReferences(ctx context.Context, sess domain.Session, text string) string {
	fields := strings.Fields(text)
	seen := map[string]bool{}
	var blocks []string
	for _, f := range fields {
		if !strings.HasPrefix(f, "@") {
			continue
		}
		ref := strings.Trim(strings.TrimPrefix(f, "@"), "`'\".,;:()[]{}")
		if ref == "" || strings.HasPrefix(ref, "skill:") || Get(ref).ID == ref && Get(ref).CanSubagent() {
			continue
		}
		if seen[ref] || len(seen) >= 8 {
			continue
		}
		seen[ref] = true
		raw, _ := json.Marshal(map[string]any{"path": ref, "offset": 1, "limit": 1200})
		res, err := r.registry.Execute(ctx, "read_file", raw, tools.ExecutionContext{CWD: sess.CWD})
		if err != nil || strings.TrimSpace(res.Content) == "" {
			continue
		}
		content := res.Content
		if len(content) > 120000 {
			content = content[:120000] + "\n…(truncated)"
		}
		blocks = append(blocks, fmt.Sprintf("\n\n<referenced_file path=%q>\n%s\n</referenced_file>", ref, content))
	}
	if len(blocks) == 0 {
		return ""
	}
	return "Additional file context referenced by the latest user message:" + strings.Join(blocks, "")
}
