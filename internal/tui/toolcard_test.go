package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// A tool that ran and produced nothing has to say so. The old card simply had no
// result row, which reads as "still working" or as a rendering bug.
func TestEmptyOutputIsNamed(t *testing.T) {
	for _, out := range []string{"", "   ", "\n\n", " \r\n\t\n"} {
		a := domain.ToolActivity{State: "success", Output: out}
		tc := domain.ToolCall{Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"cmd":"true"}`}}
		if got := toolResultClause(tc, a); got != noOutputLabel {
			t.Fatalf("output %q rendered %q, want %q", out, got, noOutputLabel)
		}
	}
}

// The expanded body says the same thing instead of showing an empty box.
func TestExpandedEmptyOutputIsNamed(t *testing.T) {
	c := NewConversation()
	a := domain.ToolActivity{State: "success", Output: "  \n\n"}
	lines := c.renderExpandedResult(a, ThemeByName("claude"), 80)
	joined := stripANSI(joinRenderLines(lines))
	if !strings.Contains(joined, noOutputLabel) {
		t.Fatalf("expanded body = %q, want %q", joined, noOutputLabel)
	}
}

// Real output is left alone apart from the padding: CRLF, trailing blanks and the
// blank lines at the edges. Inner blank lines are content.
func TestCleanToolOutputTrimsOnlyPadding(t *testing.T) {
	in := "\r\n  ok  \t\r\n\r\n  second\r\n\r\n"
	want := "  ok\n\n  second"
	if got := cleanToolOutput(in); got != want {
		t.Fatalf("cleanToolOutput(%q) = %q, want %q", in, got, want)
	}
}

// A single-line result has nothing hidden, so the card must not advertise an
// expander for it.
func TestExpandHintOnlyWhenSomethingIsHidden(t *testing.T) {
	single := domain.ToolActivity{State: "success", Output: "one line"}
	if toolExpandable(domain.ToolCall{Function: domain.ToolCallFunction{Name: "calc", Arguments: `{"expr":"1+1"}`}}, single) {
		t.Fatal("a one-line result with a shown argument claims more to show")
	}
	multi := domain.ToolActivity{State: "success", Output: "a\nb\nc"}
	if !toolExpandable(domain.ToolCall{Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"command":"ls"}`}}, multi) {
		t.Fatal("a multi-line result offered no way to open it")
	}
}

// The finished card showed its duration twice: once on the head and again on the
// result row.
func TestDurationAppearsOncePerCard(t *testing.T) {
	c := NewConversation()
	tc := domain.ToolCall{ID: "t1", Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"command":"ls"}`}}
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a := domain.ToolActivity{CallID: "t1", State: "success", Output: "a\nb", StartedAt: base, EndedAt: base.Add(1500 * time.Millisecond)}
	lines := c.renderToolCall(tc, a, ThemeByName("claude"), 80, "", 0)
	joined := stripANSI(joinRenderLines(lines))
	if n := strings.Count(joined, "1.5s"); n != 1 {
		t.Fatalf("duration appears %d times:\n%s", n, joined)
	}
}

func joinRenderLines(lines []renderLine) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(l.Text)
	}
	return b.String()
}
