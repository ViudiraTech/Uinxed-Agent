package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func TestSecretRoundTripAndNoPlaintext(t *testing.T) {
	plain := "sk-test-super-secret"
	enc, err := EncryptSecret(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain || enc == "" {
		t.Fatalf("secret was not encrypted: %q", enc)
	}
	got, err := DecryptSecret(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("roundtrip = %q, want %q", got, plain)
	}
}

func TestLegacyConfigPreservesFalseThinkingAndDefaultsNewUI(t *testing.T) {
	dir := t.TempDir()
	raw := map[string]any{
		"model": "legacy-model", "thinking": false, "activeProvider": "custom",
		"providers": []map[string]any{{"id": "custom", "name": "Custom", "baseUrl": "http://localhost:9999/v1", "models": []string{"legacy-model"}, "defaultModel": "legacy-model"}},
	}
	b, _ := json.Marshal(raw)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Snapshot()
	if cfg.Thinking {
		t.Fatal("legacy explicit thinking=false was lost")
	}
	if !cfg.Mouse || !cfg.Animations {
		t.Fatal("new UI options must default on for legacy config")
	}
	if cfg.Version != 2 {
		t.Fatalf("version=%d", cfg.Version)
	}
	if cfg.ActiveProvider != "custom" || cfg.Model != "legacy-model" {
		t.Fatalf("legacy provider/model lost: %#v", cfg)
	}
}

func TestProviderKeyWrittenEncrypted(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(c *Config) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProviderKey("deepseek", "sk-plain-must-not-appear"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.File())
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("empty config")
	}
	if contains := string(raw); len(contains) > 0 && (stringContains(contains, "sk-plain-must-not-appear")) {
		t.Fatal("plaintext API key leaked to config")
	}
	got, err := s.ProviderKey("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sk-plain-must-not-appear" {
		t.Fatalf("key=%q", got)
	}
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestLegacyHistoryPreservesReasoningAndTime(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC).UnixMilli()
	raw := fmt.Sprintf(`{"history":[{"role":"assistant","content":"answer","reasoning":"legacy reason","time":%d}],"model":"legacy","activeProvider":"deepseek"}`, ts)
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := ReadLegacySessions(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 1 || len(snap.Sessions[0].Messages) != 1 {
		t.Fatalf("snapshot=%#v", snap)
	}
	m := snap.Sessions[0].Messages[0]
	if m.ReasoningContent != "legacy reason" || m.CreatedAt.UnixMilli() != ts {
		t.Fatalf("legacy message not preserved: %#v", m)
	}
}

// TestLegacySessionsRoundTripFullSession guards a data-loss bug: the config
// storage path used to persist only the original legacy fields, silently
// dropping per-session metadata, provider/model overrides, parentage, todos and
// tool activities. Anything missing here means switching storage modes loses
// user data.
func TestLegacySessionsRoundTripFullSession(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	updated := created.Add(3 * time.Hour)
	in := domain.Session{
		ID: "s-1", Name: "auth refactor", AgentID: "coding",
		ProviderID: "router", Model: "step-3.7-flash", CWD: "/tmp/project",
		ParentID: "s-parent", CreatedAt: created, UpdatedAt: updated,
		Metadata: map[string]any{"effort": "xhigh", "thinking": false, "autotitled": true},
		Messages: []domain.Message{
			{ID: "m-1", Role: domain.RoleUser, Content: "hi", CreatedAt: created},
			{ID: "m-2", Role: domain.RoleAssistant, Content: "hello", ReasoningContent: "why", CreatedAt: created},
			{ID: "m-3", Role: domain.RoleTool, Content: "{}", ToolCallID: "call-1", Name: "bash"},
		},
		Todos:          []domain.Todo{{ID: "t-1", Subject: "extract session", Status: domain.TodoInProgress, Reason: "started"}},
		ToolActivities: []domain.ToolActivity{{ID: "a-1", CallID: "call-1", Name: "bash", State: "success", Output: "ok"}},
	}
	if err := s.SaveLegacySessions([]domain.Session{in}, in.ID); err != nil {
		t.Fatal(err)
	}
	snap, err := ReadLegacySessions(s.File())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 1 {
		t.Fatalf("sessions=%d", len(snap.Sessions))
	}
	got := snap.Sessions[0]

	if got.ID != in.ID || got.Name != in.Name || got.ParentID != in.ParentID {
		t.Errorf("identity lost: %#v", got)
	}
	if got.ProviderID != in.ProviderID || got.Model != in.Model || got.CWD != in.CWD {
		t.Errorf("routing lost: provider=%q model=%q cwd=%q", got.ProviderID, got.Model, got.CWD)
	}
	if !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(updated) {
		t.Errorf("timestamps lost: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
	if got.Metadata["effort"] != "xhigh" || got.Metadata["autotitled"] != true {
		t.Errorf("metadata lost: %#v", got.Metadata)
	}
	if _, ok := got.Metadata["thinking"].(bool); !ok {
		t.Errorf("explicit false metadata must survive: %#v", got.Metadata)
	}
	if len(got.Messages) != 3 || got.Messages[0].ID != "m-1" || got.Messages[1].ReasoningContent != "why" {
		t.Errorf("messages lost: %#v", got.Messages)
	}
	if len(got.Todos) != 1 || got.Todos[0].Status != domain.TodoInProgress {
		t.Errorf("todos lost: %#v", got.Todos)
	}
	if len(got.ToolActivities) != 1 || got.ToolActivities[0].Output != "ok" {
		t.Errorf("tool activities lost: %#v", got.ToolActivities)
	}
}
