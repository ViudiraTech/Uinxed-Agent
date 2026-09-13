package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/app"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

// stripANSI makes rendered output assertable as plain text.
func stripANSI(s string) string { return ansi.Strip(s) }

// layoutModel builds a Model with enough content to exercise every render path:
// user and assistant turns, reasoning, tool calls in all three states, todos
// and a populated session list for the sidebar.
func layoutModel(t *testing.T) *Model {
	t.Helper()
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 6
	ta.SetWidth(80)
	_ = ta.Focus()

	reasoning := "Checking how the session is issued before touching the token path, because the two share a signing key."
	session := domain.Session{
		ID: "s1", Name: "auth refactor", AgentID: "coding", Model: "test-model",
		ProviderID: "test-provider", CWD: "/home/someone/projects/uinxed-agent-internal",
		Messages: []domain.Message{
			{ID: "u1", Role: domain.RoleUser, Content: "拆一下 auth 模块，把 session 处理分离出去"},
			{ID: "a1", Role: domain.RoleAssistant,
				Content:          "先确认边界。\n\n- `session.go` 签发\n- `token.go` 校验\n\n切点在这里。",
				ReasoningContent: reasoning,
				ToolCalls: []domain.ToolCall{
					{ID: "t1", Function: domain.ToolCallFunction{Name: "read_file", Arguments: `{"path":"internal/auth/session.go"}`}},
					{ID: "t2", Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"cmd":"go test ./internal/auth/..."}`}},
					{ID: "t3", Function: domain.ToolCallFunction{Name: "write_file", Arguments: `{"path":"internal/auth/split.go"}`}},
				}},
		},
		Todos: []domain.Todo{
			{ID: "td1", Subject: "Read existing auth code", Status: "completed"},
			{ID: "td2", Subject: "Extract session handling", Status: "in_progress"},
		},
	}
	m := &Model{
		ctrl: &app.Controller{Config: store}, cfg: store.Snapshot(), session: session,
		prompt: ta, conv: NewConversation(), subagents: map[string]domain.AgentRun{},
		activities: []domain.ToolActivity{
			{ID: "x1", CallID: "t1", Name: "read_file", State: "success", Output: "package auth\n... 142 lines"},
			{ID: "x2", CallID: "t2", Name: "bash", State: "failed", Error: "FAIL internal/auth [build failed]"},
			{ID: "x3", CallID: "t3", Name: "write_file", State: "running"},
		},
		sessions: []domain.Session{{ID: "s1", Name: "auth refactor"}, {ID: "s2", Name: "docs pass"}},
	}
	m.setFocus(FocusPrompt)
	m.conv.SetSession(session, 100)
	return m
}

// TestLayoutFitsEveryTerminalSize is the guard for the hand-rolled height and
// width arithmetic in renderBase: the view must be exactly h lines of exactly w
// columns, for every theme, glyph mode, sidebar state and terminal size.
func TestLayoutFitsEveryTerminalSize(t *testing.T) {
	for _, themeName := range config.Themes() {
		for _, glyphMode := range []string{"unicode", "ascii"} {
			for _, sidebar := range []string{"off", "on"} {
				for _, w := range []int{40, 60, 80, 100, 140} {
					for _, h := range []int{10, 24, 40} {
						m := layoutModel(t)
						m.cfg.Theme = themeName
						m.cfg.Glyphs = glyphMode
						m.cfg.Sidebar = sidebar
						m.width, m.height = w, h
						m.prompt.SetValue("next instruction")
						m.resize() // View() does this before renderBase

						out := m.renderBase(themeFor(m.cfg))
						lines := strings.Split(out, "\n")
						if len(lines) != h {
							t.Fatalf("%s/%s/sidebar=%s %dx%d: got %d lines, want %d",
								themeName, glyphMode, sidebar, w, h, len(lines), h)
						}
						for i, l := range lines {
							if got := lipgloss.Width(l); got != w {
								t.Fatalf("%s/%s/sidebar=%s %dx%d line %d: width %d, want %d\n%q",
									themeName, glyphMode, sidebar, w, h, i, got, w, stripANSI(l))
							}
						}
					}
				}
			}
		}
	}
}

// TestOverlayFitsEveryTerminalSize covers the modal path, which has its own
// centering arithmetic in renderOverlay.
func TestOverlayFitsEveryTerminalSize(t *testing.T) {
	for _, overlay := range []overlayKind{overlayPicker, overlayHelp, overlayTodos, overlayInfo, overlayDiff, overlayConnect, overlayConfirmDelete} {
		for _, w := range []int{30, 50, 80, 120} {
			for _, h := range []int{8, 16, 30} {
				m := layoutModel(t)
				m.width, m.height = w, h
				m.resize()
				m.overlay = overlay
				m.infoTitle = "Info"
				m.infoText = strings.Repeat("some explanatory text that should wrap. ", 12)
				m.picker.Reset("Commands", ActionCommand, []PickerItem{{ID: "a", Label: "Alpha", Description: "first"}})

				out := m.renderOverlay(themeFor(m.cfg))
				lines := strings.Split(out, "\n")
				if len(lines) != h {
					t.Fatalf("overlay=%d %dx%d: got %d lines, want %d", overlay, w, h, len(lines), h)
				}
				for i, l := range lines {
					if got := lipgloss.Width(l); got > w {
						t.Fatalf("overlay=%d %dx%d line %d: width %d exceeds %d", overlay, w, h, i, got, w)
					}
				}
			}
		}
	}
}

// TestSidebarShowsSessionsAndTodos pins what Ctrl+B actually surfaces.
func TestSidebarShowsSessionsAndTodos(t *testing.T) {
	m := layoutModel(t)
	m.cfg.Sidebar = "on"
	m.width, m.height = 110, 32
	out := stripANSI(m.renderBase(themeFor(m.cfg)))
	for _, want := range []string{"SESSIONS", "auth refactor", "docs pass", "TODOS 1/2", "Read existing auth code"} {
		if !strings.Contains(out, want) {
			t.Errorf("sidebar missing %q:\n%s", want, out)
		}
	}
}

// TestAssistantMarkerFollowsGlyphMode guards the LANG=C fallback end to end:
// the turn marker in the transcript must come from the glyph set, not a
// hard-coded Unicode rune.
func TestAssistantMarkerFollowsGlyphMode(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 100, 30
	m.cfg.Glyphs = "ascii"
	out := stripANSI(m.renderBase(themeFor(m.cfg)))
	if strings.Contains(out, "⏺") || strings.Contains(out, "❯") || strings.Contains(out, "⎿") {
		t.Fatalf("ascii mode leaked a Unicode marker:\n%s", out)
	}
	if !strings.Contains(out, "* ") || !strings.Contains(out, "> ") {
		t.Fatalf("ascii mode should draw its own turn markers:\n%s", out)
	}
}

func TestStatusLineIsAlwaysExactlyOneRow(t *testing.T) {
	m := layoutModel(t)
	m.height = 30
	for _, w := range []int{12, 20, 40, 80, 200} {
		line, _ := m.statusLine(themeFor(m.cfg), w)
		if n := strings.Count(line, "\n"); n != 0 {
			t.Fatalf("width %d: status row has %d newlines", w, n)
		}
		if got := lipgloss.Width(line); got != w {
			t.Fatalf("width %d: status row rendered %d columns", w, got)
		}
	}
}

// TestStatusLineRegionsPointAtRealSegments keeps click targets aligned with the
// row they were measured against.
func TestStatusLineRegionsPointAtRealSegments(t *testing.T) {
	m := layoutModel(t)
	m.height = 30
	m.cfg.StatusItems = []string{"model", "agent", "provider"}
	line, regs := m.statusLine(themeFor(m.cfg), 120)
	plain := stripANSI(line)
	if len(regs) != 3 {
		t.Fatalf("expected three clickable segments, got %d", len(regs))
	}
	for _, r := range regs {
		if r.Rect.Y != m.height-1 {
			t.Fatalf("region %s anchored to row %d, want %d", r.Kind, r.Rect.Y, m.height-1)
		}
		if r.Rect.X < 0 || r.Rect.X+r.Rect.W > lipgloss.Width(line) {
			t.Fatalf("region %s spans x=%d w=%d outside the row", r.Kind, r.Rect.X, r.Rect.W)
		}
		if r.Rect.W > len(plain) {
			t.Fatalf("region %s wider than the rendered row", r.Kind)
		}
	}
}

func TestFormatAgoUsesEnglishUnits(t *testing.T) {
	// Guards against the copy regressing to the previous mixed-language UI.
	if got := formatAgo(time.Now()); got != "now" {
		t.Fatalf("recent timestamp = %q, want %q", got, "now")
	}
	if got := formatAgo(time.Now().Add(-3 * time.Hour)); got != "3h" {
		t.Fatalf("hours timestamp = %q, want %q", got, "3h")
	}
}

func TestToolResultSummaryCompressesOutput(t *testing.T) {
	cases := []struct {
		name string
		act  domain.ToolActivity
		want string
	}{
		{"running has no summary", domain.ToolActivity{State: "running", Output: "partial"}, ""},
		{"empty has no summary", domain.ToolActivity{State: "success"}, ""},
		{"single line is shown", domain.ToolActivity{State: "success", Output: "ok\n"}, "ok"},
		{"many lines collapse", domain.ToolActivity{State: "success", Output: "a\nb\nc\n"}, "3 lines"},
		{"error is shown", domain.ToolActivity{State: "failed", Error: "boom"}, "boom"},
	}
	for _, c := range cases {
		if got := toolResultSummary(c.act); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

// benchConversation builds a long transcript so the benchmarks measure the
// steady-state repaint cost rather than a near-empty screen.
func benchConversation(b *testing.B, blocks int) *Conversation {
	b.Helper()
	msgs := make([]domain.Message, 0, blocks)
	for i := 0; i < blocks; i++ {
		if i%2 == 0 {
			msgs = append(msgs, domain.Message{ID: fmt.Sprintf("u%d", i), Role: domain.RoleUser, Content: "请检查这个模块的实现，注意边界条件"})
			continue
		}
		msgs = append(msgs, domain.Message{
			ID: fmt.Sprintf("a%d", i), Role: domain.RoleAssistant,
			Content:          "这里是分析结果。\n\n- 第一点\n- 第二点\n\n结论如上。",
			ReasoningContent: "先确认模块边界，再看错误处理路径。",
			ToolCalls: []domain.ToolCall{
				{ID: fmt.Sprintf("t%d", i), Function: domain.ToolCallFunction{Name: "read_file", Arguments: `{"path":"internal/tui/view.go"}`}},
			},
		})
	}
	c := NewConversation()
	c.SetSession(domain.Session{ID: "bench", Messages: msgs}, 100)
	// Warm the cache the way a live session would be.
	c.Render(40, ThemeByName("uinxed"), RenderOptions{StreamContent: "", StreamReasoning: "", Activities: nil})
	return c
}

// BenchmarkTranscriptFrame measures a steady-state repaint: nothing changed, so
// every visible block should be served from the render cache.
func BenchmarkTranscriptFrame(b *testing.B) {
	c := benchConversation(b, 400)
	acts := []domain.ToolActivity{{ID: "x", CallID: "t1", State: "success", Output: "ok"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Render(40, ThemeByName("uinxed"), RenderOptions{StreamContent: "", StreamReasoning: "", Activities: acts})
	}
}

// BenchmarkStreamingFrame measures the hot path during generation: the streamed
// message grows every frame while the rest of the transcript stays cached. This
// is the frame budget that decides whether output feels live or sluggish.
func BenchmarkStreamingFrame(b *testing.B) {
	c := benchConversation(b, 400)
	t := ThemeByName("uinxed")
	var sb strings.Builder
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sb.WriteString("这是一段正在流式输出的回复内容，用来模拟真实的生成过程。")
		c.Render(40, t, RenderOptions{StreamContent: sb.String()})
	}
}

// BenchmarkRenderBaseFrame measures a full-screen repaint including the status
// row, composer and line padding — the cost Bubble Tea actually pays per frame.
func BenchmarkRenderBaseFrame(b *testing.B) {
	m := layoutModelB(b)
	m.width, m.height = 120, 40
	m.resize()
	m.prompt.SetValue("把 token 校验也一起拆了")
	t := themeFor(m.cfg)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.renderBase(t)
	}
}

// layoutModelB is layoutModel for benchmarks.
func layoutModelB(b *testing.B) *Model {
	b.Helper()
	store, err := config.NewStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 6
	ta.SetWidth(80)
	_ = ta.Focus()
	m := &Model{
		ctrl: &app.Controller{Config: store}, cfg: store.Snapshot(),
		prompt: ta, conv: NewConversation(), subagents: map[string]domain.AgentRun{},
	}
	m.cfg.Theme = "uinxed"
	m.setFocus(FocusPrompt)
	return m
}

// TestLiveStreamShowsReasoningText is the readability contract for streaming:
// the block currently being written must show its reasoning as it arrives.
// Collapsing it to an "N lines" summary left the screen apparently frozen
// during long thinking, because neither the text nor the count visibly changed.
func TestLiveStreamShowsReasoningText(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 90, 24
	m.cfg.Sidebar = "off"
	m.resize()
	m.streamReasoning = "先确认认证模块的边界在哪里，再决定 token 校验的位置"
	m.streamContent = ""

	out := stripANSI(m.renderBase(themeFor(m.cfg)))
	if !strings.Contains(out, "先确认认证模块的边界在哪里") {
		t.Fatalf("live reasoning text must be visible while streaming:\n%s", out)
	}
	// The live block's own header must be the plain "Thinking…" form. The
	// finished turn above it legitimately keeps its "(N lines · ctrl+t)"
	// summary, so this checks the line directly above the streamed text.
	lines := strings.Split(out, "\n")
	idx := -1
	for i, l := range lines {
		if strings.Contains(l, "先确认认证模块的边界在哪里") {
			idx = i
			break
		}
	}
	if idx < 1 {
		t.Fatalf("streamed reasoning not found in output:\n%s", out)
	}
	if header := lines[idx-1]; !strings.Contains(header, "Thinking…") || strings.Contains(header, "lines") {
		t.Fatalf("live reasoning should sit under a plain Thinking… header, got %q", header)
	}
}

// TestCompletedTurnCollapsesReasoning is the other half of that contract: once
// the turn is history, only the summary belongs on screen.
func TestCompletedTurnCollapsesReasoning(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 90, 30
	m.cfg.Sidebar = "off"
	m.resize()

	out := stripANSI(m.renderBase(themeFor(m.cfg)))
	if strings.Contains(out, "Checking how the session is issued") {
		t.Fatalf("completed turns should collapse their reasoning:\n%s", out)
	}
	if !strings.Contains(out, "Thinking… (") {
		t.Fatalf("expected a collapsed reasoning summary:\n%s", out)
	}
}

// TestSettleMessageEndsTheStreamingRound pins the round-boundary behaviour: a
// settled round lands in the transcript, the live buffers reset so the next
// round starts clean, and a redelivery cannot duplicate it.
func TestSettleMessageEndsTheStreamingRound(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 90, 30
	m.cfg.Sidebar = "off"
	m.resize()
	m.streamContent = "partial answer"
	m.streamReasoning = "partial thinking"

	msg := domain.Message{
		ID: "new-1", Role: domain.RoleAssistant, Content: "settled answer",
		ToolCalls: []domain.ToolCall{{ID: "c1", Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"cmd":"ls"}`}}},
	}
	before := len(m.session.Messages)
	m.settleMessage(msg)

	if m.streamContent != "" || m.streamReasoning != "" {
		t.Fatalf("streaming buffers must reset, got content=%q reasoning=%q", m.streamContent, m.streamReasoning)
	}
	if got := len(m.session.Messages); got != before+1 {
		t.Fatalf("messages = %d, want %d", got, before+1)
	}
	m.settleMessage(msg)
	if got := len(m.session.Messages); got != before+1 {
		t.Fatalf("a redelivered round duplicated the transcript: %d messages", got)
	}

	out := stripANSI(m.renderBase(themeFor(m.cfg)))
	if !strings.Contains(out, "Run command · ls") {
		t.Fatalf("the settled round's tool call should render as a readable action:\n%s", out)
	}
	if strings.Contains(out, "bash(ls)") {
		t.Fatalf("internal tool syntax should not be exposed in the collapsed transcript:\n%s", out)
	}
	if strings.Contains(out, "partial answer") {
		t.Fatalf("settled text must replace the streaming buffer:\n%s", out)
	}
}
