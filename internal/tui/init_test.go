package tui

import (
	"strings"
	"testing"
)

// /init's whole value is that the agent reads the repository instead of filling in
// a skeleton, so the prompt has to ask for verified commands, merging with existing
// instruction files, and a bounded document.
func TestInitPromptAsksForAVerifiedDocument(t *testing.T) {
	for _, want := range []string{
		"AGENTS.md",
		"Makefile",
		"CLAUDE.md",
		"出处",
		"不要徽章",
	} {
		if !strings.Contains(initPrompt, want) {
			t.Fatalf("init prompt is missing %q", want)
		}
	}
}

// A new session must not leave the composer holding the prompt: /init is a
// command, and what the user sees in the transcript is the agent's answer.
func TestInitDispatchesATurnWithoutTouchingTheComposer(t *testing.T) {
	m := layoutModel(t)
	m.session.CWD = t.TempDir()
	m.prompt.SetValue("")

	cmd := m.runInit()
	if cmd == nil {
		t.Fatal("/init dispatched nothing")
	}
	if got := strings.TrimSpace(m.prompt.Value()); got != "" {
		t.Fatalf("/init put %q in the composer", got)
	}
	if strings.Contains(strings.Join(m.history, "\n"), "AGENTS.md") {
		t.Fatal("/init leaked its prompt into the prompt history")
	}
}

// Plan mode refuses writes, so starting the turn would only produce a refused tool
// call. The command should say why instead.
func TestInitRefusesInPlanMode(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"mode": "plan"}
	if cmd := m.runInit(); cmd != nil {
		t.Fatal("/init started a turn in plan mode")
	}
	if !strings.Contains(strings.ToLower(m.toast), "plan") {
		t.Fatalf("toast = %q, want an explanation about plan mode", m.toast)
	}
}

// And a running turn is a single-slot resource: /init must not queue behind it
// silently.
func TestInitRefusesWhileATurnRuns(t *testing.T) {
	m := layoutModel(t)
	m.busy = true
	if cmd := m.runInit(); cmd != nil {
		t.Fatal("/init started a second turn")
	}
	if !strings.Contains(strings.ToLower(m.toast), "turn") {
		t.Fatalf("toast = %q, want an explanation about the running turn", m.toast)
	}
}
