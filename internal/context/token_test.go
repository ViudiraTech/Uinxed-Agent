package contextmgr

import (
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"testing"
)

func TestWindowLookupAndSuffix(t *testing.T) {
	cases := map[string]int{"deepseek-v4-pro": 1000000, "foo-128k": 128000, "bar-1.5m": 1500000, "unknown": DefaultContextWindow}
	for in, want := range cases {
		if got := Window(in); got != want {
			t.Errorf("Window(%q)=%d want %d", in, got, want)
		}
	}
}

func TestFitMessagesKeepsToolParent(t *testing.T) {
	msgs := []domain.Message{
		{Role: domain.RoleAssistant, Content: "call", ToolCalls: []domain.ToolCall{{ID: "c1", Function: domain.ToolCallFunction{Name: "read_file"}}}},
		{Role: domain.RoleTool, ToolCallID: "c1", Content: "result"},
		{Role: domain.RoleUser, Content: "next"},
	}
	// Budget enough for the final user and the tool message, but not the original assistant.
	out := FitMessages(msgs, EstimateMessages(msgs[1:]))
	if len(out) < 2 {
		t.Fatalf("unexpected fit: %#v", out)
	}
	if out[0].Role == domain.RoleTool {
		t.Fatal("fit must not start with orphan tool message")
	}
}

func TestEstimateTextCJK(t *testing.T) {
	if EstimateText("你好世界") <= EstimateText("abcd") {
		t.Fatal("CJK estimate should be denser than latin for equal rune count")
	}
}
