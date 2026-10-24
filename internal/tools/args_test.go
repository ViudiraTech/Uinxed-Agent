package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func execRaw(t *testing.T, tool Tool, cwd, args string) (Result, error) {
	t.Helper()
	return tool.Execute(context.Background(), json.RawMessage(args), ExecutionContext{CWD: cwd})
}

// A model carrying Claude Code's bash schema asks for `command`, not `cmd`, and
// usually sends a `description` beside it. That call used to die in the JSON
// decoder with `unknown field "command"`; it has to run instead.
func TestBashFoldsCommandAliasAndIgnoresExtraFields(t *testing.T) {
	res, err := execRaw(t, &BashTool{}, t.TempDir(), `{"command":"echo alias-ok","description":"say it"}`)
	if err != nil {
		t.Fatalf("command alias rejected: %v", err)
	}
	if !strings.Contains(res.Content, "alias-ok") {
		t.Fatalf("output = %q, want the command's stdout", res.Content)
	}
}

func TestEditFoldsFileAndStringAliases(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execRaw(t, &EditFileTool{}, root, `{"file_path":"a.txt","old_string":"before","new_string":"after"}`); err != nil {
		t.Fatalf("claude-style edit rejected: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after\n" {
		t.Fatalf("content = %q, want the edit applied", got)
	}
}

func TestGrepFoldsGlobAliasOntoInclude(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("// TODO go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("TODO text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := execRaw(t, &GrepTool{}, root, `{"pattern":"TODO","glob":"*.go"}`)
	if err != nil {
		t.Fatalf("glob alias rejected: %v", err)
	}
	if !strings.Contains(res.Content, "a.go") || strings.Contains(res.Content, "b.txt") {
		t.Fatalf("include filter not applied: %q", res.Content)
	}
}

// A call that sends both spellings must not silently keep the stale one.
func TestCanonicalArgumentWinsOverAlias(t *testing.T) {
	var a struct {
		Cmd string `json:"cmd"`
	}
	if err := decode(json.RawMessage(`{"cmd":"canonical","command":"alias"}`), &a); err != nil {
		t.Fatal(err)
	}
	if a.Cmd != "canonical" {
		t.Fatalf("cmd = %q, want the canonical value", a.Cmd)
	}
}

// Tolerating spellings must not turn a missing required argument into a silent
// no-op: the tool still names the field the model has to resend.
func TestMissingRequiredArgumentStillFails(t *testing.T) {
	_, err := execRaw(t, &BashTool{}, t.TempDir(), `{"argv":["echo","hi"]}`)
	if err == nil {
		t.Fatal("bash without a command unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "cmd") {
		t.Fatalf("error should name the field to resend, got %v", err)
	}
}

func TestMalformedArgumentsStillFail(t *testing.T) {
	if _, err := execRaw(t, &BashTool{}, t.TempDir(), `{"cmd":`); err == nil {
		t.Fatal("truncated JSON unexpectedly decoded")
	}
}

func TestEmptyAndNullArgumentsDecodeToDefaults(t *testing.T) {
	type args struct {
		Path string `json:"path"`
	}
	for _, raw := range []string{"", "null", "{}"} {
		var a args
		if err := decode(json.RawMessage(raw), &a); err != nil {
			t.Fatalf("decode(%q) = %v", raw, err)
		}
		if a.Path != "" {
			t.Fatalf("decode(%q) left %q", raw, a.Path)
		}
	}
}
