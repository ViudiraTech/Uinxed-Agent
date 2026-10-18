package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// The pragmas are per-connection settings, so every connection the pool opens
// has to get them. A connection without foreign_keys leaves orphaned rows behind
// every cascade delete; one without busy_timeout fails a concurrent writer at once.
func TestPragmasApplyToEveryConnection(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	c1, err := db.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, err := db.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	for i, c := range []*sql.Conn{c1, c2} {
		var fk, bt int
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&bt); err != nil {
			t.Fatal(err)
		}
		if fk != 1 || bt == 0 {
			t.Fatalf("connection %d: foreign_keys=%d busy_timeout=%d", i+1, fk, bt)
		}
	}
	var mode string
	if err := c2.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

// Deleting a session relies on ON DELETE CASCADE. With foreign_keys off the
// session vanishes but its transcript stays in the database forever.
func TestDeleteCascadesWithForeignKeysOn(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	s := domain.Session{ID: "s", Name: "s", CreatedAt: now, UpdatedAt: now,
		Messages: []domain.Message{{ID: "m", Role: domain.RoleUser, Content: "x", CreatedAt: now}}}
	if err := db.SaveSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteSession(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id=?`, "s").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("delete left %d orphan message rows", n)
	}
}

// A settings change must not rewrite the transcript. Saving a whole stale session
// for one metadata key is what deleted a reply the running turn had just saved.
func TestSettingsUpdateKeepsMessages(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	s := domain.Session{ID: "s", Name: "s", CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]any{"mode": "auto-edit"},
		Messages: []domain.Message{{ID: "m1", Role: domain.RoleUser, Content: "first", CreatedAt: now}}}
	if err := db.SaveSession(ctx, s); err != nil {
		t.Fatal(err)
	}

	// The turn finishes and persists its reply.
	s.Messages = append(s.Messages, domain.Message{ID: "m2", Role: domain.RoleAssistant, Content: "answer", CreatedAt: now})
	if err := db.SaveSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	// A settings write, the way Controller.SetApprovalMode performs it.
	if err := db.UpdateSettings(ctx, "s", func(x *domain.Session) {
		x.Metadata["mode"] = "read-only"
	}); err != nil {
		t.Fatal(err)
	}

	got, err := db.LoadSession(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("the settings write changed the transcript: %d of 2 messages remain", len(got.Messages))
	}
	if got.Metadata["mode"] != "read-only" {
		t.Fatalf("setting not written: %v", got.Metadata)
	}
}

// The v1 (Node-era) database stored messages as {"role","content","reasoning",
// "time",...}. Decoding that straight into domain.Message — whose tags are
// reasoning_content/created_at — dropped both fields.
func TestLegacyMigrationKeepsReasoningAndTime(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE sessions(
		id TEXT PRIMARY KEY, name TEXT, agent_id TEXT, cwd TEXT, updated_at INTEGER,
		history TEXT, conversation TEXT)`); err != nil {
		t.Fatal(err)
	}
	history := `[{"role":"assistant","content":"answer","reasoning":"because","time":1700000000000}]`
	if _, err := raw.ExecContext(ctx, `INSERT INTO sessions(id,name,agent_id,cwd,updated_at,history,conversation)
		VALUES(?,?,?,?,?,?,?)`, "s1", "old session", "build", ".", 1700000000000, history, history); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.LoadSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages = %d", len(got.Messages))
	}
	m := got.Messages[0]
	if m.ReasoningContent != "because" {
		t.Fatalf("reasoning was dropped: %q", m.ReasoningContent)
	}
	if m.CreatedAt.IsZero() {
		t.Fatal("timestamp was dropped")
	}
}
