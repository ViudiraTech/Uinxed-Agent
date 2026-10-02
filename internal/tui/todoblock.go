package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

// maxTodoRows caps the task list so a long plan cannot push the transcript off
// the screen. The remainder collapses into a count that points at /todos.
const maxTodoRows = 5

// todoRowsShown is how many entries the block renders. Kept separate from the
// renderer because renderBase has to reserve the height before anything is
// drawn, and the two must agree exactly or the view drifts.
func todoRowsShown(n int) int { return min(n, maxTodoRows) }

// todoBlockHeight is the number of rows the task list will occupy.
func (m *Model) todoBlockHeight() int {
	n := len(m.session.Todos)
	if n == 0 {
		return 0
	}
	h := 1 + todoRowsShown(n) // header + entries
	if n > maxTodoRows {
		h++ // the "… +N more" line
	}
	return h
}

// renderTodoBlock draws the task list directly above the composer, which is
// where Claude Code keeps it: beside the input rather than in a side panel, so
// it stays visible while the user is typing and costs no horizontal room.
//
// An empty list renders nothing at all — the block is not permanent chrome, and
// a session with no plan should not reserve a row for one.
func (m *Model) renderTodoBlock(t Theme, w, baseY int) []string {
	todos := m.session.Todos
	if len(todos) == 0 || w <= 0 {
		return nil
	}
	g := t.Glyphs
	done := 0
	for _, x := range todos {
		if x.Status == "completed" {
			done++
		}
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(t.Muted)
	out := []string{fitLine(" "+header.Render(fmt.Sprintf("TODOS %d/%d", done, len(todos))), w)}

	shown := 0
	for _, x := range todos {
		if shown >= maxTodoRows {
			more := lipgloss.NewStyle().Foreground(t.Muted).Render(
				fmt.Sprintf("  … +%d more (/todos)", len(todos)-shown))
			out = append(out, fitLine(more, w))
			break
		}
		icon, iStyle := g.TodoOpen, lipgloss.NewStyle().Foreground(t.Muted)
		switch x.Status {
		case "completed":
			icon, iStyle = g.TodoDone, lipgloss.NewStyle().Foreground(t.Success)
		case "in_progress":
			icon, iStyle = g.Bullet, lipgloss.NewStyle().Foreground(t.Warning)
		}
		line := "  " + iStyle.Render(icon) + " " + terminalutil.SanitizeText(x.Subject)
		out = append(out, fitLine(line, w))
		// Regions are produced during rendering, so the row they land on has to
		// be registered here — baseY is where this block starts on screen.
		m.regions = append(m.regions, Region{Rect: Rect{0, baseY + len(out) - 1, w, 1}, Kind: ActionTodo, Value: x.ID})
		shown++
	}
	return out
}
