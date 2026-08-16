package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/agent"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
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
		if m.busy && m.cfg.Animations {
			m.activityFrame++
			return m, animationTick()
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
	case domain.EventAgentStarted:
		if a, ok := e.Data.(domain.AgentEvent); ok {
			if a.Run.SessionID == m.session.ID {
				m.busy = true
				m.mainRunID = a.Run.ID
				if m.cfg.Animations {
					return animationTick()
				}
			} else {
				m.subagents[a.Run.ID] = a.Run
			}
		}
	case domain.EventAgentFinished:
		if a, ok := e.Data.(domain.AgentEvent); ok {
			if a.Run.SessionID == m.session.ID {
				m.busy = false
				m.mainRunID = ""
				return m.reloadSession()
			}
			m.subagents[a.Run.ID] = a.Run
		}
	case domain.EventStreamDelta:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.StreamDelta); ok {
				m.streamMessageID = d.MessageID
				m.streamContent += d.Text
			}
		}
	case domain.EventReasoningDelta:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.ReasoningDelta); ok {
				m.streamMessageID = d.MessageID
				m.streamReasoning += d.Text
			}
		}
	case domain.EventToolStarted, domain.EventToolOutput, domain.EventToolFinished:
		if e.SessionID == m.session.ID {
			if d, ok := e.Data.(domain.ToolEvent); ok {
				m.mergeActivity(d.Activity)
			}
		}
	case domain.EventTodoChanged:
		if e.SessionID == m.session.ID {
			if ts, ok := e.Data.([]domain.Todo); ok {
				m.session.Todos = append([]domain.Todo(nil), ts...)
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
func (m *Model) mergeActivity(a domain.ToolActivity) {
	for i := range m.activities {
		if m.activities[i].ID == a.ID {
			m.activities[i] = a
			return
		}
	}
	m.activities = append(m.activities, a)
}

func (m *Model) handleKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	// There is no standalone keyboard mode for chat/sidebar. If no modal is
	// active, typing must always belong to the prompt. This prevents mouse
	// clicks, resizes, or async UI updates from leaving the textarea stranded.
	m.ensurePromptFocus()
	key := k.String()
	if key == "ctrl+c" {
		if m.overlay != overlayNone {
			m.closeOverlay()
			return nil, true
		}
		if m.busy && m.ctrl.Cancel(m.session.ID) {
			m.showToast("取消当前生成…")
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
			m.showToast("取消当前生成…")
			return nil, true
		}
		m.conv.GotoBottom()
		return nil, true
	}
	if key == "ctrl+o" && m.overlay == overlayTodos {
		m.closeOverlay()
		return nil, true
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
	case "ctrl+p":
		m.openCommandPalette()
		return nil, true
	case "ctrl+t":
		m.conv.ToggleAllThinking(m.streamReasoning)
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
	case "enter":
		return m.submitPrompt(), true
	case "shift+enter", "alt+enter":
		m.prompt.InsertString("\n")
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
				m.showToast("@" + name + " 后面需要任务")
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
		m.showToast("当前生成仍在运行；Esc/Ctrl+C 可取消")
		return nil
	}
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
		m.conv.ToggleThinking(id, m.streamReasoning)
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
				detail := fmt.Sprintf("%s\n\n状态: %s", todo.Subject, todo.Status)
				if todo.Reason != "" {
					detail += "\n原因: " + todo.Reason
				}
				m.openInfo("Todo", detail, overlayInfo)
				break
			}
		}
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

func (m *Model) handleOp(x opMsg) tea.Cmd {
	if x.err != nil {
		m.showError(x.err)
		return nil
	}
	switch x.op {
	case "submit":
		m.busy = true
	case "start_subagent":
		if child, ok := x.value.(domain.Session); ok {
			m.setSession(child)
			m.busy = true
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
	case "storage":
		m.cfg = m.ctrl.Config.Snapshot()
		m.showToast(fmt.Sprintf("✓ storage migrated (%v sessions)", x.value))
		return m.refreshSessions()
	case "theme":
		m.cfg = m.ctrl.Config.Snapshot()
		m.conv.SetTheme(m.cfg.Theme)
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
	if m.width >= 120 && m.cfg.Sidebar != "off" {
		chatW -= 30
	}
	if chatW < 30 {
		chatW = max(20, m.width)
	}
	m.prompt.SetWidth(max(10, chatW-2))
	m.conv.SetWidth(max(20, chatW-2))
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
