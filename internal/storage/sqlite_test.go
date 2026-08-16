package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func testSession() domain.Session {
	now := time.Now().Truncate(time.Millisecond)
	exit := 0
	return domain.Session{ID: "s1", Name: "scheduler fix", CreatedAt: now, UpdatedAt: now, ProviderID: "deepseek", Model: "deepseek-v4-flash", AgentID: "coding", CWD: "/tmp/project", Metadata: map[string]any{"effort": "high"}, Messages: []domain.Message{{ID: "m1", Role: domain.RoleUser, Content: "hello", CreatedAt: now}, {ID: "m2", Role: domain.RoleAssistant, Content: "world", ReasoningContent: "reason", CreatedAt: now}}, Todos: []domain.Todo{{ID: "t1", Subject: "test", Status: domain.TodoCompleted, UpdatedAt: now}}, ToolActivities: []domain.ToolActivity{{ID: "a1", CallID: "c1", Name: "bash", Arguments: json.RawMessage(`{"cmd":"true"}`), Output: "ok", ExitCode: &exit, State: "success", StartedAt: now, EndedAt: now}}}
}

func TestSQLiteRoundTripSearchDelete(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "ux-agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := testSession()
	if err := db.SaveSession(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := db.LoadSession(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.ProviderID != want.ProviderID || len(got.Messages) != 2 || len(got.Todos) != 1 || len(got.ToolActivities) != 1 {
		t.Fatalf("roundtrip mismatch: %#v", got)
	}
	ss, err := db.SearchSessions(ctx, "scheduler", 10)
	if err != nil || len(ss) != 1 {
		t.Fatalf("search=%d err=%v", len(ss), err)
	}
	if err := db.DeleteSession(ctx, want.ID); err != nil {
		t.Fatal(err)
	}
	ss, err = db.ListSessions(ctx)
	if err != nil || len(ss) != 0 {
		t.Fatalf("after delete=%d err=%v", len(ss), err)
	}
}

func TestMigrateLegacyConfigBacksUpAndVerifies(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacy := map[string]any{"model": "legacy-model", "activeProvider": "ux-gateway", "activeSessionId": "old-1", "storage": "config", "sessions": []map[string]any{{"id": "old-1", "name": "old", "agentId": "build", "cwd": dir, "updatedAt": time.Now().UnixMilli(), "conversation": []map[string]any{{"role": "user", "content": "old prompt"}, {"role": "assistant", "content": "old answer"}}}}}
	raw, _ := json.MarshalIndent(legacy, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenSQLite(ctx, filepath.Join(dir, "ux-agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	report, err := MigrateLegacyConfig(ctx, cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Detected != 1 || report.Migrated != 1 || report.Verified != 1 || report.Backup == "" {
		t.Fatalf("report=%+v", report)
	}
	if _, err := os.Stat(report.Backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	got, err := db.LoadSession(ctx, "old-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[0].Content != "old prompt" {
		t.Fatalf("messages=%#v", got.Messages)
	}
	if cfg.Snapshot().Storage != "db" || cfg.Snapshot().ActiveSessionID != "old-1" {
		t.Fatalf("config not switched: %#v", cfg.Snapshot())
	}
	// Successful migration writes the new config shape, so stale legacy session arrays cannot overwrite newer DB data on the next boot.
	snap, err := config.ReadLegacySessions(cfg.File())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 0 {
		t.Fatalf("legacy sessions remained after migration: %d", len(snap.Sessions))
	}
}

func BenchmarkSQLiteSessionLoad1000Messages(b *testing.B) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	s := testSession()
	s.Messages = make([]domain.Message, 1000)
	for i := range s.Messages {
		s.Messages[i] = domain.Message{ID: "m", Role: domain.RoleUser, Content: "benchmark message payload"}
	}
	if err := db.SaveSession(ctx, s); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := db.LoadSession(ctx, s.ID); err != nil {
			b.Fatal(err)
		}
	}
}

func TestImportSessionsIsAtomic(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "ux-agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	good := testSession()
	good.ID = "good"
	bad := testSession()
	bad.ID = ""
	if err := db.ImportSessions(ctx, []domain.Session{good, bad}); err == nil {
		t.Fatal("expected import failure")
	}
	ss, err := db.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 0 {
		t.Fatalf("partial migration committed: %#v", ss)
	}
}

func TestEscapeLikeLiteralWildcards(t *testing.T) {
	got := escapeLike(`100%_done\path`)
	if got != `100\%\_done\\path` {
		t.Fatalf("escapeLike=%q", got)
	}
}
