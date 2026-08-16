package domain

import "time"

type EventKind string

const (
	EventStreamDelta    EventKind = "stream.delta"
	EventReasoningDelta EventKind = "stream.reasoning"
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

func NewEvent(kind EventKind, sessionID string, data any) Event {
	return Event{Kind: kind, SessionID: sessionID, At: time.Now(), Data: data}
}
