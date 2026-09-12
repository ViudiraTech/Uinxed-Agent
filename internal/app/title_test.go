package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// titleServer stands in for the model endpoint, replying with a fixed title and
// recording how many completion requests it saw.
func titleServer(t *testing.T, reply string, requests *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if requests != nil {
			*requests++
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"model":"m","choices":[{"delta":{"content":`+jsonString(reply)+`}}]}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
}

func jsonString(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\n':
			b = append(b, '\\', 'n')
		default:
			b = append(b, []byte(string(r))...)
		}
	}
	return string(append(b, '"'))
}

// titleController wires a Controller to a stub model endpoint in config storage
// mode so no real provider or database is needed.
func titleController(t *testing.T, baseURL string) *Controller {
	t.Helper()
	cfg, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Update(func(c *config.Config) error {
		c.Storage = "config"
		c.Providers = []config.Provider{{
			ID: "test", Name: "test", BaseURL: baseURL,
			Models: []string{"m"}, DefaultModel: "m", Builtin: true,
		}}
		c.ActiveProvider = "test"
		c.Model = "m"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctrl, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ctrl.Close() })
	return ctrl
}

func seedExchange(t *testing.T, ctrl *Controller, name string) domain.Session {
	t.Helper()
	ctx := context.Background()
	s, err := ctrl.NewSession(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	s.Messages = []domain.Message{
		{ID: "u1", Role: domain.RoleUser, Content: "帮我重构 auth 模块，把 session 处理拆出去"},
		{ID: "a1", Role: domain.RoleAssistant, Content: "好的，我先读一遍现有实现。"},
	}
	if err := ctrl.Store.SaveSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAutoTitleSessionNamesFromFirstExchange(t *testing.T) {
	srv := titleServer(t, "重构认证模块", nil)
	defer srv.Close()
	ctrl := titleController(t, srv.URL+"/v1")
	s := seedExchange(t, ctrl, "Session 1")

	title, err := ctrl.AutoTitleSession(context.Background(), s.ID)
	if err != nil {
		t.Fatalf("auto title: %v", err)
	}
	if title != "重构认证模块" {
		t.Fatalf("title = %q", title)
	}
	got, err := ctrl.LoadSession(context.Background(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "重构认证模块" {
		t.Fatalf("persisted name = %q", got.Name)
	}
	if titled, _ := got.Metadata["autotitled"].(bool); !titled {
		t.Fatal("session should be marked as titled so it never runs twice")
	}
}

func TestAutoTitleSessionRunsAtMostOnce(t *testing.T) {
	requests := 0
	srv := titleServer(t, "标题", &requests)
	defer srv.Close()
	ctrl := titleController(t, srv.URL+"/v1")
	ctx := context.Background()
	s := seedExchange(t, ctrl, "Session 1")

	if _, err := ctrl.AutoTitleSession(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	title, err := ctrl.AutoTitleSession(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if title != "" {
		t.Fatalf("second call should be a no-op, got %q", title)
	}
	if requests != 1 {
		t.Fatalf("model was asked %d times, want 1", requests)
	}
}

func TestAutoTitleSessionSkipsIncompleteOrRenamedSessions(t *testing.T) {
	requests := 0
	srv := titleServer(t, "标题", &requests)
	defer srv.Close()
	ctrl := titleController(t, srv.URL+"/v1")
	ctx := context.Background()

	// A session with a question but no answer yet must not be titled.
	pending, err := ctrl.NewSession(ctx, "Session 2")
	if err != nil {
		t.Fatal(err)
	}
	pending.Messages = []domain.Message{{ID: "u", Role: domain.RoleUser, Content: "只有提问"}}
	if err := ctrl.Store.SaveSession(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if title, _ := ctrl.AutoTitleSession(ctx, pending.ID); title != "" {
		t.Fatalf("unanswered session was titled %q", title)
	}

	// A name the user chose is theirs to keep.
	renamed := seedExchange(t, ctrl, "my own name")
	if title, _ := ctrl.AutoTitleSession(ctx, renamed.ID); title != "" {
		t.Fatalf("renamed session was overwritten with %q", title)
	}
	if requests != 0 {
		t.Fatalf("no model request should have been made, got %d", requests)
	}
}

func TestAutoTitleSessionSkipsSubagentSessions(t *testing.T) {
	requests := 0
	srv := titleServer(t, "标题", &requests)
	defer srv.Close()
	ctrl := titleController(t, srv.URL+"/v1")
	ctx := context.Background()
	s := seedExchange(t, ctrl, "Session 3")
	s.ParentID = "s-parent"
	if err := ctrl.Store.SaveSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	if title, _ := ctrl.AutoTitleSession(ctx, s.ID); title != "" {
		t.Fatalf("subagent session was titled %q", title)
	}
	if requests != 0 {
		t.Fatalf("no model request should have been made, got %d", requests)
	}
}

func TestCleanTitleStripsModelFormatting(t *testing.T) {
	cases := map[string]string{
		"  重构认证模块  ":               "重构认证模块",
		"\"重构认证模块\"":               "重构认证模块",
		"**Refactor auth module**": "Refactor auth module",
		"标题：重构认证模块。":               "标题：重构认证模块",
		"first line\nsecond line":  "first line",
		"`code title`":             "code title",
		"":                         "",
	}
	for in, want := range cases {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsDefaultSessionName(t *testing.T) {
	for _, name := range []string{"", "Session 1", "Session 42", "会话 3", "New session", "新会话"} {
		if !isDefaultSessionName(name) {
			t.Errorf("%q should count as a generated name", name)
		}
	}
	for _, name := range []string{"auth refactor", "Session", "Session x", "my Session 1"} {
		if isDefaultSessionName(name) {
			t.Errorf("%q should count as a user-chosen name", name)
		}
	}
}
