package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	c.SetSession(domain.Session{Messages: msgs}, 80)
	lines := c.Render(24, ThemeByName("dark"), RenderOptions{StreamContent: "", StreamReasoning: "", Activities: nil})
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

	c.ToggleThinking("a1")
	if !c.expandedThinking["a1"] {
		t.Fatal("reasoning should be expanded")
	}
	if c.scroll <= 0 {
		t.Fatalf("expanding reasoning should compensate scroll, got %d", c.scroll)
	}

	for i := 0; i < 2; i++ {
		lines := c.Render(12, ThemeByName("dark"), RenderOptions{StreamContent: "", StreamReasoning: "", Activities: nil})
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
	c.ToggleThinking("a1")
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
	c.ToggleThinking("a1")
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

func TestStreamingAssistantUsesLightweightPlainRenderer(t *testing.T) {
	c := NewConversation()
	c.SetSession(domain.Session{ID: "s1"}, 80)
	lines := c.Render(8, ThemeByName("dark"), RenderOptions{StreamContent: "**hello**", StreamReasoning: "", Activities: nil})
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(stripANSI(line.Text))
		b.WriteByte('\n')
	}
	if !strings.Contains(b.String(), "**hello**") {
		t.Fatalf("streaming content should use the lightweight plain renderer until completion:\n%s", b.String())
	}
}

func TestModernThemes(t *testing.T) {
	for _, name := range []string{"uinxed", "tokyonight", "catppuccin", "gruvbox", "nord", "dracula", "dark", "light"} {
		th := ThemeByName(name)
		if th.Name != name {
			t.Fatalf("theme name mismatch: got %s, want %s", th.Name, name)
		}
		if th.Primary == nil || th.Secondary == nil || th.Border == nil {
			t.Fatalf("theme %s has nil color components", name)
		}
		if th.Reasoning == nil || th.Gutter == nil {
			t.Fatalf("theme %s is missing the transcript tokens", name)
		}
		if len(th.Glyphs.Spinner) == 0 {
			t.Fatalf("theme %s has no spinner frames", name)
		}
	}
}

// TestNoColorPalette pins the NO_COLOR behaviour: every semantic token must
// resolve to a no-op color so the UI is readable without escape sequences.
func TestNoColorPalette(t *testing.T) {
	if _, ok := noColorPalette.Primary.(lipgloss.NoColor); !ok {
		t.Fatal("NO_COLOR palette should use lipgloss.NoColor for every token")
	}
}

// TestASCIIGlyphsAreNonUnicode guards the LANG=C fallback: nothing may leak a
// multi-byte glyph into a terminal that cannot render it.
func TestASCIIGlyphsAreNonUnicode(t *testing.T) {
	g := glyphSet("ascii")
	for name, v := range map[string]string{
		"Assistant": g.Assistant, "User": g.User, "Result": g.Result, "Thinking": g.Thinking,
		"Success": g.Success, "Failure": g.Failure, "Pending": g.Pending, "Bullet": g.Bullet,
		"ListBullet": g.ListBullet, "Cursor": g.Cursor, "TodoDone": g.TodoDone, "TodoOpen": g.TodoOpen,
		"BarFull": g.BarFull, "BarEmpty": g.BarEmpty, "Sep": g.Sep, "Rule": g.Rule, "VBar": g.VBar,
	} {
		if strings.ContainsFunc(v, func(r rune) bool { return r > 127 }) {
			t.Fatalf("ascii glyph %s = %q contains a non-ASCII rune", name, v)
		}
	}
	for _, f := range g.Spinner {
		if strings.ContainsFunc(f, func(r rune) bool { return r > 127 }) {
			t.Fatalf("ascii spinner frame %q contains a non-ASCII rune", f)
		}
	}
}

// TestTranscriptIsBorderless pins the central visual decision: turns are drawn
// with a glyph gutter, not box-drawing cards.
func TestTranscriptIsBorderless(t *testing.T) {
	c := NewConversation()
	c.SetSession(domain.Session{
		ID: "s1",
		Messages: []domain.Message{
			{ID: "u1", Role: domain.RoleUser, Content: "hello there"},
			{ID: "a1", Role: domain.RoleAssistant, Content: "hi, doing well", ToolCalls: []domain.ToolCall{
				{ID: "t1", Function: domain.ToolCallFunction{Name: "read_file", Arguments: `{"path":"internal/tui/view.go"}`}},
			}},
		},
	}, 80)
	lines := c.Render(40, ThemeByName("uinxed"), RenderOptions{StreamContent: "", StreamReasoning: "", Activities: nil})
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(stripANSI(l.Text))
		b.WriteByte('\n')
	}
	got := b.String()
	for _, box := range []string{"╭", "╰", "│", "╮", "╯"} {
		if strings.Contains(got, box) {
			t.Fatalf("transcript should not draw card borders, found %q in:\n%s", box, got)
		}
	}
	if !strings.Contains(got, "read_file(internal/tui/view.go)") {
		t.Fatalf("expected a collapsed one-line tool call, got:\n%s", got)
	}
	if !strings.Contains(got, "❯ hello there") {
		t.Fatalf("expected the user gutter marker, got:\n%s", got)
	}
}

func TestModernRenderBaseLayout(t *testing.T) {
	m := newMouseTestModel(t)
	m.width, m.height = 100, 30
	v := m.renderBase(ThemeByName("uinxed"))
	plain := stripANSI(v)
	if strings.Contains(plain, "╭─") || strings.Contains(plain, "╰─") {
		t.Fatal("base layout should not draw card borders")
	}
	if strings.Contains(plain, "UINXED AGENT") {
		t.Fatal("header brand was removed; model info belongs in the status row")
	}
	if !strings.Contains(plain, "test-model") {
		t.Fatalf("status row should show the active model, got:\n%s", plain)
	}
	if !strings.Contains(plain, "─") {
		t.Fatal("composer should be delimited by a horizontal rule")
	}
	if strings.Contains(plain, "? for shortcuts") {
		t.Fatal("the status row must stay quiet when idle; the composer placeholder carries the hint")
	}
}

// TestStatusLineDropsLowValueSegments keeps the status row to one line on a
// narrow terminal by discarding the least important segments first.
func TestStatusLineDropsLowValueSegments(t *testing.T) {
	m := newMouseTestModel(t)
	m.cfg.StatusItems = []string{"model", "cwd", "context", "agent", "session"}
	m.session.Name = "a-very-long-session-name-that-would-not-fit"
	m.height = 30

	wide, _ := m.statusLine(ThemeByName("uinxed"), 120)
	narrow, _ := m.statusLine(ThemeByName("uinxed"), 20)
	if lipgloss.Width(narrow) > 20 {
		t.Fatalf("narrow status row overflowed: %q", stripANSI(narrow))
	}
	if !strings.Contains(stripANSI(narrow), "test-model") {
		t.Fatalf("model is the last segment to be dropped, got %q", stripANSI(narrow))
	}
	if len(stripANSI(narrow)) >= len(stripANSI(wide)) {
		t.Fatalf("narrow row should render fewer segments:\n%q\n%q", stripANSI(narrow), stripANSI(wide))
	}
}

// TestToolSummaryPrefersSalientArgument keeps collapsed call lines readable.
func TestToolSummaryPrefersSalientArgument(t *testing.T) {
	cases := []struct{ tool, args, want string }{
		{"read_file", `{"path":"src/app.go","offset":1,"limit":500}`, "src/app.go"},
		{"bash", `{"cmd":"go test ./...","timeout":30}`, "go test ./..."},
		{"grep", `{"pattern":"TODO","include":"*.go"}`, "TODO"},
		{"write_file", `{"path":"a/b/c.go","content":"package a"}`, "a/b/c.go"},
		{"calc", `{"expr":"1+1"}`, "1+1"},
	}
	for _, c := range cases {
		if got := toolSummary(c.tool, c.args); got != c.want {
			t.Errorf("toolSummary(%s) = %q, want %q", c.tool, got, c.want)
		}
	}
	if got := toolSummary("bash", `{"cmd":"echo a\n echo b"}`); strings.Contains(got, "\n") {
		t.Errorf("summary must be single-line, got %q", got)
	}
	if got := toolSummary("bash", "{}"); got != "" {
		t.Errorf("empty arguments should produce no summary, got %q", got)
	}
}

func TestToggleSidebar(t *testing.T) {
	m := newMouseTestModel(t)
	m.cfg.Sidebar = "on"
	m.toggleSidebar()
	if m.cfg.Sidebar != "off" {
		t.Fatalf("expected sidebar to be off, got %s", m.cfg.Sidebar)
	}
	m.toggleSidebar()
	if m.cfg.Sidebar != "on" {
		t.Fatalf("expected sidebar to be on, got %s", m.cfg.Sidebar)
	}
}

func TestModernPickerRender(t *testing.T) {
	var p Picker
	p.Reset("Commands", ActionCommand, []PickerItem{
		{ID: "sidebar", Label: "Toggle Sidebar", Description: "Show or hide the sidebar", Shortcut: "Ctrl+B"},
	})
	lines, regs := p.Render(80, 20, ThemeByName("uinxed"), "")
	if len(lines) == 0 || len(regs) == 0 {
		t.Fatal("expected picker lines and regions")
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(stripANSI(l))
		b.WriteByte('\n')
	}
	if !strings.Contains(b.String(), "Commands") || !strings.Contains(b.String(), "Toggle Sidebar") {
		t.Fatalf("missing picker content:\n%s", b.String())
	}
}
