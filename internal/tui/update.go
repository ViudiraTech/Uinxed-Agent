package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/agent"
	"github.com/ViudiraTech/Uinxed-Agent/internal/approval"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/storage"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = x.Width, x.Height
		m.resize()
		m.ensurePromptFocus()
		return m, nil
	case tea.FocusMsg:
		m.ensurePromptFocus()
		if m.overlay == overlayNone {
			return m, m.prompt.Focus()
		}
		return m, nil
	case tea.BlurMsg:
		// Keep logical focus stable. On terminal focus regain we explicitly
		// re-assert textarea focus so Alt-Tab/tmux focus changes cannot strand input.
		return m, nil
	case runtimeMsg:
		if !x.ok {
			return m, nil
		}
		cmd := m.handleRuntime(x.event)
		return m, tea.Batch(cmd, waitRuntime(m.ctrl.Events()))
	case sessionMsg:
		if x.err != nil {
			m.showError(x.err)
			return m, nil
		}
		m.setSession(x.s)
		return m, m.refreshSessions()
	case sessionsMsg:
		if x.err != nil {
			m.showError(x.err)
		} else {
			m.sessions = x.ss
		}
		return m, nil
	case modelsMsg:
		if x.err != nil {
			m.showError(x.err)
			return m, nil
		}
		m.openModelPicker(x.models)
		return m, nil
	case diffMsg:
		if x.err != nil {
			m.showError(x.err)
			return m, nil
		}
		m.diff.Set(x.s)
		m.overlay = overlayDiff
		m.setFocus(FocusDiff)
		if len(x.s.Files) > 0 {
			return m, m.loadDiffFile(x.s.Files[0].Path)
		}
		return m, nil
	case fileDiffMsg:
		if x.err != nil {
			m.showError(x.err)
		} else {
			m.diff.FileText = x.text
			m.diff.Scroll = 0
			for i, f := range m.diff.Snapshot.Files {
				if f.Path == x.path {
					m.diff.Selected = i
					break
				}
			}
		}
		return m, nil
	case opMsg:
		return m, m.handleOp(x)
	case toastMsg:
		m.showToast(string(x))
		return m, nil
	case animationTickMsg:
		if m.cfg.Animations && (m.busy || m.hasRunningSubagents()) {
			m.activityFrame++
			return m, animationTick()
		}
		return m, nil
	case statusCmdTickMsg:
		if strings.TrimSpace(m.ctrl.Config.Snapshot().StatuslineCommand) == "" {
			return m, nil
		}
		return m, tea.Batch(m.runStatusCommand(), statusTick())
	case statusCmdMsg:
		if x.err == nil {
			m.statusCmdText = x.text
		}
		return m, nil
	case tea.MouseClickMsg:
		mouse := x.Mouse()
		r, ok := findRegion(m.regions, mouse.X, mouse.Y)
		return m, m.handleMouseClick(mouse, r, ok)
	case tea.MouseWheelMsg:
		return m, m.handleMouseWheel(x.Mouse(), m.layout)
	case tea.MouseMotionMsg:
		mouse := x.Mouse()
		r, ok := findRegion(m.regions, mouse.X, mouse.Y)
		m.handleMouseMotion(mouse, r, ok)
		return m, nil
	case tea.PasteMsg:
		if m.overlay == overlayConnect {
			m.connect.Input += x.Content
			return m, nil
		}
		if m.overlay == overlayPicker {
			m.picker.SetQuery(m.picker.Query + x.Content)
			return m, nil
		}
		if m.overlay == overlayHistory {
			m.setHistoryQuery(m.historyQuery + x.Content)
			return m, nil
		}
	case tea.KeyPressMsg:
		if cmd, handled := m.handleKey(x); handled {
			return m, cmd
		}
	}
	if m.overlay == overlayNone && m.focus.Current() == FocusPrompt {
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		m.updateInlineSuggestions()
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleRuntime(e domain.Event) tea.Cmd {
	switch e.Kind {
	// Approval events deliberately bypass the per-session filter below: they
	// can be raised by a delegate child while the user is browsing the parent,
	// and a filtered prompt would hang that child's whole tool round forever.
	case domain.EventApprovalRequested:
		if req, ok := e.Data.(domain.ApprovalRequest); ok {
			req.SessionID = e.SessionID
			req.RunID = e.RunID
			m.approval = &req
			m.approvalChoice = 0
			m.approvalFeedback = false
			m.approvalInput = ""
			m.overlay = overlayApproval
			m.setFocus(FocusOverlay)
		}
	case domain.EventApprovalResolved:
		if d, ok := e.Data.(domain.ApprovalResolved); ok && m.approval != nil && m.approval.ID == d.ID {
			m.approval = nil
			if m.overlay == overlayApproval {
				m.overlay = overlayNone
				m.setFocus(FocusPrompt)
			}
		}
	case domain.EventAgentStarted:
		if a, ok := e.Data.(domain.AgentEvent); ok {
			if a.Run.SessionID == m.session.ID {
				m.busy = true
				m.busySince = time.Now()
				m.mainRunID = a.Run.ID
				if m.cfg.Animations {
					return animationTick()
				}
			} else {
				m.upsertSubagent(a.Run)
				if m.cfg.Animations && !m.busy {
					return animationTick()
				}
			}
		}
	case domain.EventAgentFinished:
		if a, ok := e.Data.(domain.AgentEvent); ok {
			if a.Run.SessionID == m.session.ID {
				m.busy = false
				m.mainRunID = ""
				return tea.Batch(m.reloadSession(), m.autoTitle())
			}
			m.upsertSubagent(a.Run)
		}
	case domain.EventStreamDelta:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.StreamDelta); ok {
				m.streamMessageID = d.MessageID
				m.streamContent += d.Text
			}
		} else {
			m.touchSubagentHeartbeat(e.RunID, e.SessionID)
		}
	case domain.EventReasoningDelta:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.ReasoningDelta); ok {
				m.streamMessageID = d.MessageID
				m.streamReasoning += d.Text
			}
		} else {
			m.touchSubagentHeartbeat(e.RunID, e.SessionID)
		}
	case domain.EventMessageAdded:
		// A turn is made of several model rounds; each finished round arrives
		// here already persisted. Settling it immediately is what keeps one
		// round's text and tool calls from being concatenated with the next.
		if e.SessionID == m.session.ID {
			if msg, ok := e.Data.(domain.Message); ok {
				m.settleMessage(msg)
			}
		}
	case domain.EventToolStarted, domain.EventToolFinished:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.ToolEvent); ok {
				m.mergeActivity(d.Activity)
			}
		} else if d, ok := e.Data.(domain.ToolEvent); ok {
			// Child-session tool activity belongs to a delegate subagent.
			// Mirror only start/finish (not every output chunk) so the sidebar
			// stays live without re-rendering on every streamed byte.
			if d.Activity.Name != "" {
				m.trackSubagentTool(e.RunID, e.SessionID, d.Activity, e.Kind == domain.EventToolStarted)
			}
		}
	case domain.EventToolOutput:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.ToolEvent); ok {
				m.mergeActivity(d.Activity)
			}
		}
		// Deliberately ignored for subagents: output chunks arrive at byte
		// granularity and would repaint the sidebar far more often than the
		// 125ms spinner tick, which is what made it look like flicker.
	case domain.EventTodoChanged:
		if e.SessionID == m.session.ID {
			if ts, ok := e.Data.([]domain.Todo); ok {
				m.session.Todos = append([]domain.Todo(nil), ts...)
			}
		}
	case domain.EventPlanChanged:
		if e.SessionID == m.session.ID {
			if steps, ok := e.Data.([]domain.PlanStep); ok {
				// The plan lives in session metadata so it survives both
				// storage backends; mirror the live copy there.
				if m.session.Metadata == nil {
					m.session.Metadata = map[string]any{}
				}
				if len(steps) == 0 {
					m.session.Metadata["plan"] = ""
				} else if b, err := json.Marshal(steps); err == nil {
					m.session.Metadata["plan"] = string(b)
				}
			}
		}
	case domain.EventSessionChanged:
		if e.SessionID == m.session.ID {
			// Mid-turn updates (a switch_mode applied by the model) must not
			// reload the session: setSession clears the streaming buffers and
			// would blank the message being rendered. Patch metadata only.
			if d, ok := e.Data.(domain.SessionChanged); ok && d.Mode != "" {
				if m.session.Metadata == nil {
					m.session.Metadata = map[string]any{}
				}
				m.session.Metadata["mode"] = d.Mode
			}
		}
	case domain.EventCompaction:
		if e.SessionID == m.session.ID {
			m.showToast("Context compact: " + fmt.Sprint(e.Data))
		}
	case domain.EventError:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.ErrorData); ok {
				m.showError(fmt.Errorf("%s", d.Message))
			}
		}
	}
	return nil
}

// upsertSubagent inserts or updates a background run and keeps the list bounded
// so the sidebar height stays stable instead of growing with every delegate.
func (m *Model) upsertSubagent(run domain.AgentRun) {
	if run.ID == "" {
		return
	}
	if m.subagents == nil {
		m.subagents = map[string]domain.AgentRun{}
	}
	if m.subagentProgress == nil {
		m.subagentProgress = map[string]subagentProgress{}
	}
	m.subagents[run.ID] = run
	if _, ok := m.subagentProgress[run.ID]; !ok {
		m.subagentProgress[run.ID] = subagentProgress{UpdatedAt: time.Now()}
	}
	m.pruneSubagents()
}

// trackSubagentTool records live tool progress for a delegate child. It matches
// by run ID first, then by child session ID for events that arrived before the
// AgentStarted was processed.
func (m *Model) trackSubagentTool(runID, sessionID string, act domain.ToolActivity, started bool) {
	if m.subagents == nil {
		m.subagents = map[string]domain.AgentRun{}
	}
	if m.subagentProgress == nil {
		m.subagentProgress = map[string]subagentProgress{}
	}
	key := runID
	if _, ok := m.subagents[key]; !ok && sessionID != "" {
		for id, r := range m.subagents {
			if r.SessionID == sessionID {
				key = id
				break
			}
		}
	}
	if _, ok := m.subagents[key]; !ok {
		return
	}
	p := m.subagentProgress[key]
	if started {
		p.Tools++
	}
	if act.Name != "" {
		p.LastTool = act.Name
	}
	p.UpdatedAt = time.Now()
	m.subagentProgress[key] = p
}

// touchSubagentHeartbeat keeps a running subagent's elapsed timer visibly fresh
// without storing per-byte output. Stream deltas are throttled to 1s so they
// cannot drive the render loop by themselves.
func (m *Model) touchSubagentHeartbeat(runID, sessionID string) {
	key := runID
	if _, ok := m.subagents[key]; !ok && sessionID != "" {
		for id, r := range m.subagents {
			if r.SessionID == sessionID {
				key = id
				break
			}
		}
	}
	p, ok := m.subagentProgress[key]
	if !ok {
		return
	}
	if _, running := m.subagents[key]; !running {
		return
	}
	if time.Since(p.UpdatedAt) < time.Second {
		return
	}
	p.UpdatedAt = time.Now()
	m.subagentProgress[key] = p
}

func (m *Model) hasRunningSubagents() bool {
	for _, r := range m.subagents {
		if r.State == "running" {
			return true
		}
	}
	return false
}

const maxVisibleSubagents = 8

func (m *Model) pruneSubagents() {
	if len(m.subagents) <= maxVisibleSubagents {
		return
	}
	// Keep running runs plus the most recently started finished runs.
	type entry struct {
		id      string
		running bool
		started time.Time
	}
	entries := make([]entry, 0, len(m.subagents))
	for id, r := range m.subagents {
		entries = append(entries, entry{id: id, running: r.State == "running", started: r.StartedAt})
	}
	// Sort: running first, then newest start first.
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			swap := false
			if entries[j].running && !entries[i].running {
				swap = true
			} else if entries[j].running == entries[i].running && entries[j].started.After(entries[i].started) {
				swap = true
			}
			if swap {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	for _, e := range entries[maxVisibleSubagents:] {
		delete(m.subagents, e.id)
		delete(m.subagentProgress, e.id)
	}
}
func (m *Model) mergeActivity(a domain.ToolActivity) {
	// Nameless activities belong to ghost calls that were filtered upstream;
	// keeping them would resurrect an empty card via the activity list.
	if a.Name == "" {
		// Allow lookup by CallID to fill in the name late, but never store a
		// permanently nameless entry.
		for i := range m.activities {
			if m.activities[i].ID == a.ID && m.activities[i].Name != "" {
				return
			}
		}
		return
	}
	for i := range m.activities {
		if m.activities[i].ID == a.ID {
			m.activities[i] = a
			return
		}
	}
	m.activities = append(m.activities, a)
}

// settleMessage folds a finished model round into the transcript and clears the
// streaming buffers, so the next round draws as its own turn. Without this the
// live block would accumulate every round of a multi-round turn into one blob
// and hide the tool calls made in between.
func (m *Model) settleMessage(msg domain.Message) {
	for _, existing := range m.session.Messages {
		if existing.ID != "" && existing.ID == msg.ID {
			return
		}
	}
	msg.ToolCalls = sanitizeToolCalls(msg.ToolCalls)
	// A round that only carried ghost calls carries no visible content; settling
	// it would insert a bare marker line. The following rounds still arrive.
	if len(msg.ToolCalls) == 0 && msg.Content == "" && msg.ReasoningContent == "" {
		m.streamContent = ""
		m.streamReasoning = ""
		m.streamMessageID = ""
		return
	}
	m.session.Messages = append(m.session.Messages, msg)
	m.session.UpdatedAt = time.Now()
	m.streamContent = ""
	m.streamReasoning = ""
	m.streamMessageID = ""
	m.conv.SetSession(m.session, m.conv.width)
}

func (m *Model) handleKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	// There is no standalone keyboard mode for chat/sidebar. If no modal is
	// active, typing must always belong to the prompt. This prevents mouse
	// clicks, resizes, or async UI updates from leaving the textarea stranded.
	m.ensurePromptFocus()
	key := k.String()
	// Approval prompts must be intercepted before the ctrl+c/esc branches
	// below: the generic esc handler closes any overlay, which here would
	// dismiss the prompt visually while the tool goroutine stays blocked on it.
	if m.overlay == overlayApproval {
		return m.handleApprovalKey(k)
	}
	if key == "ctrl+c" {
		if m.overlay != overlayNone {
			m.closeOverlay()
			return nil, true
		}
		if m.busy && m.ctrl.Cancel(m.session.ID) {
			m.showToast("cancelling…")
			return nil, true
		}
		return tea.Quit, true
	}
	if key == "esc" {
		if m.overlay != overlayNone {
			m.closeOverlay()
			return nil, true
		}
		if m.busy && m.ctrl.Cancel(m.session.ID) {
			m.showToast("cancelling…")
			return nil, true
		}
		m.conv.GotoBottom()
		return nil, true
	}
	if key == "ctrl+o" && m.overlay == overlayTodos {
		m.closeOverlay()
		return nil, true
	}
	if m.overlay == overlayHistory {
		return m.handleHistoryKey(k), true
	}
	if m.overlay == overlayConnect {
		return m.handleConnectKey(k), true
	}
	if m.overlay == overlayConfirmRestore || m.overlay == overlayConfirmDelete {
		return m.handleConfirmKey(k), true
	}
	if m.overlay == overlayPicker {
		return m.handlePickerKey(k), true
	}
	if m.overlay == overlayDiff {
		switch key {
		case "up", "k":
			m.diff.ScrollBy(-1)
		case "down", "j":
			m.diff.ScrollBy(1)
		case "pgup":
			m.diff.ScrollBy(-max(1, m.layout.overlay.H-3))
		case "pgdown":
			m.diff.ScrollBy(max(1, m.layout.overlay.H-3))
		case "tab":
			if len(m.diff.Snapshot.Files) > 0 {
				m.diff.Selected = (m.diff.Selected + 1) % len(m.diff.Snapshot.Files)
				return m.loadDiffFile(m.diff.Snapshot.Files[m.diff.Selected].Path), true
			}
		default:
			return nil, false
		}
		return nil, true
	}
	if m.overlay != overlayNone {
		switch key {
		case "up", "k":
			m.overlayScroll = max(0, m.overlayScroll-1)
			return nil, true
		case "down", "j":
			m.overlayScroll++
			return nil, true
		case "pgup":
			m.overlayScroll = max(0, m.overlayScroll-max(1, m.layout.overlay.H-3))
			return nil, true
		case "pgdown":
			m.overlayScroll += max(1, m.layout.overlay.H-3)
			return nil, true
		}
		return nil, false
	}
	switch key {
	case "ctrl+b":
		return m.toggleSidebar(), true
	case "ctrl+d":
		return m.executeCommand("/diff"), true
	case "ctrl+p":
		m.openCommandPalette()
		return nil, true
	case "ctrl+t":
		m.conv.ToggleAllThinking()
		return nil, true
	case "ctrl+o":
		m.overlay = overlayTodos
		m.overlayScroll = 0
		m.setFocus(FocusTodos)
		return nil, true
	case "ctrl+e":
		m.conv.ToggleAllTools()
		return nil, true
	case "tab":
		if len(m.atMatches) > 0 {
			return m.acceptAutocomplete(), true
		}
		if len(m.commandMatches) > 0 {
			return m.acceptCommandSuggestion(), true
		}
		return m.cycleAgent(), true
	case "shift+tab":
		// Mode ring: plan → read-only → auto-edit → full-auto. Distinct from
		// plain tab, which handles completions and agent cycling.
		return m.cycleApprovalMode(), true
	case "enter":
		return m.submitPrompt(), true
	case "?":
		// Only steal "?" when the composer is empty, so typing a question at
		// the start of a prompt still works.
		if strings.TrimSpace(m.prompt.Value()) == "" {
			return m.executeCommand("/help"), true
		}
		return nil, false
	case "shift+enter", "alt+enter":
		m.prompt.InsertString("\n")
		return nil, true
	case "ctrl+r":
		m.openHistorySearch()
		return nil, true
	case "alt+up":
		m.historyMove(-1)
		return nil, true
	case "alt+down":
		m.historyMove(1)
		return nil, true
	case "pgup":
		m.conv.ScrollUp(max(3, m.layout.chat.H-2))
		return nil, true
	case "pgdown":
		m.conv.ScrollDown(max(3, m.layout.chat.H-2))
		return nil, true
	}
	return nil, false
}

// handleApprovalKey runs the approval overlay's key handling. It is installed
// ahead of every global binding so esc means "deny this operation" rather than
// "close the overlay" — the waiter behind the prompt must always receive an
// explicit decision or a cancelled context, never a silent dismissal.
func (m *Model) handleApprovalKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	key := k.String()
	if m.approval == nil {
		m.overlay = overlayNone
		m.setFocus(FocusPrompt)
		return nil, true
	}
	// Feedback entry mode: option 3 collects a message for the model.
	if m.approvalFeedback {
		switch key {
		case "esc":
			m.approvalFeedback = false
			m.approvalInput = ""
			return nil, true
		case "enter":
			reason := strings.TrimSpace(m.approvalInput)
			if reason == "" {
				reason = "the user declined this action"
			}
			return m.resolveApproval(false, false, reason), true
		case "backspace":
			m.approvalInput = removeLastRune(m.approvalInput)
			return nil, true
		default:
			if k.Text != "" {
				m.approvalInput += k.Text
			}
			return nil, true
		}
	}
	switch key {
	case "1":
		return m.resolveApproval(true, false, ""), true
	case "2":
		return m.resolveApproval(true, true, ""), true
	case "3":
		m.approvalFeedback = true
		m.approvalInput = ""
		return nil, true
	case "up", "k":
		m.approvalChoice = max(0, m.approvalChoice-1)
		return nil, true
	case "down", "j":
		m.approvalChoice = min(2, m.approvalChoice+1)
		return nil, true
	case "enter":
		switch m.approvalChoice {
		case 0:
			return m.resolveApproval(true, false, ""), true
		case 1:
			return m.resolveApproval(true, true, ""), true
		default:
			m.approvalFeedback = true
			m.approvalInput = ""
			return nil, true
		}
	case "esc":
		// Deny this operation only; the turn keeps running so the model can
		// propose a different approach.
		return m.resolveApproval(false, false, "the user declined this action"), true
	case "ctrl+c":
		// Quitting with a prompt pending must not strand the waiter: deny the
		// request, then fall through to the normal interrupt path.
		m.resolveApproval(false, false, "the user interrupted the session")
		if m.busy && m.ctrl.Cancel(m.session.ID) {
			m.showToast("cancelling…")
			return nil, true
		}
		return tea.Quit, true
	}
	return nil, false
}

// resolveApproval hands the decision to the runtime and clears the prompt. The
// response travels by direct method call, never as an event: the event stream
// is batched and reordered, and a lost answer would hang the tool round.
func (m *Model) resolveApproval(allow, always bool, reason string) tea.Cmd {
	a := m.approval
	if a == nil {
		return nil
	}
	m.approval = nil
	m.approvalFeedback = false
	m.approvalInput = ""
	m.overlay = overlayNone
	m.setFocus(FocusPrompt)
	d := agent.Decision{Allow: allow, Always: always, Reason: reason}
	reqID := a.ID
	return func() tea.Msg {
		if !m.ctrl.ResolveApproval(a.RunID, reqID, d) {
			return toastMsg("approval already settled")
		}
		return nil
	}
}

// cycleApprovalMode moves the session to the next mode in the ring and
// persists it in the session metadata, where the runtime reads it each round.
func (m *Model) cycleApprovalMode() tea.Cmd {
	next := string(approval.Next(approval.Normalize(m.currentMode())))
	sid := m.session.ID
	return asyncOp("set_mode", func() (any, error) {
		return next, m.ctrl.SetApprovalMode(m.ctx, sid, next)
	})
}

func (m *Model) currentMode() string {
	if v, ok := m.session.Metadata["mode"].(string); ok && v != "" {
		return v
	}
	return m.cfg.ApprovalMode
}

// openHistorySearch starts a Ctrl+R reverse search over the prompt history.
// Matches are ordered newest first so the default selection is the last thing
// typed, mirroring shell history search expectations.
func (m *Model) openHistorySearch() {
	m.historyQuery = ""
	m.historyMatches = nil
	for i := len(m.history) - 1; i >= 0; i-- {
		m.historyMatches = append(m.historyMatches, m.history[i])
	}
	m.historySel = 0
	m.overlay = overlayHistory
	m.setFocus(FocusOverlay)
}

func (m *Model) handleHistoryKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	switch key {
	case "esc":
		m.closeOverlay()
	case "enter":
		if m.historySel >= 0 && m.historySel < len(m.historyMatches) {
			m.prompt.SetValue(m.historyMatches[m.historySel])
		}
		m.closeOverlay()
	case "up", "ctrl+k", "ctrl+p":
		m.historySel = max(0, m.historySel-1)
	case "down", "ctrl+j", "ctrl+n":
		m.historySel = min(len(m.historyMatches)-1, m.historySel+1)
	case "backspace":
		m.setHistoryQuery(removeLastRune(m.historyQuery))
	default:
		if k.Text != "" {
			m.setHistoryQuery(m.historyQuery + k.Text)
		}
	}
	return nil
}

func (m *Model) setHistoryQuery(q string) {
	m.historyQuery = q
	m.historyMatches = filterHistory(m.history, q)
	m.historySel = 0
}

// filterHistory returns the history entries containing the query, newest
// first. An empty query matches everything.
func filterHistory(history []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []string
	for i := len(history) - 1; i >= 0; i-- {
		if q == "" || strings.Contains(strings.ToLower(history[i]), q) {
			out = append(out, history[i])
		}
	}
	return out
}

func (m *Model) submitPrompt() tea.Cmd {
	text := strings.TrimSpace(m.prompt.Value())
	if text == "" {
		return nil
	}
	m.history = append(m.history, text)
	m.historyIndex = len(m.history)
	m.prompt.SetValue("")
	m.commandMatches = nil
	m.atMatches = nil
	if strings.HasPrefix(text, "/") {
		return m.executeCommand(text)
	}
	if fields := strings.Fields(text); len(fields) > 0 && strings.HasPrefix(fields[0], "@") {
		name := strings.TrimPrefix(fields[0], "@")
		if def := agent.Get(name); def.ID == name && def.CanSubagent() {
			task := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
			if task == "" {
				m.showToast("@" + name + " needs a task description")
				return nil
			}
			parentID := m.session.ID
			return asyncOp("start_subagent", func() (any, error) {
				child, _, err := m.ctrl.StartSubagent(m.ctx, parentID, name, task)
				if err != nil {
					return nil, err
				}
				_ = m.ctrl.SwitchSession(m.ctx, child.ID)
				return child, nil
			})
		}
	}
	if m.busy {
		m.showToast("a turn is already running; esc or ctrl+c to cancel")
		return nil
	}
	m.errorText = ""
	m.streamContent = ""
	m.streamReasoning = ""
	m.conv.GotoBottom()
	id := m.session.ID
	return asyncOp("submit", func() (any, error) { return m.ctrl.Submit(m.ctx, id, text) })
}

func (m *Model) historyMove(delta int) {
	if len(m.history) == 0 {
		return
	}
	m.historyIndex += delta
	if m.historyIndex < 0 {
		m.historyIndex = 0
	}
	if m.historyIndex > len(m.history) {
		m.historyIndex = len(m.history)
	}
	if m.historyIndex == len(m.history) {
		m.prompt.SetValue("")
	} else {
		m.prompt.SetValue(m.history[m.historyIndex])
	}
}

func (m *Model) handleMouseClick(mouse tea.Mouse, r Region, ok bool) tea.Cmd {
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	if !ok {
		// Background clicks are navigation gestures, not a reason to steal
		// keyboard input from the prompt.
		m.ensurePromptFocus()
		return nil
	}
	switch r.Kind {
	case ActionAgent:
		m.openAgentPicker()
	case ActionModel:
		return m.fetchModels()
	case ActionProvider:
		m.openProviderPicker()
	case ActionSession:
		m.ensurePromptFocus()
		return m.switchSessionCmd(r.Value)
	case ActionTool:
		m.conv.ToggleTool(r.Value)
		m.ensurePromptFocus()
	case ActionThinking:
		id := strings.TrimPrefix(r.Value, "thinking:")
		m.conv.ToggleThinking(id)
		m.ensurePromptFocus()
	case ActionCommand:
		m.prompt.SetValue(r.Value)
		m.setFocus(FocusPrompt)
		return m.submitPrompt()
	case ActionPicker:
		for i, j := range m.picker.Filtered {
			if m.picker.Items[j].ID == r.Value {
				m.picker.Index = i
				break
			}
		}
		return m.choosePicker()
	case ActionDiffFile:
		return m.loadDiffFile(r.Value)
	case ActionTodo:
		for _, todo := range m.session.Todos {
			if todo.ID == r.Value {
				detail := fmt.Sprintf("%s\n\nStatus: %s", todo.Subject, todo.Status)
				if todo.Reason != "" {
					detail += "\nReason: " + todo.Reason
				}
				m.openInfo("Todo", detail, overlayInfo)
				break
			}
		}
	case ActionFileChip:
		// Clicking a chip removes that reference from the composer, matching
		// the × affordance drawn next to it.
		m.prompt.SetValue(removeFileRef(m.prompt.Value(), r.Value))
		m.ensurePromptFocus()
	case ActionPrompt, ActionChat, ActionSidebar:
		m.setFocus(FocusPrompt)
	}
	return nil
}

func (m *Model) handleMouseWheel(mouse tea.Mouse, layout layoutState) tea.Cmd {
	up := mouse.Button == tea.MouseWheelUp
	down := mouse.Button == tea.MouseWheelDown
	if !up && !down {
		return nil
	}
	d := m.cfg.ScrollSpeed
	if d < 1 {
		d = 3
	}
	if up {
		d = -d
	}
	if m.overlay == overlayDiff && layout.overlay.Contains(mouse.X, mouse.Y) {
		m.diff.ScrollBy(d)
		return nil
	}
	if m.overlay == overlayPicker && layout.overlay.Contains(mouse.X, mouse.Y) {
		if d < 0 {
			m.picker.Move(-1)
		} else {
			m.picker.Move(1)
		}
		return nil
	}
	if m.overlay != overlayNone && layout.overlay.Contains(mouse.X, mouse.Y) {
		m.overlayScroll += d
		if m.overlayScroll < 0 {
			m.overlayScroll = 0
		}
		return nil
	}
	if layout.chat.Contains(mouse.X, mouse.Y) {
		if d < 0 {
			m.conv.ScrollUp(-d)
		} else {
			m.conv.ScrollDown(d)
		}
		m.ensurePromptFocus()
		return nil
	}
	if layout.sidebar.Contains(mouse.X, mouse.Y) {
		m.sidebarOffset += d
		if m.sidebarOffset < 0 {
			m.sidebarOffset = 0
		}
		maxOff := max(0, len(m.sessions)-layout.sidebar.H+2)
		if m.sidebarOffset > maxOff {
			m.sidebarOffset = maxOff
		}
		m.ensurePromptFocus()
		return nil
	}
	return nil
}

func (m *Model) handleMouseMotion(_ tea.Mouse, r Region, ok bool) {
	if ok {
		m.hover = r.Value
	} else {
		m.hover = ""
	}
}

// autoTitle names the session from its first exchange once a turn completes.
// Titling is cosmetic, so a failure is swallowed rather than surfaced: the
// session simply keeps its generated name and the next turn may try again.
func (m *Model) autoTitle() tea.Cmd {
	if titled, _ := m.session.Metadata["autotitled"].(bool); titled {
		return nil
	}
	id := m.session.ID
	return asyncOp("autotitle", func() (any, error) {
		title, err := m.ctrl.AutoTitleSession(m.ctx, id)
		if err != nil {
			return "", nil
		}
		return title, nil
	})
}

func (m *Model) handleOp(x opMsg) tea.Cmd {
	if x.err != nil {
		m.showError(x.err)
		return nil
	}
	switch x.op {
	case "autotitle":
		title, _ := x.value.(string)
		if title == "" {
			return nil
		}
		m.showToast("✓ " + title)
		return tea.Batch(m.reloadSession(), m.refreshSessions())
	case "submit":
		m.busy = true
		m.busySince = time.Now()
	case "start_subagent":
		if child, ok := x.value.(domain.Session); ok {
			m.setSession(child)
			m.busy = true
			m.busySince = time.Now()
			return m.refreshSessions()
		}
	case "switch_session":
		if s, ok := x.value.(domain.Session); ok {
			m.setSession(s)
			m.closeOverlay()
			return m.refreshSessions()
		}
	case "new_session":
		if s, ok := x.value.(domain.Session); ok {
			m.setSession(s)
			m.showToast("✓ new session")
			return m.refreshSessions()
		}
	case "search_sessions":
		ss, _ := x.value.([]domain.Session)
		if len(ss) == 0 {
			m.showToast("no sessions matched")
			return nil
		}
		items := make([]PickerItem, 0, len(ss))
		for _, s := range ss {
			desc := storage.SearchSnippet(s)
			if desc == "" {
				desc = s.AgentID + " · " + formatAgo(s.UpdatedAt)
			}
			if s.ID == m.session.ID {
				desc = "current · " + desc
			}
			items = append(items, PickerItem{s.ID, s.Name, desc, ""})
		}
		m.picker.Reset("Search Results", ActionSession, items)
		m.pickerPurpose = "search"
		m.overlay = overlayPicker
		m.setFocus(FocusPicker)
	case "export":
		path, _ := x.value.(string)
		if path != "" {
			m.showToast("✓ exported to " + path)
		}
	case "storage":
		m.cfg = m.ctrl.Config.Snapshot()
		m.showToast(fmt.Sprintf("✓ storage migrated (%v sessions)", x.value))
		return m.refreshSessions()
	case "theme":
		m.cfg = m.ctrl.Config.Snapshot()
		m.showToast("✓ theme: " + m.cfg.Theme)
		return nil
	case "mouse":
		m.cfg = m.ctrl.Config.Snapshot()
		m.showToast(fmt.Sprintf("✓ mouse: %v", m.cfg.Mouse))
		return nil
	case "set_agent", "set_model", "set_provider", "set_effort", "set_thinking", "clear", "compact", "cd", "key", "connect", "delete":
		m.showToast("✓ " + strings.ReplaceAll(x.op, "_", " "))
		if x.op == "delete" {
			m.closeOverlay()
			return tea.Batch(m.ensureCurrentSession(), m.refreshSessions())
		}
		if x.op == "connect" {
			m.closeOverlay()
		}
		return m.reloadSession()
	case "set_mode":
		mode, _ := x.value.(string)
		if mode != "" {
			m.showToast("mode: " + mode)
		}
		return m.reloadSession()
	case "profile":
		raw, _ := json.MarshalIndent(x.value, "", "  ")
		m.openInfo("Quota / Profile", string(raw), overlayInfo)
	case "reset":
		return tea.Quit
	case "rename":
		m.showToast("✓ session renamed")
		return tea.Batch(m.reloadSession(), m.refreshSessions())
	}
	return nil
}

func (m *Model) resize() {
	chatW := m.width
	if m.width >= 96 && m.cfg.Sidebar != "off" {
		sidebarW := min(32, max(24, m.width/4))
		chatW -= (sidebarW + 1)
	}
	if chatW < 30 {
		chatW = max(20, m.width)
	}
	m.prompt.SetWidth(max(10, chatW-4))
	m.conv.SetWidth(max(20, chatW))
}
func (m *Model) fetchModels() tea.Cmd {
	id := m.session.ProviderID
	return func() tea.Msg { v, e := m.ctrl.Models(m.ctx, id); return modelsMsg{v, e} }
}
func (m *Model) loadDiffFile(path string) tea.Cmd {
	cwd := m.session.CWD
	return func() tea.Msg { s, e := m.ctrl.FileDiff(m.ctx, cwd, path); return fileDiffMsg{path, s, e} }
}
func (m *Model) switchSessionCmd(id string) tea.Cmd {
	return asyncOp("switch_session", func() (any, error) {
		if err := m.ctrl.SwitchSession(m.ctx, id); err != nil {
			return nil, err
		}
		return m.ctrl.LoadSession(m.ctx, id)
	})
}
func (m *Model) ensureCurrentSession() tea.Cmd {
	return func() tea.Msg { s, e := m.ctrl.EnsureSession(m.ctx); return sessionMsg{s, e} }
}
func removeLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	return string(r[:len(r)-1])
}
func parseBoolWord(s string, current bool) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "true", "1", "yes":
		return true, nil
	case "off", "false", "0", "no":
		return false, nil
	case "":
		return !current, nil
	}
	return current, fmt.Errorf("expected on/off")
}
func atoi(s string) int { v, _ := strconv.Atoi(s); return v }

func animationTick() tea.Cmd {
	return tea.Tick(125*time.Millisecond, func(time.Time) tea.Msg { return animationTickMsg{} })
}
