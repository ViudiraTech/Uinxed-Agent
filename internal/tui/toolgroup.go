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

// groupableTools are the calls that fold into a single aggregate row. It is a
// curated list rather than tools.CategoryRead: that category also holds use_skill,
// and folding a skill activation into "Read 1 file" would misreport what the agent
// did.
//
// bash is deliberately NOT groupable. A group row says what kind of work happened
// ("read 2 files, searched 1 pattern"), which is the right summary for reads — but
// for a command the command itself is the information, and folding it into "ran 1
// command" hides the only part a reader needs to check.
var groupableTools = map[string]bool{
	"read_file": true, "list_dir": true, "grep": true, "glob": true, "tree": true,
	"git_status": true, "git_diff": true, "git_log": true,
	"fetch_url": true, "web_search": true,
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

// toolRows is the height the calls occupy. Every run — grouped or not — is one
// call row plus one summary row. Keeping the two cases the same height is what
// lets the virtualizer size a block without knowing which calls grouped.
func toolRows(calls []domain.ToolCall) int {
	return 2 * len(planToolRuns(calls))
}

// renderToolGroup draws a run of read-only calls as a call row plus a summary
// row. The summary lists what was touched, which is the one thing the aggregate
// row can lose: opening it restores every individual card.
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

	head := lipgloss.NewStyle().Foreground(t.Gutter).Render(t.Glyphs.Assistant) + " " + c.groupHead(run, t, hover, value, running)
	out := []renderLine{{Text: head, Action: ActionTool, Value: value}}
	out = append(out, c.groupSummary(run, acts, t, width, running, failed, open, frame))
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

// groupHead is the call row: a gerund while the work is in flight and a dim
// past-tense summary once it settles.
func (c *Conversation) groupHead(run toolRun, t Theme, hover, value string, running bool) string {
	text := groupPhrase(run.calls, running)
	if running {
		text += "…"
	}
	style := lipgloss.NewStyle().Foreground(t.Muted)
	if running {
		style = lipgloss.NewStyle().Foreground(t.Text)
	}
	if hover == value {
		style = style.Underline(true)
	}
	return style.Render(text)
}

// groupSummary is the row under a run: the state marker and what was touched.
// The paths and patterns are the information the aggregate would otherwise hide.
func (c *Conversation) groupSummary(run toolRun, acts map[string]domain.ToolActivity, t Theme, width int, running, failed, expanded bool, frame int) renderLine {
	g := t.Glyphs
	state := domain.ToolActivity{State: "success"}
	switch {
	case running:
		state.State = "running"
	case failed:
		state.State = "failed"
	}

	detail := groupDetail(run.calls)
	switch {
	case running:
		detail = "running"
	case failed:
		detail = ""
		for _, tc := range run.calls {
			if a := acts[tc.ID]; a.State == "failed" && strings.TrimSpace(a.Error) != "" {
				detail = firstLine(a.Error)
				break
			}
		}
		if detail == "" {
			detail = "failed"
		}
	}
	var parts []string
	if detail != "" {
		parts = append(parts, detail)
	}
	if !expanded {
		parts = append(parts, "ctrl+o")
	}

	glyph, glyphStyle := toolStateGlyph(state, t, frame)
	style := lipgloss.NewStyle().Foreground(t.Muted)
	if failed {
		style = lipgloss.NewStyle().Foreground(t.Error)
	} else if running {
		style = lipgloss.NewStyle().Foreground(t.Text)
	}
	gutter := resultGutter(g)
	body := strings.Join(parts, " · ")
	text := gutter + glyphStyle.Render(glyph)
	if body != "" {
		text += " " + style.Render(truncWidth(body, max(6, width-lipgloss.Width(gutter)-2)))
	}
	return renderLine{Text: text}
}

// groupDetail lists what the run touched, in call order. It is the summary line's
// payload: "read 3 files" says how many, this says which.
func groupDetail(calls []domain.ToolCall) string {
	parts := make([]string, 0, len(calls))
	for _, tc := range calls {
		if s := toolSummary(tc.Function.Name, tc.Function.Arguments); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
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
