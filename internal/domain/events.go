package domain

import (
	"encoding/json"
	"time"
)

type EventKind string

const (
	EventStreamDelta    EventKind = "stream.delta"
	EventReasoningDelta EventKind = "stream.reasoning"
	EventMessageAdded   EventKind = "message.added"
	EventToolStarted    EventKind = "tool.started"
	EventToolOutput     EventKind = "tool.output"
	EventToolFinished   EventKind = "tool.finished"
	EventAgentStarted   EventKind = "agent.started"
	EventAgentFinished  EventKind = "agent.finished"
	EventTodoChanged    EventKind = "todo.changed"
	EventSessionChanged EventKind = "session.changed"
	EventUsageChanged   EventKind = "usage.changed"
	EventCompaction     EventKind = "context.compaction"
	EventError          EventKind = "error"
	EventStatus         EventKind = "status"

	// Approval events are notifications only. The user's answer travels back
	// through a direct method call (Controller.ResolveApproval), never through
	// an event: the event pipeline is one-way and CoalesceEvents reorders and
	// batches, which cannot safely carry a response.
	EventApprovalRequested EventKind = "approval.requested"
	EventApprovalResolved  EventKind = "approval.resolved"

	// EventPlanChanged carries the session's plan steps after plan_write ran,
	// so the UI can refresh its plan view without reloading the conversation.
	EventPlanChanged EventKind = "plan.changed"
)

type Event struct {
	Kind      EventKind
	SessionID string
	RunID     string
	At        time.Time
	Data      any
}

type StreamDelta struct {
	MessageID string
	Text      string
}

type ReasoningDelta struct {
	MessageID string
	Text      string
}

type ToolEvent struct {
	Activity ToolActivity
}

type AgentEvent struct {
	Run AgentRun
}

type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

type ErrorData struct {
	Op      string
	Message string
	Details string
}

// ApprovalRequest is one pending tool call awaiting a user decision.
//
// It is deliberately self-contained: the TUI renders the prompt from this
// value alone, so an approval raised by a delegate child session displays just
// as well as one from the session the user is looking at.
type ApprovalRequest struct {
	ID        string          `json:"id"`
	ToolName  string          `json:"tool_name"`
	Category  string          `json:"category"`
	Summary   string          `json:"summary"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	// SessionID and RunID identify the origin so the UI can attribute a
	// request to a subagent instead of assuming it belongs to the open session.
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
}

// ApprovalResolved reports how a pending request ended. It carries no authority;
// it exists so the UI can clear a prompt that was answered elsewhere (a
// cancelled turn, or a second client).
type ApprovalResolved struct {
	ID      string `json:"id"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// SessionChanged reports a lightweight in-turn session-scope change (a mode
// switch applied by switch_mode) so the UI can update the mode pill in place
// without reloading the whole conversation mid-stream.
type SessionChanged struct {
	Mode string `json:"mode"`
}

func NewEvent(kind EventKind, sessionID string, data any) Event {
	return Event{Kind: kind, SessionID: sessionID, At: time.Now(), Data: data}
}
