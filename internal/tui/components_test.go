package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/app"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func TestPickerFuzzyAndSelection(t *testing.T) {
	var p Picker
	p.Reset("Commands", ActionCommand, []PickerItem{
		{ID: "model", Label: "Change Model", Description: "选择模型"},
		{ID: "session", Label: "Switch Session", Description: "切换会话"},
		{ID: "diff", Label: "Open Diff", Description: "审阅代码"},
	})
	p.SetQuery("mdl")
	it, ok := p.Selected()
	if !ok || it.ID != "model" {
		t.Fatalf("selected=%#v ok=%v filtered=%#v", it, ok, p.Filtered)
	}
	p.SetQuery("zzzz")
	if len(p.Filtered) != 0 {
		t.Fatalf("unexpected matches: %#v", p.Filtered)
	}
}

func TestRegionTopmostWins(t *testing.T) {
	rs := []Region{
		{Rect: Rect{0, 0, 10, 10}, Kind: ActionChat, Value: "chat"},
		{Rect: Rect{2, 2, 3, 3}, Kind: ActionButton, Value: "button"},
	}
	r, ok := findRegion(rs, 3, 3)
	if !ok || r.Value != "button" {
		t.Fatalf("region=%#v ok=%v", r, ok)
	}
}

func TestConversationHidesSystemAndVirtualizes(t *testing.T) {
	msgs := []domain.Message{{ID: "sys", Role: domain.RoleSystem, Content: "secret-context"}}
	for i := 0; i < 1200; i++ {
		msgs = append(msgs, domain.Message{ID: fmt.Sprintf("m-%d", i), Role: domain.RoleUser, Content: "line of text"})
	}
	c := NewConversation()
	c.SetTheme("dark")
	c.SetSession(domain.Session{Messages: msgs}, 80)
	lines := c.Render(24, ThemeByName("dark"), "", "", nil)
	if len(lines) != 24 {
		t.Fatalf("visible lines=%d", len(lines))
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Text)
	}
	if strings.Contains(b.String(), "secret-context") {
		t.Fatal("system context leaked into conversation UI")
	}
	if len(c.blocks) != 1200 {
		t.Fatalf("blocks=%d", len(c.blocks))
	}
}

func TestToggleAllTools(t *testing.T) {
	c := NewConversation()
	c.SetSession(domain.Session{Messages: []domain.Message{{
		ID: "a", Role: domain.RoleAssistant,
		ToolCalls: []domain.ToolCall{
			{ID: "1", Function: domain.ToolCallFunction{Name: "read_file"}},
			{ID: "2", Function: domain.ToolCallFunction{Name: "bash"}},
		},
	}}}, 80)
	c.ToggleAllTools()
	if !c.expandedTools["1"] || !c.expandedTools["2"] {
		t.Fatal("tools were not expanded")
	}
	c.ToggleAllTools()
	if c.expandedTools["1"] || c.expandedTools["2"] {
		t.Fatal("tools were not collapsed")
	}
}

func TestFindRegionPrefersSpecificControlOverChatBackground(t *testing.T) {
	rs := []Region{
		{Rect: Rect{X: 0, Y: 0, W: 80, H: 20}, Kind: ActionTool, Value: "tool-1"},
		{Rect: Rect{X: 0, Y: 0, W: 80, H: 20}, Kind: ActionChat, Value: "chat"},
	}
	r, ok := findRegion(rs, 10, 5)
	if !ok {
		t.Fatal("expected a hit")
	}
	if r.Kind != ActionTool || r.Value != "tool-1" {
		t.Fatalf("specific control should win over chat background, got %#v", r)
	}
}

func TestChatClickKeepsPromptFocus(t *testing.T) {
	m := &Model{prompt: textarea.New()}
	m.overlay = overlayNone
	m.focus.Set(FocusChat)
	m.handleMouseClick(tea.Mouse{X: 5, Y: 5, Button: tea.MouseLeft}, Region{Kind: ActionChat, Value: "chat"}, true)
	if got := m.focus.Current(); got != FocusPrompt {
		t.Fatalf("chat click must keep prompt keyboard focus, got %v", got)
	}
}

func TestReasoningClickStaysExpandedAcrossRenders(t *testing.T) {
	c := NewConversation()
	reasoning := "first reasoning line\nsecond reasoning line\nthird reasoning line"
	c.SetSession(domain.Session{ID: "s1", Messages: []domain.Message{{
		ID: "a1", Role: domain.RoleAssistant, Content: "answer", ReasoningContent: reasoning,
	}}}, 40)

	c.ToggleThinking("a1", "")
	if !c.expandedThinking["a1"] {
		t.Fatal("reasoning should be expanded")
	}
	if c.scroll <= 0 {
		t.Fatalf("expanding reasoning should compensate scroll, got %d", c.scroll)
	}

	for i := 0; i < 2; i++ {
		lines := c.Render(12, ThemeByName("dark"), "", "", nil)
		var b strings.Builder
		for _, line := range lines {
			b.WriteString(stripANSI(line.Text))
			b.WriteByte('\n')
		}
		if !strings.Contains(b.String(), "first reasoning line") {
			t.Fatalf("render %d lost expanded reasoning:\n%s", i+1, b.String())
		}
		if !c.expandedThinking["a1"] {
			t.Fatalf("render %d mutated expansion state", i+1)
		}
	}
}

func TestReasoningExpansionIsPerMessage(t *testing.T) {
	c := NewConversation()
	c.SetSession(domain.Session{ID: "s1", Messages: []domain.Message{
		{ID: "a1", Role: domain.RoleAssistant, ReasoningContent: "reason one"},
		{ID: "a2", Role: domain.RoleAssistant, ReasoningContent: "reason two"},
	}}, 60)
	c.ToggleThinking("a1", "")
	if !c.expandedThinking["a1"] {
		t.Fatal("first reasoning should be expanded")
	}
	if c.expandedThinking["a2"] {
		t.Fatal("second reasoning must remain collapsed")
	}
}

func TestReasoningStateAndViewportSurviveSameSessionReload(t *testing.T) {
	c := NewConversation()
	s := domain.Session{ID: "s1", Messages: []domain.Message{{
		ID: "a1", Role: domain.RoleAssistant, Content: "answer", ReasoningContent: "one\ntwo\nthree",
	}}}
	c.SetSession(s, 40)
	c.ToggleThinking("a1", "")
	wantScroll := c.scroll
	if wantScroll == 0 {
		t.Fatal("expected scroll compensation")
	}

	// Runtime completion/session refresh must not collapse the reasoning or
	// snap the viewport back to the bottom.
	c.SetSession(s, 40)
	if !c.expandedThinking["a1"] {
		t.Fatal("same-session reload lost reasoning expansion")
	}
	if c.scroll != wantScroll {
		t.Fatalf("same-session reload changed viewport: got %d want %d", c.scroll, wantScroll)
	}

	c.SetSession(domain.Session{ID: "s2"}, 40)
	if c.expandedThinking["a1"] {
		t.Fatal("switching sessions should not leak reasoning expansion state")
	}
	if c.scroll != 0 {
		t.Fatalf("switching sessions should reset scroll, got %d", c.scroll)
	}
}

func newMouseTestModel(t *testing.T) *Model {
	t.Helper()
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ta := textarea.New()
	ta.SetWidth(80)
	_ = ta.Focus()
	session := domain.Session{ID: "s1", AgentID: "build", Model: "test-model", ProviderID: "test-provider", CWD: "."}
	m := &Model{
		ctrl:      &app.Controller{Config: store},
		cfg:       store.Snapshot(),
		session:   session,
		prompt:    ta,
		conv:      NewConversation(),
		subagents: map[string]domain.AgentRun{},
		width:     100,
		height:    30,
	}
	m.setFocus(FocusPrompt)
	m.conv.SetTheme(m.cfg.Theme)
	m.conv.SetSession(session, 80)
	return m
}

func TestMouseViewUsesSingleSynchronousRoutingPath(t *testing.T) {
	m := newMouseTestModel(t)
	m.cfg.Mouse = true
	v := m.View()
	if v.OnMouse != nil {
		t.Fatal("mouse events must not be routed through View.OnMouse and Model.Update at the same time")
	}
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v, want cell motion", v.MouseMode)
	}
}

func TestReasoningRawMouseClickTogglesExactlyOnce(t *testing.T) {
	m := newMouseTestModel(t)
	reasoning := "line one\nline two\nline three"
	m.session.Messages = []domain.Message{{
		ID: "a1", Role: domain.RoleAssistant, Content: "answer", ReasoningContent: reasoning,
	}}
	m.conv.SetSession(m.session, 80)
	m.width, m.height = 100, 30
	m.cfg.Mouse = true
	_ = m.View() // populate regions exactly as the screen the click belongs to

	var target Region
	found := false
	for _, r := range m.regions {
		if r.Kind == ActionThinking && r.Value == "thinking:a1" {
			target, found = r, true
			break
		}
	}
	if !found {
		t.Fatal("reasoning region not rendered")
	}
	msg := tea.MouseClickMsg(tea.Mouse{X: target.Rect.X, Y: target.Rect.Y, Button: tea.MouseLeft})
	_, _ = m.Update(msg)
	if !m.conv.expandedThinking["a1"] {
		t.Fatal("one physical/raw click should leave reasoning expanded")
	}
}
