package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// todoBlockHeight is consulted before anything renders, so it has to agree with
// what renderTodoBlock actually emits — a mismatch shows up as a view that
// drifts by a row as the task list grows.
func TestTodoBlockHeightMatchesWhatRenders(t *testing.T) {
	cases := []int{0, 1, 5, 6, 12}
	for _, n := range cases {
		m := layoutModel(t)
		m.session.Todos = nil
		for i := 0; i < n; i++ {
			m.session.Todos = append(m.session.Todos, domain.Todo{
				ID: fmt.Sprintf("td%d", i), Subject: fmt.Sprintf("task %d", i), Status: "pending",
			})
		}
		lines := m.renderTodoBlock(ThemeByName("claude"), 80, 0)
		if got := m.todoBlockHeight(); got != len(lines) {
			t.Fatalf("%d todos: height=%d but %d lines rendered", n, got, len(lines))
		}
	}
}

// A long plan must not push the transcript off the screen; the remainder is
// counted rather than listed.
func TestTodoBlockCapsLongLists(t *testing.T) {
	m := layoutModel(t)
	m.session.Todos = nil
	for i := 0; i < maxTodoRows+4; i++ {
		m.session.Todos = append(m.session.Todos, domain.Todo{ID: fmt.Sprintf("td%d", i), Subject: fmt.Sprintf("task %d", i)})
	}
	m.width, m.height = 100, 30
	m.resize()
	out := stripANSI(m.renderBase(themeFor(m.cfg)))

	if !strings.Contains(out, "TODOS 0/9") {
		t.Fatalf("header should still count every task:\n%s", out)
	}
	if !strings.Contains(out, "… +4 more (/todos)") {
		t.Fatalf("the overflow should be counted, not listed:\n%s", out)
	}
	if strings.Contains(out, fmt.Sprintf("task %d", maxTodoRows)) {
		t.Fatalf("a row past the cap leaked into the view:\n%s", out)
	}
}
