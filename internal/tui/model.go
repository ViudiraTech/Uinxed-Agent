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
	mainRunID           string
	streamContent       string
	streamReasoning     string
	streamMessageID     string
	activities          []domain.ToolActivity
	subagents           map[string]domain.AgentRun
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
	ta.Placeholder = "Ask anything…"
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 6
	ta.MaxContentHeight = 2000
	ta.CharLimit = 200000
	ta.SetWidth(80)
	_ = ta.Focus()
	m := &Model{ctx: ctx, ctrl: ctrl, cfg: cfg, session: session, prompt: ta, conv: NewConversation(), subagents: map[string]domain.AgentRun{}}
	m.setFocus(FocusPrompt)
	m.activities = append([]domain.ToolActivity(nil), session.ToolActivities...)
	m.conv.SetTheme(cfg.Theme)
	m.conv.SetSession(session, 80)
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.prompt.Focus(), waitRuntime(m.ctrl.Events()), m.refreshSessions())
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
	m.activities = append([]domain.ToolActivity(nil), s.ToolActivities...)
	m.streamContent = ""
	m.streamReasoning = ""
	m.streamMessageID = ""
	chatW := m.layout.chat.W
	if chatW < 20 {
		chatW = max(20, m.width)
	}
	m.conv.SetWidth(chatW)
	m.conv.SetTheme(m.cfg.Theme)
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
func padBetween(left, right string, width int) string {
	gap := width - visibleLen(left) - visibleLen(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
func visibleLen(s string) int { return lipgloss.Width(s) }
func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	csi := false
	for _, r := range s {
		if r == 0x1b {
			esc = true
			continue
		}
		if esc {
			if r == '[' {
				csi = true
				continue
			}
			esc = false
		}
		if csi {
			if r >= '@' && r <= '~' {
				csi = false
				esc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
func formatAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "刚刚"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
