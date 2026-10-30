package tui

import (
	"strings"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func TestStableCutStopsAtClosedParagraphs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{name: "no blank line", content: "一段还没结束的文字", want: 0},
		{name: "one closed paragraph", content: "第一段\n\n第二段没完", want: len("第一段\n\n")},
		{name: "trailing blank line", content: "第一段\n\n", want: len("第一段\n\n")},
		{name: "blank line inside a fence is not a break", content: "```go\nfmt.Println(1)\n\nfmt.Println(2)\n```\n\n尾段", want: len("```go\nfmt.Println(1)\n\nfmt.Println(2)\n```\n\n")},
		{name: "unterminated fence stays open", content: "```go\na\n\nb", want: 0},
		{name: "tilde fence", content: "~~~\na\n\nb\n~~~\n\n尾", want: len("~~~\na\n\nb\n~~~\n\n")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stableCut(tc.content); got != tc.want {
				t.Fatalf("stableCut(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

func TestStreamMarkdownStylesClosedAndOpenText(t *testing.T) {
	c := NewConversation()
	c.SetSession(sessionFixture(), 80)
	th := ThemeByName("dark")

	// First frame: one closed paragraph, one still growing.
	lines := renderStream(t, c, th, "**粗**体段落\n\n正在写的 `code`")
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "**") {
		t.Fatalf("closed paragraph still shows literal emphasis:\n%s", joined)
	}
	if !strings.Contains(joined, "粗") || !strings.Contains(joined, "正在写的") {
		t.Fatalf("stream lost text:\n%s", joined)
	}
	if strings.Contains(joined, "`code`") {
		t.Fatalf("the open tail should also render as Markdown, not literal backticks:\n%s", joined)
	}

	// Second frame: the tail closes and a new tail opens. Both sides must still
	// render; a prefix-only renderer would drop the new paragraph.
	lines = renderStream(t, c, th, "**粗**体段落\n\n正在写的 `code`\n\n**新段**来了")
	joined = strings.Join(lines, "\n")
	if strings.Count(joined, "**") > 0 {
		t.Fatalf("a closed paragraph reverted to literal Markdown:\n%s", joined)
	}
	if !strings.Contains(joined, "新段") || !strings.Contains(joined, "正在写的") {
		t.Fatalf("second frame lost content:\n%s", joined)
	}
}

func TestStreamMarkdownResetsForANewMessage(t *testing.T) {
	c := NewConversation()
	c.SetSession(sessionFixture(), 80)
	th := ThemeByName("dark")

	renderStreamID(t, c, th, "m1", "第一轮 **甲**\n\n")
	lines := renderStreamID(t, c, th, "m2", "第二轮 **乙**")
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "甲") {
		t.Fatalf("content from the previous round leaked into the new one:\n%s", joined)
	}
	if !strings.Contains(joined, "乙") {
		t.Fatalf("new round text missing:\n%s", joined)
	}
}

func TestStreamMarkdownFallsBackForAnOversizedTail(t *testing.T) {
	c := NewConversation()
	c.SetSession(sessionFixture(), 80)
	th := ThemeByName("dark")

	// A single unbroken paragraph past the limit must still render, wrapped
	// incrementally, and must not be dropped or duplicated.
	big := strings.Repeat("**长**", streamTailMax/4+64)
	lines := renderStream(t, c, th, "开头\n\n"+big)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "长") {
		t.Fatalf("oversized tail did not render:\n%.200s", joined)
	}
}

// renderStream drives the live block through Conversation.Render and returns
// the stripped visible lines, so the assertion is about what the screen shows.
func renderStream(t *testing.T, c *Conversation, th Theme, content string) []string {
	t.Helper()
	return renderStreamID(t, c, th, "stream-1", content)
}

func renderStreamID(t *testing.T, c *Conversation, th Theme, streamID, content string) []string {
	t.Helper()
	out := c.Render(120, th, RenderOptions{StreamContent: content, StreamMessageID: streamID})
	lines := make([]string, 0, len(out))
	for _, line := range out {
		if strings.TrimSpace(stripANSI(line.Text)) == "" {
			continue
		}
		lines = append(lines, stripANSI(line.Text))
	}
	return lines
}

func sessionFixture() domain.Session { return domain.Session{ID: "s1"} }

// The live render has to land on exactly the layout the settled reply will
// have. A mismatch here is what makes the transcript jump when the turn ends.
func TestStreamMatchesTheSettledRender(t *testing.T) {
	cases := []string{
		"段落\n\n- item",
		"段落\n\n```go\nx := 1\n```",
		"# 标题\n\n段落",
		"# 标题\n\n- item",
		"- a\n- b\n\n段落",
		"```go\nx\n```\n\n段落",
		"段落一\n\n段落二",
		"## 小节\n\n一段**粗**体文字。\n\n> 引用\n\n1. 有序\n2. 列表\n\n| a | b |\n|---|---|\n| 1 | 2 |",
	}
	for _, content := range cases {
		t.Run(strings.ReplaceAll(content, "\n", "\\n"), func(t *testing.T) {
			live := transcript(LiveRender(t, content))
			done := transcript(SettledRender(t, content))
			if len(live) != len(done) {
				t.Fatalf("live has %d rows, settled has %d\nlive:\n%s\nsettled:\n%s",
					len(live), len(done), strings.Join(live, "\n"), strings.Join(done, "\n"))
			}
			for i := range live {
				if live[i] != done[i] {
					t.Fatalf("row %d differs\nlive:    %q\nsettled: %q", i, live[i], done[i])
				}
			}
		})
	}
}

// Growing the message one byte at a time drives every intermediate cut and cache
// miss the live path can hit. The final frame must still equal the settled one,
// which catches a stale prefix being stitched onto a newer tail.
func TestStreamGrowsToTheSettledRender(t *testing.T) {
	content := "## 标题\n\n第一段 **粗**。\n\n```go\nfmt.Println(1)\n```\n\n- 列表\n- 项目\n\n收尾段落。"
	c := NewConversation()
	c.SetSession(sessionFixture(), 80)
	c.SetWidth(80)
	th := ThemeByName("dark")
	var last []renderLine
	for i := 1; i <= len(content); i++ {
		last = c.Render(120, th, RenderOptions{StreamContent: content[:i], StreamMessageID: "grow"})
	}
	live := transcript(last)
	done := transcript(SettledRender(t, content))
	if len(live) != len(done) {
		t.Fatalf("grow has %d rows, settled has %d\nlive:\n%s\nsettled:\n%s",
			len(live), len(done), strings.Join(live, "\n"), strings.Join(done, "\n"))
	}
	for i := range live {
		if live[i] != done[i] {
			t.Fatalf("row %d differs after growing\nlive:    %q\nsettled: %q", i, live[i], done[i])
		}
	}
}

func LiveRender(t *testing.T, content string) []renderLine {
	t.Helper()
	c := NewConversation()
	c.SetSession(sessionFixture(), 80)
	c.SetWidth(80)
	return c.Render(120, ThemeByName("dark"), RenderOptions{StreamContent: content, StreamMessageID: "m1"})
}

func SettledRender(t *testing.T, content string) []renderLine {
	t.Helper()
	c := NewConversation()
	c.SetSession(domain.Session{ID: "s1", Messages: []domain.Message{{ID: "a", Role: domain.RoleAssistant, Content: content}}}, 80)
	c.SetWidth(80)
	return c.Render(120, ThemeByName("dark"), RenderOptions{})
}

// transcript returns the assistant rows from the first marker on, with trailing
// padding removed so blank rows compare as blank.
func transcript(lines []renderLine) []string {
	out := []string{}
	started := false
	for _, line := range lines {
		s := strings.TrimRight(stripANSI(line.Text), " ")
		if !started {
			if !strings.Contains(s, "⏺") {
				continue
			}
			started = true
		}
		out = append(out, s)
	}
	return out
}
