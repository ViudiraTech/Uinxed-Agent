package config

import (
	"encoding/json"
	"os"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

type legacyConfig struct {
	History         []legacyMessage `json:"history"`
	Conversation    []legacyMessage `json:"conversation"`
	Sessions        []legacySession `json:"sessions"`
	ActiveSessionID string          `json:"activeSessionId"`
	AgentID         string          `json:"agentId"`
	CWD             string          `json:"cwd"`
	Model           string          `json:"model"`
	ActiveProvider  string          `json:"activeProvider"`
}

// legacySession is the on-disk shape used in config storage mode. The first
// block of fields is the original schema and must keep its names so files
// written by older versions still load. The second block is the canonical
// session, written by SaveLegacySessions; every field is optional, and a read
// falls back to the legacy values or the global config when it is absent.
type legacySession struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	AgentID      string           `json:"agentId"`
	CWD          string           `json:"cwd"`
	UpdatedAt    int64            `json:"updatedAt"`
	History      []legacyMessage  `json:"history"`
	Conversation []domain.Message `json:"conversation"`

	CreatedAt      int64                 `json:"created_at,omitempty"`
	ProviderID     string                `json:"provider,omitempty"`
	Model          string                `json:"model,omitempty"`
	ParentID       string                `json:"parent_id,omitempty"`
	Metadata       map[string]any        `json:"metadata,omitempty"`
	Todos          []domain.Todo         `json:"todos,omitempty"`
	ToolActivities []domain.ToolActivity `json:"tool_activities,omitempty"`
}

type legacyMessage struct {
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	Reasoning        string            `json:"reasoning"`
	ReasoningContent string            `json:"reasoning_content"`
	Time             int64             `json:"time"`
	ToolCallID       string            `json:"tool_call_id"`
	Name             string            `json:"name"`
	ToolCalls        []domain.ToolCall `json:"tool_calls"`
}

type LegacySnapshot struct {
	Sessions        []domain.Session
	ActiveSessionID string
}

func ReadLegacySessions(configFile string) (LegacySnapshot, error) {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return LegacySnapshot{}, err
	}
	var lc legacyConfig
	if err := json.Unmarshal(raw, &lc); err != nil {
		return LegacySnapshot{}, err
	}
	sessions := lc.Sessions
	if len(sessions) == 0 && (len(lc.History) > 0 || len(lc.Conversation) > 0) {
		// Pre-session config files stored one flat transcript. It is in the
		// legacy message shape, so it belongs in History, not Conversation.
		history := lc.Conversation
		if len(history) == 0 {
			history = lc.History
		}
		sessions = []legacySession{{
			ID: "s-default", Name: "Session 1", AgentID: lc.AgentID, CWD: lc.CWD,
			UpdatedAt: time.Now().UnixMilli(), History: history,
		}}
	}
	out := LegacySnapshot{ActiveSessionID: lc.ActiveSessionID}
	for _, s := range sessions {
		updated := time.UnixMilli(s.UpdatedAt)
		if s.UpdatedAt <= 0 {
			updated = time.Now()
		}
		created := updated
		if s.CreatedAt > 0 {
			created = time.UnixMilli(s.CreatedAt)
		}
		id := s.ID
		if id == "" {
			id = "s-legacy-" + updated.Format("20060102150405.000")
		}
		name := s.Name
		if name == "" {
			name = id
		}
		agent := s.AgentID
		if agent == "" {
			agent = "build"
		}
		session := domain.Session{
			ID: id, Name: name, CreatedAt: created, UpdatedAt: updated,
			ProviderID: s.ProviderID, Model: s.Model, AgentID: agent, CWD: s.CWD,
			ParentID: s.ParentID, Metadata: s.Metadata,
			Todos: s.Todos, ToolActivities: s.ToolActivities,
		}
		// Sessions written before these fields existed inherit the global
		// configuration, which is what the UI showed them as anyway.
		if session.ProviderID == "" {
			session.ProviderID = lc.ActiveProvider
		}
		if session.CWD == "" {
			session.CWD = lc.CWD
		}
		if session.ProviderID == "" {
			session.ProviderID = "ux-gateway"
		}
		if session.Model == "" {
			session.Model = lc.Model
		}
		if session.Model == "" {
			session.Model = DefaultModel
		}

		if len(s.Conversation) > 0 {
			session.Messages = append([]domain.Message(nil), s.Conversation...)
		} else {
			for _, m := range s.History {
				reasoning := m.ReasoningContent
				if reasoning == "" {
					reasoning = m.Reasoning
				}
				msg := domain.Message{
					Role: domain.Role(m.Role), Content: m.Content, ReasoningContent: reasoning,
					ToolCallID: m.ToolCallID, Name: m.Name, ToolCalls: m.ToolCalls,
				}
				if m.Time > 0 {
					msg.CreatedAt = time.UnixMilli(m.Time)
				}
				session.Messages = append(session.Messages, msg)
			}
		}
		out.Sessions = append(out.Sessions, session)
	}
	if out.ActiveSessionID == "" && len(out.Sessions) > 0 {
		out.ActiveSessionID = out.Sessions[0].ID
	}
	return out, nil
}

// legacySessionOut writes both the original fields (so older builds can still
// read the file) and the canonical session, so nothing is lost on round-trip.
type legacySessionOut struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	AgentID      string           `json:"agentId"`
	CWD          string           `json:"cwd"`
	UpdatedAt    int64            `json:"updatedAt"`
	History      []map[string]any `json:"history"`
	Conversation []domain.Message `json:"conversation"`

	CreatedAt      int64                 `json:"created_at,omitempty"`
	ProviderID     string                `json:"provider,omitempty"`
	Model          string                `json:"model,omitempty"`
	ParentID       string                `json:"parent_id,omitempty"`
	Metadata       map[string]any        `json:"metadata,omitempty"`
	Todos          []domain.Todo         `json:"todos,omitempty"`
	ToolActivities []domain.ToolActivity `json:"tool_activities,omitempty"`
}

func (s *Store) SaveLegacySessions(sessions []domain.Session, activeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := map[string]any{}
	if raw, err := os.ReadFile(s.file); err == nil {
		_ = json.Unmarshal(raw, &base)
	}
	cfgRaw, _ := json.Marshal(s.cfg)
	var cfgMap map[string]any
	_ = json.Unmarshal(cfgRaw, &cfgMap)
	for k, v := range cfgMap {
		base[k] = v
	}
	out := make([]legacySessionOut, 0, len(sessions))
	var active *domain.Session
	for i := range sessions {
		ss := sessions[i]
		var history []map[string]any
		for _, m := range ss.Messages {
			if m.Role != domain.RoleUser && m.Role != domain.RoleAssistant {
				continue
			}
			x := map[string]any{"role": string(m.Role), "content": m.Content}
			if !m.CreatedAt.IsZero() {
				x["time"] = m.CreatedAt.UnixMilli()
			}
			if m.ReasoningContent != "" {
				x["reasoning"] = m.ReasoningContent
			}
			history = append(history, x)
		}
		out = append(out, legacySessionOut{
			ID: ss.ID, Name: ss.Name, AgentID: ss.AgentID, CWD: ss.CWD,
			UpdatedAt: ss.UpdatedAt.UnixMilli(), History: history, Conversation: ss.Messages,
			CreatedAt: ss.CreatedAt.UnixMilli(), ProviderID: ss.ProviderID, Model: ss.Model,
			ParentID: ss.ParentID, Metadata: ss.Metadata, Todos: ss.Todos, ToolActivities: ss.ToolActivities,
		})
		if ss.ID == activeID {
			active = &ss
		}
	}
	base["sessions"] = out
	base["activeSessionId"] = activeID
	base["storage"] = "config"
	if active == nil && len(sessions) > 0 {
		a := sessions[0]
		active = &a
	}
	if active != nil {
		var history []map[string]any
		for _, m := range active.Messages {
			if m.Role == domain.RoleUser || m.Role == domain.RoleAssistant {
				history = append(history, map[string]any{"role": string(m.Role), "content": m.Content, "reasoning": m.ReasoningContent})
			}
		}
		base["history"] = history
		base["conversation"] = active.Messages
	}
	return s.writeRawMap(base)
}

func (s *Store) ClearLegacySessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := map[string]any{}
	if raw, err := os.ReadFile(s.file); err == nil {
		_ = json.Unmarshal(raw, &base)
	}
	for _, k := range []string{"sessions", "history", "conversation"} {
		delete(base, k)
	}
	return s.writeRawMap(base)
}

func (s *Store) writeRawMap(base map[string]any) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(s.dir, ".config-raw-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.file)
}
