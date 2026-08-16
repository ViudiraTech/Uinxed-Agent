package tools

import (
	"context"
	"encoding/json"

	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

type Result struct {
	Content  string         `json:"content,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	ExitCode *int           `json:"exit_code,omitempty"`
}

func (r Result) JSON() string {
	b, _ := json.Marshal(r)
	return string(b)
}

type RuntimeCallbacks struct {
	TodoWrite  func(ctx context.Context, raw json.RawMessage) (Result, error)
	TodoUpdate func(ctx context.Context, raw json.RawMessage) (Result, error)
	Delegate   func(ctx context.Context, raw json.RawMessage) (Result, error)
}

type ExecutionContext struct {
	CWD       string
	Callbacks RuntimeCallbacks
	OnOutput  func(stream, text string)
}

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Category() Category
	Execute(ctx context.Context, input json.RawMessage, env ExecutionContext) (Result, error)
}

func Definition(t Tool) provider.ToolDefinition {
	return provider.ToolDefinition{
		Type:     "function",
		Function: provider.ToolFunction{Name: t.Name(), Description: t.Description(), Parameters: t.Schema()},
	}
}
