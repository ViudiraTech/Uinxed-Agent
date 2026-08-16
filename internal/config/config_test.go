package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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
