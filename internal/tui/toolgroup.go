package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// groupPrefix marks a clickable region that stands for a whole run of tools
// rather than a single call, so ToggleTool can open or close every member at
// once — which is what the one row visually represents.
const groupPrefix = "group:"

// groupableTools are the calls Claude Code folds into a single aggregate row. It
// is a curated list rather than tools.CategoryRead: that category also holds
// use_skill, and folding a skill activation into "Read 1 file" would misreport
// what the agent did.
//
// bash is included for fidelity with Claude Code even though it is the one entry
// that can mutate. Nothing becomes unreachable: opening the row renders every
// command and its full result, so a destructive call is hidden only until asked
// for, never dropped.
var groupableTools = map[string]bool{
	"read_file": true, "list_dir": true, "grep": true, "glob": true, "tree": true,
	"git_status": true, "git_diff": true, "git_log": true,
	"fetch_url": true, "web_search": true, "bash": true,
}

// toolRun is a maximal stretch of adjacent calls that render together. A run of
// one is not a group: a lone call keeps its own card and result line, because
// "Read 1 file" is a worse line than `Read(a.go)` followed by its result.
type toolRun struct {
	calls []domain.ToolCall
	group bool
}

// planToolRuns splits a turn's calls into rendered runs, preserving order.
func planToolRuns(calls []domain.ToolCall) []toolRun {
	var runs []toolRun
	for i := 0; i < len(calls); {
		if !groupableTools[calls[i].Function.Name] {
			runs = append(runs, toolRun{calls: calls[i : i+1]})
			i++
			continue
		}
		j := i
		for j < len(calls) && groupableTools[calls[j].Function.Name] {
			j++
		}
		runs = append(runs, toolRun{calls: calls[i:j], group: j-i > 1})
		i = j
	}
	return runs
}

// toolRows is the height the calls occupy: two rows per standalone call (its
// head and its result) and one for each grouped run. The virtualizer's estimate
// has to know this, because grouping changes a block's height while its content
// stays the same.
func toolRows(calls []domain.ToolCall) int {
	rows := 0
	for _, run := range planToolRuns(calls) {
		if run.group {
			rows++
			continue
		}
		rows += 2
	}
	return rows
}

// renderToolGroup draws a run as one row: a gerund while the work is in flight
// and a dim past-tense summary once it settles, which is how Claude Code answers
// "what is happening" and then "what happened".
func (c *Conversation) renderToolGroup(run toolRun, acts map[string]domain.ToolActivity, t Theme, width int, hover string, frame int) []renderLine {
	ids := make([]string, 0, len(run.calls))
	running, failed := false, false
	for _, tc := range run.calls {
		ids = append(ids, tc.ID)
		switch acts[tc.ID].State {
		case "running":
			running = true
		case "failed":
			failed = true
		}
	}
	value := groupPrefix + strings.Join(ids, ",")
	open := c.expandedTools[run.calls[0].ID]

	out := []renderLine{{Text: c.groupLine(run, t, hover, value, frame, running, failed, open), Action: ActionTool, Value: value}}
	if !open {
		return out
	}
	// Open: every member renders in full, so the aggregate never becomes a place
	// where a command or its output can go missing.
	for _, tc := range run.calls {
		out = append(out, c.renderToolCall(tc, acts[tc.ID], t, width, hover, frame)...)
	}
	return out
}

func (c *Conversation) groupLine(run toolRun, t Theme, hover, value string, frame int, running, failed, open bool) string {
	g := t.Glyphs
	icon, fg := g.Success, t.Muted
	switch {
	case running:
		icon, fg = g.spinner(frame), t.Warning
	case failed:
		icon, fg = g.Failure, t.Error
	}

	text := groupPhrase(run.calls, running)
	if running {
		text += "…"
	}
	style := lipgloss.NewStyle().Foreground(t.Muted)
	if failed {
		style = lipgloss.NewStyle().Foreground(t.Error)
	} else if running {
		style = lipgloss.NewStyle().Foreground(t.Text)
	}
	if hover == value {
		style = style.Underline(true)
	}

	line := lipgloss.NewStyle().Foreground(fg).Render(icon) + " " + style.Render(text)
	if open {
		line += lipgloss.NewStyle().Foreground(t.Muted).Render("  · details")
	}
	return line
}

// groupPhrase counts a run by what was done rather than by how many calls it
// held, so the row says "Reading 2 files, searching 1 pattern" instead of
// "3 tools".
func groupPhrase(calls []domain.ToolCall, running bool) string {
	var reads, searches, commands, trees, refs, web int
	for _, tc := range calls {
		switch tc.Function.Name {
		case "read_file", "list_dir":
			reads++
		case "grep", "glob":
			searches++
		case "bash":
			commands++
		case "tree":
			trees++
		case "git_status", "git_diff", "git_log":
			refs++
		case "fetch_url", "web_search":
			web++
		}
	}

	tense := func(present, past string) string {
		if running {
			return present
		}
		return past
	}
	part := func(verb string, n int, one, many string) string {
		return fmt.Sprintf("%s %d %s", verb, n, plural(n, one, many))
	}

	var parts []string
	if reads > 0 {
		parts = append(parts, part(tense("Reading", "Read"), reads, "file", "files"))
	}
	if searches > 0 {
		parts = append(parts, part(tense("Searching", "Searched"), searches, "pattern", "patterns"))
	}
	if commands > 0 {
		parts = append(parts, part(tense("Running", "Ran"), commands, "command", "commands"))
	}
	if trees > 0 {
		parts = append(parts, part(tense("Scanning", "Scanned"), trees, "tree", "trees"))
	}
	if refs > 0 {
		parts = append(parts, part(tense("Reading", "Read"), refs, "git query", "git queries"))
	}
	if web > 0 {
		parts = append(parts, part(tense("Fetching", "Fetched"), web, "page", "pages"))
	}
	// Only the leading clause keeps its capital; the rest read as a list.
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToLower(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, ", ")
}

// liveSubagentProgress renders what a running delegate's child is doing, which is
// information the sidebar used to carry. The tracked run holds the agent id and
// the task text but nothing that points back at the call which spawned it, so
// those two fields are the match. A run with an empty task is the top-level turn
// rather than a delegate, and is skipped.
//
// Attaching it to the existing call card rather than emitting a separate row is
// what keeps a background delegate from appearing twice on screen.
func (c *Conversation) liveSubagentProgress(tc domain.ToolCall, t Theme) string {
	text := c.liveSubagentProgressText(tc)
	if text == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(t.Muted).Render(" · " + text)
}

// liveSubagentProgressText is the unstyled form. Keeping it separate lets the
// render cache key read exactly what the renderer draws.
func (c *Conversation) liveSubagentProgressText(tc domain.ToolCall) string {
	if len(c.subagents) == 0 {
		return ""
	}
	agent, task := parseDelegateArgs(tc.Function.Arguments)
	task = strings.TrimSpace(task)
	for _, run := range c.subagents {
		if run.State != "running" || strings.TrimSpace(run.Task) == "" {
			continue
		}
		if agent != "" && run.AgentID != agent {
			continue
		}
		if task != "" && strings.TrimSpace(run.Task) != task {
			continue
		}
		prog := c.subagentProgress[run.ID]
		var parts []string
		if el := formatSubagentElapsed(run, 0); el != "" {
			parts = append(parts, el)
		}
		if prog.Tools > 0 {
			parts = append(parts, fmt.Sprintf("%d tools", prog.Tools))
		}
		if prog.LastTool != "" {
			parts = append(parts, "↳ "+prog.LastTool)
		}
		return strings.Join(parts, " · ")
	}
	return ""
}

// subagentSignature captures what a running delegate card reads from its child
// run, so a cached block cannot serve a stale status. It matters most when
// animations are off: the frame then stops changing, and without this the card
// would freeze on whatever progress it had when the tick last ran.
func (c *Conversation) subagentSignature(calls []domain.ToolCall) string {
	if len(c.subagents) == 0 {
		return ""
	}
	var b strings.Builder
	for _, tc := range calls {
		if tc.Function.Name != "delegate" {
			continue
		}
		if text := c.liveSubagentProgressText(tc); text != "" {
			b.WriteString(text)
			b.WriteByte(';')
		}
	}
	return b.String()
}
