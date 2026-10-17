package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeInstruction(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func instructionPaths(files []InstructionFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// The two conventions have to coexist: a repository may ship AGENTS.md, CLAUDE.md
// or both, and the .ux-agent/ copies carry the agent-specific additions.
func TestLoadInstructionsFindsRepositoryFiles(t *testing.T) {
	dir := t.TempDir()
	writeInstruction(t, dir, "AGENTS.md", "# Agents\nrun make test")
	writeInstruction(t, dir, "CLAUDE.md", "# Claude\nuse tabs")
	writeInstruction(t, dir, ".ux-agent/AGENTS.md", "extra rules")

	files := LoadInstructions(dir)
	got := instructionPaths(files)
	want := []string{"AGENTS.md", "CLAUDE.md", ".ux-agent/AGENTS.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	if !strings.Contains(files[0].Body, "make test") {
		t.Fatalf("body was not read: %q", files[0].Body)
	}
	if !strings.Contains(files[2].Body, "extra rules") {
		t.Fatalf("nested file was not read: %q", files[2].Body)
	}
}

func TestLoadInstructionsNeedsAWorkingDirectory(t *testing.T) {
	if files := LoadInstructions(""); len(files) != 0 {
		t.Fatalf("empty cwd returned %d files", len(files))
	}
	if files := LoadInstructions(t.TempDir()); len(files) != 0 {
		t.Fatalf("an empty directory returned %d files", len(files))
	}
}

// A symlinked AGENTS.md is the one way these fixed names could read outside the
// sandbox, and a repository that points one at ~/.ssh must not leak it into the
// model's context.
func TestInstructionSymlinkCannotEscape(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("do not read me"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if files := LoadInstructions(dir); len(files) != 0 {
		t.Fatalf("read %d files through an escaping symlink", len(files))
	}

	// A symlink that stays inside the working directory is still a normal file.
	inside := filepath.Join(dir, "real.md")
	if err := os.WriteFile(inside, []byte("allowed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inside, filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	files := LoadInstructions(dir)
	if len(files) != 1 || files[0].Path != "CLAUDE.md" {
		t.Fatalf("internal symlink not read: %v", instructionPaths(files))
	}
}

// A document with a NUL byte is not instructions, it is a binary someone named
// AGENTS.md, and decoding it would put mojibake in the prompt.
func TestLoadInstructionsSkipsBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("abc\x00def"), 0o644); err != nil {
		t.Fatal(err)
	}
	if files := LoadInstructions(dir); len(files) != 0 {
		t.Fatalf("read a binary file: %v", instructionPaths(files))
	}
}

// Oversized files are cut at a line boundary and marked, so the model knows the
// document continues instead of guessing where it stopped.
func TestLoadInstructionsTruncatesLongFiles(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for b.Len() < instructionFileLimit*2 {
		b.WriteString("a line of instructions that repeats to fill the file\n")
	}
	writeInstruction(t, dir, "AGENTS.md", b.String())

	files := LoadInstructions(dir)
	if len(files) != 1 {
		t.Fatalf("got %d files", len(files))
	}
	if !files[0].Truncated {
		t.Fatal("a file over the limit was not marked truncated")
	}
	if len(files[0].Body) > instructionFileLimit+200 {
		t.Fatalf("body is %d bytes, over the limit", len(files[0].Body))
	}
	if strings.Contains(files[0].Body, "a line of instructions that repeats to fill the file\n(truncated") {
		t.Fatal("truncation note was glued to a partial line")
	}
	if !strings.Contains(files[0].Body, "(truncated:") {
		t.Fatalf("truncation was not explained: %q", files[0].Body[len(files[0].Body)-80:])
	}
}

// The block has to say what these documents are *not*: a checked-in file must not
// be able to talk the agent past the host's approval policy.
func TestInstructionsBlockNamesItsBoundary(t *testing.T) {
	dir := t.TempDir()
	writeInstruction(t, dir, "AGENTS.md", "always run make check")

	block := InstructionsBlock(dir)
	for _, want := range []string{"项目指令", "AGENTS.md", "always run make check", "安全策略"} {
		if !strings.Contains(block, want) {
			t.Fatalf("block is missing %q:\n%s", want, block)
		}
	}
	if block := InstructionsBlock(t.TempDir()); block != "" {
		t.Fatalf("a repository without instructions produced a block: %q", block)
	}
}

// The block is what the runtime threads into the system prompt, so the document
// has to survive the trip.
func TestSystemPromptCarriesProjectInstructions(t *testing.T) {
	dir := t.TempDir()
	writeInstruction(t, dir, "AGENTS.md", "run make check before pushing")

	prompt := SystemPromptMode(Get("build"), "test-model", InstructionsBlock(dir), false, "")
	if !strings.Contains(prompt, "run make check before pushing") {
		t.Fatalf("the project instruction is missing from the system prompt")
	}
	if !strings.Contains(prompt, "AGENTS.md") {
		t.Fatalf("the system prompt does not name the source file")
	}
}
