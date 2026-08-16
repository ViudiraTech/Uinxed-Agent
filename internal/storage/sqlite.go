package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	_ "modernc.org/sqlite"
)

type SQLite struct {
	db   *sql.DB
	path string
}

func OpenSQLite(ctx context.Context, path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(2 * time.Minute)
	s := &SQLite{db: db, path: path}
	if err := s.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) Path() string { return s.path }

func (s *SQLite) configure(ctx context.Context) error {
	for _, q := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA synchronous=NORMAL`,
	} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("sqlite pragma: %w", err)
		}
	}
	return s.db.PingContext(ctx)
}

func (s *SQLite) migrate(ctx context.Context) error {
	legacy, err := s.hasLegacySessions(ctx)
	if err != nil {
		return err
	}
	if legacy {
		// journal_mode=WAL is enabled before migration. Checkpoint before copying
		// the legacy database so the rollback artifact is self-contained and does
		// not depend on a transient -wal sidecar.
		if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return fmt.Errorf("checkpoint legacy db: %w", err)
		}
		if err := backupDB(s.path); err != nil {
			return fmt.Errorf("backup legacy db: %w", err)
		}
		if err := s.migrateLegacyV1(ctx); err != nil {
			return fmt.Errorf("legacy migration: %w", err)
		}
	}
	_, err = s.db.ExecContext(ctx, schema)
	return err
}

func (s *SQLite) hasLegacySessions(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&exists)
	if err != nil || exists == 0 {
		return false, err
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(sessions)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var hasHistory, hasCreated bool
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var def any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			return false, err
		}
		if name == "history" || name == "conversation" {
			hasHistory = true
		}
		if name == "created_at" {
			hasCreated = true
		}
	}
	return hasHistory && !hasCreated, rows.Err()
}

type legacyDBRow struct {
	ID           string
	Name         string
	AgentID      sql.NullString
	CWD          sql.NullString
	UpdatedAt    int64
	History      string
	Conversation string
}

func (s *SQLite) migrateLegacyV1(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,name,agent_id,cwd,updated_at,history,conversation FROM sessions ORDER BY updated_at`)
	if err != nil {
		return err
	}
	var legacy []legacyDBRow
	for rows.Next() {
		var r legacyDBRow
		if err := rows.Scan(&r.ID, &r.Name, &r.AgentID, &r.CWD, &r.UpdatedAt, &r.History, &r.Conversation); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, r)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE sessions RENAME TO legacy_sessions_v1`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return err
	}
	for _, r := range legacy {
		updated := time.UnixMilli(r.UpdatedAt)
		if r.UpdatedAt <= 0 {
			updated = time.Now()
		}
		sess := domain.Session{
			ID: r.ID, Name: r.Name, AgentID: r.AgentID.String, CWD: r.CWD.String,
			ProviderID: "ux-gateway", Model: "glm-4-flash", CreatedAt: updated, UpdatedAt: updated,
		}
		if sess.AgentID == "" {
			sess.AgentID = "build"
		}
		var msgs []domain.Message
		src := r.Conversation
		if strings.TrimSpace(src) == "" || src == "[]" {
			src = r.History
		}
		_ = json.Unmarshal([]byte(src), &msgs)
		sess.Messages = msgs
		if err := saveSessionTx(ctx, tx, sess); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	applied_at INTEGER NOT NULL
);
INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, unixepoch()*1000);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	provider_id TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	agent_id TEXT NOT NULL DEFAULT 'build',
	cwd TEXT NOT NULL DEFAULT '',
	parent_id TEXT NOT NULL DEFAULT '',
	metadata TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS sessions_updated_idx ON sessions(updated_at DESC);
CREATE INDEX IF NOT EXISTS sessions_name_idx ON sessions(name);

CREATE TABLE IF NOT EXISTS messages (
	session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	position INTEGER NOT NULL,
	id TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	content TEXT NOT NULL DEFAULT '',
	reasoning_content TEXT NOT NULL DEFAULT '',
	tool_calls TEXT NOT NULL DEFAULT '[]',
	tool_call_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(session_id, position)
);
CREATE INDEX IF NOT EXISTS messages_session_idx ON messages(session_id, position);

CREATE TABLE IF NOT EXISTS todos (
	session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	position INTEGER NOT NULL,
	id TEXT NOT NULL,
	subject TEXT NOT NULL,
	status TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL,
	PRIMARY KEY(session_id, position)
);

CREATE TABLE IF NOT EXISTS tool_activities (
	session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	position INTEGER NOT NULL,
	id TEXT NOT NULL,
	call_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	arguments TEXT NOT NULL DEFAULT '{}',
	output TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	exit_code INTEGER,
	state TEXT NOT NULL,
	started_at INTEGER NOT NULL,
	ended_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(session_id, position)
);
`

func (s *SQLite) ListSessions(ctx context.Context) ([]domain.Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,created_at,updated_at,provider_id,model,agent_id,cwd,parent_id,metadata FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		ss, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

func (s *SQLite) SearchSessions(ctx context.Context, query string, limit int) ([]domain.Session, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	q := "%" + escapeLike(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,created_at,updated_at,provider_id,model,agent_id,cwd,parent_id,metadata
		FROM sessions WHERE name LIKE ? ESCAPE '\' ORDER BY updated_at DESC LIMIT ?`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		ss, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

type scanner interface{ Scan(dest ...any) error }

func scanSession(sc scanner) (domain.Session, error) {
	var s domain.Session
	var created, updated int64
	var metadata string
	if err := sc.Scan(&s.ID, &s.Name, &created, &updated, &s.ProviderID, &s.Model, &s.AgentID, &s.CWD, &s.ParentID, &metadata); err != nil {
		return s, err
	}
	s.CreatedAt = time.UnixMilli(created)
	s.UpdatedAt = time.UnixMilli(updated)
	if metadata != "" {
		_ = json.Unmarshal([]byte(metadata), &s.Metadata)
	}
	return s, nil
}

func (s *SQLite) LoadSession(ctx context.Context, id string) (domain.Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,name,created_at,updated_at,provider_id,model,agent_id,cwd,parent_id,metadata FROM sessions WHERE id=?`, id)
	ss, err := scanSession(row)
	if err != nil {
		return ss, err
	}
	if err := s.loadMessages(ctx, &ss); err != nil {
		return ss, err
	}
	if err := s.loadTodos(ctx, &ss); err != nil {
		return ss, err
	}
	if err := s.loadActivities(ctx, &ss); err != nil {
		return ss, err
	}
	return ss, nil
}

func (s *SQLite) loadMessages(ctx context.Context, sess *domain.Session) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,role,content,reasoning_content,tool_calls,tool_call_id,name,created_at FROM messages WHERE session_id=? ORDER BY position`, sess.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var m domain.Message
		var role, tc string
		var created int64
		if err := rows.Scan(&m.ID, &role, &m.Content, &m.ReasoningContent, &tc, &m.ToolCallID, &m.Name, &created); err != nil {
			return err
		}
		m.Role = domain.Role(role)
		if tc != "" {
			_ = json.Unmarshal([]byte(tc), &m.ToolCalls)
		}
		if created > 0 {
			m.CreatedAt = time.UnixMilli(created)
		}
		sess.Messages = append(sess.Messages, m)
	}
	return rows.Err()
}

func (s *SQLite) loadTodos(ctx context.Context, sess *domain.Session) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,subject,status,reason,updated_at FROM todos WHERE session_id=? ORDER BY position`, sess.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var t domain.Todo
		var status string
		var updated int64
		if err := rows.Scan(&t.ID, &t.Subject, &status, &t.Reason, &updated); err != nil {
			return err
		}
		t.Status = domain.TodoStatus(status)
		t.UpdatedAt = time.UnixMilli(updated)
		sess.Todos = append(sess.Todos, t)
	}
	return rows.Err()
}

func (s *SQLite) loadActivities(ctx context.Context, sess *domain.Session) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,call_id,name,arguments,output,error,exit_code,state,started_at,ended_at FROM tool_activities WHERE session_id=? ORDER BY position`, sess.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a domain.ToolActivity
		var args string
		var exit sql.NullInt64
		var st, en int64
		if err := rows.Scan(&a.ID, &a.CallID, &a.Name, &args, &a.Output, &a.Error, &exit, &a.State, &st, &en); err != nil {
			return err
		}
		a.Arguments = json.RawMessage(args)
		a.StartedAt = time.UnixMilli(st)
		if en > 0 {
			a.EndedAt = time.UnixMilli(en)
		}
		if exit.Valid {
			v := int(exit.Int64)
			a.ExitCode = &v
		}
		sess.ToolActivities = append(sess.ToolActivities, a)
	}
	return rows.Err()
}

func (s *SQLite) SaveSession(ctx context.Context, sess domain.Session) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveSessionTx(ctx, tx, sess); err != nil {
		return err
	}
	return tx.Commit()
}

// ImportSessions persists a migration batch atomically. Either every session and
// its messages/todos/tool activity is committed, or the database is unchanged.
func (s *SQLite) ImportSessions(ctx context.Context, sessions []domain.Session) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, sess := range sessions {
		if err := saveSessionTx(ctx, tx, sess); err != nil {
			return fmt.Errorf("save session %s: %w", sess.ID, err)
		}
	}
	for _, sess := range sessions {
		var messages, todos, activities int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id=?`, sess.ID).Scan(&messages); err != nil {
			return fmt.Errorf("verify session %s messages: %w", sess.ID, err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM todos WHERE session_id=?`, sess.ID).Scan(&todos); err != nil {
			return fmt.Errorf("verify session %s todos: %w", sess.ID, err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_activities WHERE session_id=?`, sess.ID).Scan(&activities); err != nil {
			return fmt.Errorf("verify session %s tool activities: %w", sess.ID, err)
		}
		if messages != len(sess.Messages) || todos != len(sess.Todos) || activities != len(sess.ToolActivities) {
			return fmt.Errorf("verify session %s counts: messages %d/%d todos %d/%d activities %d/%d",
				sess.ID, messages, len(sess.Messages), todos, len(sess.Todos), activities, len(sess.ToolActivities))
		}
	}
	return tx.Commit()
}

func saveSessionTx(ctx context.Context, tx *sql.Tx, sess domain.Session) error {
	now := time.Now()
	if sess.ID == "" {
		return errors.New("session ID required")
	}
	if sess.Name == "" {
		sess.Name = sess.ID
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = now
	}
	if sess.AgentID == "" {
		sess.AgentID = "build"
	}
	meta, _ := json.Marshal(sess.Metadata)
	_, err := tx.ExecContext(ctx, `INSERT INTO sessions(id,name,created_at,updated_at,provider_id,model,agent_id,cwd,parent_id,metadata)
	VALUES(?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET name=excluded.name,updated_at=excluded.updated_at,provider_id=excluded.provider_id,
	model=excluded.model,agent_id=excluded.agent_id,cwd=excluded.cwd,parent_id=excluded.parent_id,metadata=excluded.metadata`,
		sess.ID, sess.Name, sess.CreatedAt.UnixMilli(), sess.UpdatedAt.UnixMilli(), sess.ProviderID, sess.Model, sess.AgentID, sess.CWD, sess.ParentID, string(meta))
	if err != nil {
		return err
	}
	for _, table := range []string{"messages", "todos", "tool_activities"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE session_id=?", sess.ID); err != nil {
			return err
		}
	}
	ms, err := tx.PrepareContext(ctx, `INSERT INTO messages(session_id,position,id,role,content,reasoning_content,tool_calls,tool_call_id,name,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ms.Close()
	for i, m := range sess.Messages {
		tc, _ := json.Marshal(m.ToolCalls)
		created := int64(0)
		if !m.CreatedAt.IsZero() {
			created = m.CreatedAt.UnixMilli()
		}
		if _, err := ms.ExecContext(ctx, sess.ID, i, m.ID, string(m.Role), m.Content, m.ReasoningContent, string(tc), m.ToolCallID, m.Name, created); err != nil {
			return err
		}
	}
	ts, err := tx.PrepareContext(ctx, `INSERT INTO todos(session_id,position,id,subject,status,reason,updated_at) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ts.Close()
	for i, t := range sess.Todos {
		if t.UpdatedAt.IsZero() {
			t.UpdatedAt = now
		}
		if _, err := ts.ExecContext(ctx, sess.ID, i, t.ID, t.Subject, string(t.Status), t.Reason, t.UpdatedAt.UnixMilli()); err != nil {
			return err
		}
	}
	as, err := tx.PrepareContext(ctx, `INSERT INTO tool_activities(session_id,position,id,call_id,name,arguments,output,error,exit_code,state,started_at,ended_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer as.Close()
	for i, a := range sess.ToolActivities {
		st, en := int64(0), int64(0)
		if !a.StartedAt.IsZero() {
			st = a.StartedAt.UnixMilli()
		}
		if !a.EndedAt.IsZero() {
			en = a.EndedAt.UnixMilli()
		}
		args := string(a.Arguments)
		if args == "" {
			args = "{}"
		}
		if _, err := as.ExecContext(ctx, sess.ID, i, a.ID, a.CallID, a.Name, args, a.Output, a.Error, a.ExitCode, a.State, st, en); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id)
	return err
}
func (s *SQLite) DeleteAllSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions`)
	return err
}
func (s *SQLite) Close() error { return s.db.Close() }

func backupDB(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	dst := path + ".pre-go-migration." + time.Now().UTC().Format("20060102T150405Z") + ".bak"
	return os.WriteFile(dst, raw, 0o600)
}
