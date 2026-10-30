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
