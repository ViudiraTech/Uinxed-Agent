package provider

import (
	"context"
	"encoding/json"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

type Request struct {
	Model    string
	Messages []domain.Message
	Tools    []ToolDefinition
	Effort   string
	Thinking bool
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type EventKind string

const (
	EventContent   EventKind = "content"
	EventReasoning EventKind = "reasoning"
	EventToolCall  EventKind = "tool_call"
	EventUsage     EventKind = "usage"
	EventDone      EventKind = "done"
	EventError     EventKind = "error"
)

type Event struct {
	Kind         EventKind
	Text         string
	ToolCalls    []domain.ToolCall
	Usage        domain.Usage
	FinishReason string
	Model        string
	Err          error
}

type Provider interface {
	Stream(ctx context.Context, req Request) (<-chan Event, error)
	Models(ctx context.Context) ([]string, error)
	CheckKey(ctx context.Context, key string) error
}

func ToolDefinitionFromJSON(raw []byte) (ToolDefinition, error) {
	var d ToolDefinition
	err := json.Unmarshal(raw, &d)
	return d, err
}
