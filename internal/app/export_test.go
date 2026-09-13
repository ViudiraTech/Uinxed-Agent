package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"database/sql"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func testCtx() context.Context { return context.Background() }

// memExportStore is a minimal in-memory storage.Store for export tests.
type memExportStore struct {
	mu sync.Mutex
	s  map[string]domain.Session
}

func newMemStoreForExport() *memExportStore { return &memExportStore{s: map[string]domain.Session{}} }

func (m *memExportStore) ListSessions(context.Context) ([]domain.Session, error) { return nil, nil }
func (m *memExportStore) SearchSessions(context.Context, string, int) ([]domain.Session, error) {
	return nil, nil
}
func (m *memExportStore) LoadSession(_ context.Context, id string) (domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.s[id]
	if !ok {
		return domain.Session{}, sql.ErrNoRows
	}
	return s.Clone(), nil
}
func (m *memExportStore) SaveSession(_ context.Context, s domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s[s.ID] = s.Clone()
	return nil
}
func (m *memExportStore) DeleteSession(_ context.Context, id string) error { return nil }
func (m *memExportStore) DeleteAllSessions(context.Context) error          { return nil }
func (m *memExportStore) Close() error                                     { return nil }

// TestSessionMarkdownFencesAndSections covers the export format: title header,
// one section per message, bodies fenced so embedded markdown cannot leak out.
func TestSessionMarkdownFencesAndSections(t *testing.T) {
	s := domain.Session{
		ID: "s1", Name: "Export me", AgentID: "build", Model: "m1",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Content: "read the file"},
			{Role: domain.RoleAssistant, Content: "## heading inside\n```go\ncode()\n```\ndone"},
			{Role: domain.RoleTool, Name: "read_file", Content: `{"content":"x"}`},
			{Role: domain.RoleSystem, Content: "must not appear"},
			{Role: domain.RoleAssistant, Content: ""},
		},
	}
	out := sessionMarkdown(s)
	if !strings.HasPrefix(out, "# Export me\n") {
		t.Errorf("missing title header:\n%s", out)
	}
	for _, want := range []string{"## user", "## assistant", "## tool · read_file"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing section %q", want)
		}
	}
	if strings.Contains(out, "must not appear") {
		t.Error("system message leaked into export")
	}
	if !strings.Contains(out, "_(no content)_") {
		t.Error("empty message body not rendered as placeholder")
	}
	// The assistant body contains a ``` fence; the export must wrap it in a
	// longer fence so the inner fence does not terminate the block early.
	if !strings.Contains(out, "````") {
		t.Error("inner fence not protected by a longer fence")
	}
	if strings.Count(out, "````\n")%2 != 0 {
		t.Error("unbalanced outer fence")
	}
}

func TestExportSessionWritesDefaultPath(t *testing.T) {
	dir := t.TempDir()
	s := domain.Session{
		ID: "s1", Name: "My Session", AgentID: "build", Model: "m", CWD: dir,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "hello"}},
	}
	// A store-only controller is enough: ExportSession touches Store and disk.
	st := newMemStoreForExport()
	_ = st.SaveSession(nil, s)
	c := &Controller{Store: st}
	path, err := c.ExportSession(testCtx(), "s1", "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("default path %q is not in the session CWD", path)
	}
	if filepath.Base(path) != "My-Session.md" {
		t.Fatalf("default filename = %q", filepath.Base(path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "# My Session\n") {
		t.Fatalf("content=%q", raw)
	}
	// An explicit relative path resolves inside the session CWD.
	rel, err := c.ExportSession(testCtx(), "s1", "out/export.md")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(rel) != "export.md" {
		t.Fatalf("explicit path = %q", rel)
	}
	if _, err := os.Stat(rel); err != nil {
		t.Fatalf("explicit export not written: %v", err)
	}
}

func TestExportSessionRejectsUnknownSession(t *testing.T) {
	c := &Controller{Store: newMemStoreForExport()}
	if _, err := c.ExportSession(testCtx(), "missing", ""); err == nil {
		t.Fatal("export of unknown session accepted")
	}
}
