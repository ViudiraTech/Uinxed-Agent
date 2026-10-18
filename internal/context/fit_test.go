package contextmgr

import (
	"strings"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// The history budget is the only bound on what is sent to the provider. It used
// to be dead code: the loop's break condition could never fire.
func TestFitMessagesHonoursBudget(t *testing.T) {
	var msgs []domain.Message
	for i := 0; i < 100; i++ {
		msgs = append(msgs, domain.Message{ID: "m", Role: domain.RoleUser, Content: strings.Repeat("x", 400)})
	}
	const budget = 1000
	got := FitMessages(msgs, budget)
	if len(got) == len(msgs) {
		t.Fatalf("FitMessages returned all %d messages for a %d-token budget (estimated %d)",
			len(got), budget, EstimateMessages(got))
	}
	if est := EstimateMessages(got); est > budget {
		t.Fatalf("kept %d messages worth %d tokens, budget %d", len(got), est, budget)
	}
	// The newest message must survive: dropping it would lose the current turn.
	if got[len(got)-1].Content != msgs[len(msgs)-1].Content {
		t.Fatal("the newest message was trimmed")
	}
}

// A cut must never leave a tool result without the assistant call that owns it,
// or the provider rejects the whole request.
func TestFitMessagesKeepsToolRoundPaired(t *testing.T) {
	msgs := []domain.Message{
		{ID: "u1", Role: domain.RoleUser, Content: "old"},
		{ID: "a1", Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{
			{ID: "c1", Function: domain.ToolCallFunction{Name: "read_file"}},
			{ID: "c2", Function: domain.ToolCallFunction{Name: "grep"}},
		}},
		{ID: "t1", Role: domain.RoleTool, ToolCallID: "c1", Content: strings.Repeat("a", 100)},
		{ID: "t2", Role: domain.RoleTool, ToolCallID: "c2", Content: strings.Repeat("b", 100)},
	}
	// A budget that keeps only the last tool result: the cut lands on t2.
	budget := EstimateMessages(msgs[3:4]) + 1
	got := FitMessages(msgs, budget)
	if len(got) == 0 {
		t.Fatal("nothing kept")
	}
	if got[0].Role == domain.RoleTool {
		t.Fatalf("the trimmed history starts with a tool result: %#v", got[0])
	}
	// The assistant that owns the kept tool result has to come with it.
	if got[0].Role != domain.RoleAssistant {
		t.Fatalf("first message = %#v, want the assistant call", got[0])
	}
}
