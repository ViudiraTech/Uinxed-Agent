// Copyright 2026 Uinxed Project
// Licensed under the Apache License, Version 2.0.

package domain

import (
	"encoding/json"
	"time"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	Index    int              `json:"-"`
	ID       string           `json:"id"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	ID               string     `json:"id,omitempty"`
	Role             Role       `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	Name             string     `json:"name,omitempty"`
	CreatedAt        time.Time  `json:"created_at,omitempty"`
}

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

type Todo struct {
	ID        string     `json:"id"`
	Subject   string     `json:"subject"`
	Status    TodoStatus `json:"status"`
	Reason    string     `json:"reason,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type ToolActivity struct {
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Output    string          `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
	ExitCode  *int            `json:"exit_code,omitempty"`
	State     string          `json:"state"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   time.Time       `json:"ended_at,omitempty"`
}

type AgentRun struct {
	ID           string    `json:"id"`
	ParentID     string    `json:"parent_id,omitempty"`
	SessionID    string    `json:"session_id"`
	AgentID      string    `json:"agent_id"`
	Task         string    `json:"task,omitempty"`
	State        string    `json:"state"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	InputTokens  int64     `json:"input_tokens,omitempty"`
	OutputTokens int64     `json:"output_tokens,omitempty"`
}

type Session struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	ProviderID     string         `json:"provider"`
	Model          string         `json:"model"`
	AgentID        string         `json:"agent"`
	CWD            string         `json:"cwd"`
	Messages       []Message      `json:"messages"`
	Todos          []Todo         `json:"todos"`
	ToolActivities []ToolActivity `json:"tool_activities,omitempty"`
	ParentID       string         `json:"parent_id,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

func (s Session) Clone() Session {
	out := s
	out.Messages = append([]Message(nil), s.Messages...)
	out.Todos = append([]Todo(nil), s.Todos...)
	out.ToolActivities = append([]ToolActivity(nil), s.ToolActivities...)
	if s.Metadata != nil {
		out.Metadata = make(map[string]any, len(s.Metadata))
		for k, v := range s.Metadata {
			out.Metadata[k] = v
		}
	}
	return out
}
