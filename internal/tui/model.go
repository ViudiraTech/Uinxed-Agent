package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/app"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	gitutil "github.com/ViudiraTech/Uinxed-Agent/internal/git"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayPicker
	overlayHelp
	overlayTodos
	overlayContext
	overlayInfo
	overlayDiff
	overlayConnect
	overlayConfirmRestore
	overlayConfirmDelete
	overlayApproval
)

type layoutState struct {
	chat    Rect
	sidebar Rect
	prompt  Rect
	status  Rect
	overlay Rect
	chatX   int
}

type Model struct {
	ctx                 context.Context
	ctrl                *app.Controller
	cfg                 config.Config
	session             domain.Session
	sessions            []domain.Session
	width, height       int
	prompt              textarea.Model
	conv                *Conversation
	focus               FocusManager
	overlay             overlayKind
	picker              Picker
	pickerPurpose       string
	diff                DiffView
	infoTitle, infoText string
	errorText           string
	toast               string
	toastUntil          time.Time
	busy                bool
	busySince           time.Time
	mainRunID           string
	streamContent       string
	streamReasoning     string
	streamMessageID     string
	activities          []domain.ToolActivity
	subagents           map[string]domain.AgentRun
	subagentProgress    map[string]subagentProgress
	regions             []Region
	hover               string
	layout              layoutState
	sidebarOffset       int
	history             []string
	historyIndex        int
	commandMatches      []PickerItem
	atMatches           []PickerItem
	connect             connectWizard
	confirmTarget       string
	overlayScroll       int
	activityFrame       int
	statusCmdText       string
	// approval is the currently presented prompt, if any. It lives outside the
	// overlay queue: an approval raised by a delegate child must be visible
	// regardless of which session the user is browsing.
	approval *domain.ApprovalRequest
	// approvalChoice is the highlighted option (0 allow-once, 1 always,
	// 2 deny-with-feedback); approvalFeedback switches the overlay into its
	// reason-entry mode.
	approvalChoice   int
	approvalFeedback bool
	approvalInput    string
}

// subagentProgress tracks live activity inside a delegate child so the sidebar
// can show something more useful than a static "running" label. It is keyed by
// the child run ID, matching Model.subagents.
type subagentProgress struct {
	Tools     int
	LastTool  string
	UpdatedAt time.Time
}

type connectWizard struct {
	Step    int
	Input   string
	Name    string
	BaseURL string
	Models  string
	Key     string
}

func New(ctx context.Context, ctrl *app.Controller, session domain.Session) *Model {
	cfg := ctrl.Config.Snapshot()
	ta := textarea.New()
	ta.Placeholder = "Ask anything…   ( / commands · @ files · ? shortcuts )"
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 6
	ta.MaxContentHeight = 2000
	ta.CharLimit = 200000
	ta.SetWidth(80)
	_ = ta.Focus()
	m := &Model{ctx: ctx, ctrl: ctrl, cfg: cfg, session: session, prompt: ta, conv: NewConversation(), subagents: map[string]domain.AgentRun{}, subagentProgress: map[string]subagentProgress{}}
	m.setFocus(FocusPrompt)
	m.activities = append([]domain.ToolActivity(nil), session.ToolActivities...)
	m.conv.SetSession(session, 80)
	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.prompt.Focus(), waitRuntime(m.ctrl.Events()), m.refreshSessions()}
	if strings.TrimSpace(m.cfg.StatuslineCommand) != "" {
		cmds = append(cmds, statusTick(), m.runStatusCommand())
	}
	return tea.Batch(cmds...)
}

type runtimeMsg struct {
	event domain.Event
	ok    bool
}
type sessionMsg struct {
	s   domain.Session
	err error
}
type sessionsMsg struct {
	ss  []domain.Session
	err error
}
type modelsMsg struct {
	models []string
	err    error
}
type diffMsg struct {
	s   gitutil.Snapshot
	err error
}
type fileDiffMsg struct {
	path, text string
	err        error
}
type opMsg struct {
	op    string
	value any
	err   error
}
type toastMsg string
type animationTickMsg struct{}

func waitRuntime(ch <-chan domain.Event) tea.Cmd {
	return func() tea.Msg { e, ok := <-ch; return runtimeMsg{e, ok} }
}
func (m *Model) refreshSessions() tea.Cmd {
	return func() tea.Msg { ss, err := m.ctrl.ListSessions(m.ctx); return sessionsMsg{ss, err} }
}
func (m *Model) reloadSession() tea.Cmd {
	id := m.session.ID
	return func() tea.Msg { s, err := m.ctrl.LoadSession(m.ctx, id); return sessionMsg{s, err} }
}
func asyncOp(op string, fn func() (any, error)) tea.Cmd {
	return func() tea.Msg { v, err := fn(); return opMsg{op: op, value: v, err: err} }
}

func (m *Model) setSession(s domain.Session) {
	m.session = s
	m.activities = m.activities[:0]
	for _, a := range s.ToolActivities {
		if a.Name == "" {
			continue
		}
		m.activities = append(m.activities, a)
	}
	// Subagent runs belong to the session that spawned them. Keeping the
	// previous session's entries would show stale rows that never update and,
	// because Go map iteration is random, shuffle the sidebar every frame.
	if m.subagents == nil {
		m.subagents = map[string]domain.AgentRun{}
	} else {
		clear(m.subagents)
	}
	if m.subagentProgress == nil {
		m.subagentProgress = map[string]subagentProgress{}
	} else {
		clear(m.subagentProgress)
	}
	m.streamContent = ""
	m.streamReasoning = ""
	m.streamMessageID = ""
	chatW := m.layout.chat.W
	if chatW < 20 {
		chatW = max(20, m.width)
	}
	m.conv.SetWidth(chatW)
	m.conv.SetSession(s, chatW)
}

func (m *Model) showToast(s string) {
	m.toast = terminalutil.SanitizeText(s)
	m.toastUntil = time.Now().Add(3 * time.Second)
}
func (m *Model) showError(err error) {
	if err == nil {
		return
	}
	m.errorText = terminalutil.SanitizeText(err.Error())
	m.showToast("× " + err.Error())
}
func (m *Model) closeOverlay() {
	m.overlay = overlayNone
	m.pickerPurpose = ""
	m.connect = connectWizard{}
	m.confirmTarget = ""
	m.overlayScroll = 0
	m.setFocus(FocusPrompt)
}

func (m *Model) setFocus(f Focus) {
	m.focus.Set(f)
	if f == FocusPrompt {
		_ = m.prompt.Focus()
		return
	}
	m.prompt.Blur()
}

func (m *Model) ensurePromptFocus() {
	if m.overlay == overlayNone && m.focus.Current() != FocusPrompt {
		m.setFocus(FocusPrompt)
	}
}
func (m *Model) openInfo(title, text string, kind overlayKind) {
	m.infoTitle = terminalutil.SanitizeText(title)
	m.infoText = terminalutil.SanitizeText(text)
	m.overlayScroll = 0
	m.overlay = kind
	m.setFocus(FocusOverlay)
}
func (m *Model) currentEffort() string {
	if v, ok := m.session.Metadata["effort"].(string); ok && v != "" {
		return v
	}
	return m.cfg.Effort
}
func (m *Model) thinkingEnabled() bool {
	if v, ok := m.session.Metadata["thinking"].(bool); ok {
		return v
	}
	return m.cfg.Thinking
}

// themeFor resolves the configured theme and glyph set together. Every render
// path must go through it so a configured glyph mode is never silently ignored.
func themeFor(cfg config.Config) Theme {
	t := theme(cfg.Theme)
	t.Glyphs = glyphSet(cfg.Glyphs)
	return t
}

func padBetween(left, right string, width int) string {
	gap := width - visibleLen(left) - visibleLen(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
func visibleLen(s string) int { return lipgloss.Width(s) }

func formatAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
