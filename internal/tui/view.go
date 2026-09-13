package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/agent"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) View() tea.View {
	m.cfg = m.ctrl.Config.Snapshot()
	m.regions = m.regions[:0]
	t := themeFor(m.cfg)
	if m.width <= 0 {
		m.width = 100
	}
	if m.height <= 0 {
		m.height = 30
	}
	m.resize()

	var content string
	if m.overlay == overlayNone {
		content = m.renderBase(t)
	} else {
		content = m.renderOverlay(t)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.ReportFocus = true
	if m.cfg.Mouse {
		// Keep mouse routing synchronous in Model.Update. Bubble Tea v2 still
		// delivers the original MouseMsg even when View.OnMouse is set; using
		// both paths would process one physical click twice. CellMotion provides
		// click/release/wheel with broader terminal support.
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// renderBase lays out the transcript-first screen: conversation fills the
// terminal, the composer is the only bordered element, and a single dim status
// row anchors the bottom. There is no header — model, directory and context
// live in the status row so vertical space goes to the conversation.
func (m *Model) renderBase(t Theme) string {
	width, height := m.width, m.height
	sidebarW := 0
	if width >= 96 && m.cfg.Sidebar != "off" {
		sidebarW = min(32, max(24, width/4))
	}
	chatX := 0
	chatW := width
	if sidebarW > 0 {
		chatX = sidebarW + 1
		chatW = width - chatX
	}
	if chatW < 24 {
		sidebarW = 0
		chatX = 0
		chatW = width
	}

	sugg := m.renderSuggestions(t, chatW)
	chips, chipRegs := m.renderFileChips(t, chatW)
	promptH := m.prompt.Height()
	if promptH < 1 {
		promptH = 1
	}
	if promptH > 6 {
		promptH = 6
	}
	statusH := 1
	promptFrameH := promptH + 2 // one rule above, one below
	spacerH := 1                // breathing room between transcript and composer
	chipH := len(chips)
	chatH := height - statusH - promptFrameH - spacerH - len(sugg) - chipH
	if chatH < 4 {
		chatH = 4
	}
	if total := statusH + promptFrameH + spacerH + len(sugg) + chipH + chatH; total > height {
		chatH = max(1, chatH-(total-height))
	}

	m.layout.chat = Rect{chatX, 0, chatW, chatH}
	m.layout.sidebar = Rect{0, 0, sidebarW, max(0, height-statusH)}
	m.layout.prompt = Rect{chatX, chatH + spacerH + len(sugg) + chipH, chatW, promptFrameH}
	m.layout.status = Rect{0, height - 1, width, 1}
	m.layout.chatX = chatX

	// ==================== 1. Conversation ====================
	var chatLines []string
	convLines := m.conv.Render(chatH, t, RenderOptions{
		StreamContent:   m.streamContent,
		StreamReasoning: m.streamReasoning,
		StreamMessageID: m.streamMessageID,
		Activities:      m.activities,
		Hover:           m.hover,
		Frame:           m.activityFrame,
	})
	for row, line := range convLines {
		chatLines = append(chatLines, fitLine(line.Text, chatW))
		if line.Action != "" {
			m.regions = append(m.regions, Region{Rect: Rect{chatX, row, chatW, 1}, Kind: line.Action, Value: line.Value})
		}
	}
	chatLines = append(chatLines, sugg...)
	// Every row must already be fitted to chatW: clampLines only fills the tail.
	chatLines = append(chatLines, fitLine("", chatW))

	// ==================== 2. Composer ====================
	borderColor := t.Border
	if m.focus.Current() == FocusPrompt {
		borderColor = t.Primary
	}
	ruleStyle := lipgloss.NewStyle().Foreground(borderColor)
	rule := func() string {
		return ruleStyle.Render(strings.Repeat(t.Glyphs.Rule, max(1, chatW)))
	}
	// Chips sit directly on the composer's top rule so "attached" reads as
	// part of the input rather than as transcript content.
	for _, chip := range chips {
		chatLines = append(chatLines, chip)
	}
	chatLines = append(chatLines, rule())

	prefixStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	pview := strings.TrimSuffix(m.prompt.View(), "\n")
	plines := strings.Split(pview, "\n")
	for i := 0; i < promptH; i++ {
		line := ""
		if i < len(plines) {
			line = plines[i]
		}
		prefix := "  "
		if i == 0 {
			prefix = prefixStyle.Render(t.Glyphs.User) + " "
		}
		chatLines = append(chatLines, fitLine(prefix+line, chatW))
	}
	chatLines = append(chatLines, rule())

	m.regions = append(m.regions, Region{Rect: Rect{chatX, m.layout.prompt.Y, chatW, promptFrameH}, Kind: ActionPrompt, Value: "prompt"})
	m.regions = append(m.regions, Region{Rect: m.layout.chat, Kind: ActionChat, Value: "chat"})
	// Chip regions were computed in chip-local coordinates; shift them to the
	// rows they were rendered into.
	chipY := m.layout.prompt.Y - 1
	for _, r := range chipRegs {
		r.Rect.Y += chipY
		r.Rect.X += chatX
		m.regions = append(m.regions, r)
	}

	// ==================== 3. Status row ====================
	statusText, statusRegs := m.statusLine(t, width)
	m.regions = append(m.regions, statusRegs...)

	// ==================== 4. Compose ====================
	if sidebarW == 0 {
		var all []string
		all = append(all, chatLines...)
		all = append(all, fitLine(statusText, width))
		return clampLines(all, width, height)
	}

	sideLines := m.renderSidebar(t, sidebarW, height-statusH)
	bodyH := height - statusH
	var all []string
	for y := 0; y < bodyH; y++ {
		sl, cl := "", ""
		if y < len(sideLines) {
			sl = sideLines[y]
		}
		if y < len(chatLines) {
			cl = chatLines[y]
		}
		all = append(all, fitLine(sl, sidebarW)+lipgloss.NewStyle().Foreground(t.Border).Render(t.Glyphs.VBar)+fitLine(cl, chatW))
	}
	all = append(all, fitLine(statusText, width))
	return clampLines(all, width, height)
}

func (m *Model) renderSidebar(t Theme, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	g := t.Glyphs
	var out []string
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Muted)

	out = append(out, fitLine(" "+sectionStyle.Render("SESSIONS"), w))
	start := m.sidebarOffset
	if start < 0 {
		start = 0
	}
	maxSess := max(1, min(len(m.sessions), h/3))
	end := min(len(m.sessions), start+maxSess)
	for i := start; i < end; i++ {
		s := m.sessions[i]
		isCur := s.ID == m.session.ID
		mark := "  "
		nameStyle := lipgloss.NewStyle().Foreground(t.Text)
		if isCur {
			mark = g.Bullet + " "
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
		}
		rowLeft := mark + terminalutil.SanitizeText(s.Name)
		line := padBetween(nameStyle.Render(rowLeft), lipgloss.NewStyle().Foreground(t.Muted).Render(formatAgo(s.UpdatedAt)), w-1)
		out = append(out, fitLine(line, w))
		m.regions = append(m.regions, Region{Rect: Rect{0, len(out) - 1, w, 1}, Kind: ActionSession, Value: s.ID})
	}

	if len(out) < h-3 {
		done := 0
		for _, x := range m.session.Todos {
			if x.Status == "completed" {
				done++
			}
		}
		out = append(out, "")
		out = append(out, fitLine(" "+sectionStyle.Render(fmt.Sprintf("TODOS %d/%d", done, len(m.session.Todos))), w))
		if len(m.session.Todos) == 0 {
			out = append(out, fitLine(lipgloss.NewStyle().Foreground(t.Muted).Render("  none"), w))
		} else {
			for _, x := range m.session.Todos {
				icon, iStyle := g.TodoOpen, lipgloss.NewStyle().Foreground(t.Muted)
				if x.Status == "completed" {
					icon, iStyle = g.TodoDone, lipgloss.NewStyle().Foreground(t.Success)
				} else if x.Status == "in_progress" {
					icon, iStyle = g.Bullet, lipgloss.NewStyle().Foreground(t.Warning)
				}
				line := "  " + iStyle.Render(icon) + " " + terminalutil.SanitizeText(x.Subject)
				out = append(out, fitLine(line, w))
				m.regions = append(m.regions, Region{Rect: Rect{0, len(out) - 1, w, 1}, Kind: ActionTodo, Value: x.ID})
				if len(out) >= h-2 {
					break
				}
			}
		}
	}

	if len(m.subagents) > 0 && len(out) < h-3 {
		out = append(out, "")
		out = append(out, fitLine(" "+sectionStyle.Render("SUBAGENTS"), w))
		for _, a := range m.sortedSubagents() {
			icon, iStyle := g.Pending, lipgloss.NewStyle().Foreground(t.Muted)
			stateLabel := terminalutil.SanitizeText(a.State)
			if stateLabel == "" {
				stateLabel = "pending"
			}
			switch a.State {
			case "running":
				// Spinner frame makes a live subagent visibly tick without
				// changing row order or count, which is what previously read
				// as flicker when the map order shuffled every frame.
				icon, iStyle = g.spinner(m.activityFrame), lipgloss.NewStyle().Foreground(t.Warning)
			case "done", "completed":
				icon, iStyle = g.Success, lipgloss.NewStyle().Foreground(t.Success)
			case "failed", "cancelled":
				icon, iStyle = g.Failure, lipgloss.NewStyle().Foreground(t.Error)
			}
			prog := m.subagentProgress[a.ID]
			elapsed := formatSubagentElapsed(a, m.activityFrame)
			head := fmt.Sprintf("  %s %s · %s", iStyle.Render(icon), terminalutil.SanitizeText(a.AgentID), stateLabel)
			if elapsed != "" {
				head += lipgloss.NewStyle().Foreground(t.Muted).Render(" · " + elapsed)
			}
			if prog.Tools > 0 {
				head += lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf(" · %d tools", prog.Tools))
			}
			out = append(out, fitLine(head, w))
			if a.SessionID != "" {
				m.regions = append(m.regions, Region{Rect: Rect{0, len(out) - 1, w, 1}, Kind: ActionSession, Value: a.SessionID})
			}
			if len(out) >= h-1 {
				break
			}
			// Second dim line carries the task / last-tool so progress is
			// visible at a glance. It is always rendered (possibly empty) so
			// a running subagent never changes the sidebar height mid-turn.
			detail := m.subagentDetail(a, prog)
			if detail != "" {
				dim := lipgloss.NewStyle().Foreground(t.Muted).Render("    " + detail)
				out = append(out, fitLine(dim, w))
			} else {
				out = append(out, fitLine("", w))
			}
			if len(out) >= h {
				break
			}
		}
	}

	for len(out) < h {
		out = append(out, "")
	}
	if len(out) > h {
		out = out[:h]
	}
	return out
}

// sortedSubagents returns background runs in a stable order. Go map iteration
// is randomized, so rendering the map directly reshuffles the sidebar on every
// 125ms spinner frame — the flicker this fixes. Running runs come first so a
// finishing run never jumps above an active one mid-turn.
func (m *Model) sortedSubagents() []domain.AgentRun {
	if len(m.subagents) == 0 {
		return nil
	}
	out := make([]domain.AgentRun, 0, len(m.subagents))
	for _, a := range m.subagents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := out[i].State == "running", out[j].State == "running"
		if ri != rj {
			return ri
		}
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func formatSubagentElapsed(a domain.AgentRun, _ int) string {
	if a.StartedAt.IsZero() {
		return ""
	}
	end := time.Now()
	if !a.FinishedAt.IsZero() {
		end = a.FinishedAt
	}
	d := end.Sub(a.StartedAt)
	if d < 0 {
		return ""
	}
	return formatDuration(d)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	if m < 60 {
		if r := s % 60; r > 0 {
			return fmt.Sprintf("%dm%ds", m, r)
		}
		return fmt.Sprintf("%dm", m)
	}
	h := m / 60
	if r := m % 60; r > 0 {
		return fmt.Sprintf("%dh%dm", h, r)
	}
	return fmt.Sprintf("%dh", h)
}

// subagentDetail prefers the delegated task, falling back to the session name
// (which embeds the task for @-started runs) and finally the last tool seen.
func (m *Model) subagentDetail(a domain.AgentRun, prog subagentProgress) string {
	if t := strings.TrimSpace(a.Task); t != "" {
		return truncWidth(singleLine(terminalutil.SanitizeText(t)), 60)
	}
	for _, s := range m.sessions {
		if s.ID == a.SessionID && strings.TrimSpace(s.Name) != "" {
			name := terminalutil.SanitizeText(s.Name)
			if i := strings.Index(name, ": "); i >= 0 {
				name = strings.TrimSpace(name[i+2:])
			}
			if name != "" {
				return truncWidth(singleLine(name), 60)
			}
		}
	}
	if prog.LastTool != "" {
		if prog.Tools > 0 {
			return "↳ " + prog.LastTool
		}
		return prog.LastTool
	}
	return ""
}

// renderSuggestions draws the inline command / @-mention completions as a
// compact list directly above the composer, matching the flat transcript style
// instead of boxing them.
func (m *Model) renderSuggestions(t Theme, w int) []string {
	var items []PickerItem
	if len(m.atMatches) > 0 {
		items = m.atMatches
	} else if len(m.commandMatches) > 0 {
		items = m.commandMatches
	}
	if len(items) == 0 {
		return nil
	}
	n := min(6, len(items))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		it := items[i]
		label := terminalutil.SanitizeText(it.Label)
		desc := terminalutil.SanitizeText(it.Description)
		if i == 0 {
			left := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(" "+t.Glyphs.Bullet+" "+label) +
				lipgloss.NewStyle().Foreground(t.Muted).Render("  "+desc)
			right := lipgloss.NewStyle().Foreground(t.Muted).Render("[Tab] ")
			out = append(out, fitLine(padBetween(left, right, w), w))
			continue
		}
		left := lipgloss.NewStyle().Foreground(t.Text).Render("   "+label) +
			lipgloss.NewStyle().Foreground(t.Muted).Render("  "+desc)
		out = append(out, fitLine(left, w))
	}
	return out
}

// fileRefsIn extracts the @path references a turn would attach, mirroring the
// recognition rules of Runtime.expandFileReferences: any @token that is not an
// agent mention or a @skill: reference, deduplicated and capped. The chips row
// and the runtime injection must agree, so this is kept in one place.
func fileRefsIn(text string, isAgent func(string) bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range strings.Fields(text) {
		if !strings.HasPrefix(f, "@") {
			continue
		}
		ref := strings.Trim(strings.TrimPrefix(f, "@"), "`'\".,;:()[]{}")
		if ref == "" || strings.HasPrefix(ref, "skill:") || isAgent(ref) {
			continue
		}
		if seen[ref] || len(out) >= 8 {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}

// removeFileRef drops one @token from the composer text, used when a chip is
// clicked. Field-wise rebuild keeps the surrounding spacing sane.
func removeFileRef(text, ref string) string {
	fields := strings.Fields(text)
	var keep []string
	for _, f := range fields {
		stripped := strings.Trim(strings.TrimPrefix(f, "@"), "`'\".,;:()[]{}")
		if strings.HasPrefix(f, "@") && stripped == ref {
			continue
		}
		keep = append(keep, f)
	}
	return strings.Join(keep, " ")
}

// renderFileChips draws the recognized @path references directly above the
// composer, so the user can see what will actually be attached and remove one
// by clicking its chip.
func (m *Model) renderFileChips(t Theme, w int) ([]string, []Region) {
	refs := fileRefsIn(m.prompt.Value(), func(id string) bool {
		d := agent.Get(id)
		return d.ID == id && d.CanSubagent()
	})
	if len(refs) == 0 {
		return nil, nil
	}
	label := lipgloss.NewStyle().Foreground(t.Muted).Render(t.Glyphs.Result + " ")
	var chips []Region
	line := " " + label
	for _, ref := range refs {
		chip := lipgloss.NewStyle().Foreground(t.Tool).Render("@"+terminalutil.SanitizeText(ref)) +
			lipgloss.NewStyle().Foreground(t.Muted).Render(" ×")
		// Record where the clickable part (including the removal mark) starts.
		chips = append(chips, Region{Rect: Rect{lipgloss.Width(line), 0, lipgloss.Width(chip), 1}, Kind: ActionFileChip, Value: ref})
		line += chip + "  "
	}
	return []string{fitLine(line, w)}, chips
}

func (m *Model) renderOverlay(t Theme) string {
	w := min(max(30, m.width-8), 100)
	h := min(max(8, m.height-6), 28)
	if m.width < 40 {
		w = max(20, m.width-2)
	}
	if m.height < 12 {
		h = max(5, m.height-2)
	}
	if m.overlay == overlayApproval && m.isPlanApproval() {
		w = min(max(40, m.width-6), m.width)
		h = min(max(16, m.height-4), m.height)
	}
	m.layout.overlay = Rect{max(0, (m.width-w)/2), max(0, (m.height-h)/2), w, h}
	var lines []string
	var regs []Region
	switch m.overlay {
	case overlayPicker:
		lines, regs = m.picker.Render(max(20, w-4), max(5, h-4), t, m.hover)
	case overlayDiff:
		lines, regs = m.diff.Render(max(20, w-4), max(4, h-4), t)
	case overlayTodos:
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Todos"), "")
		if len(m.session.Todos) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("  No active todos for this session."))
		} else {
			for _, x := range m.session.Todos {
				icon, iStyle := t.Glyphs.TodoOpen, lipgloss.NewStyle().Foreground(t.Muted)
				if x.Status == "completed" {
					icon, iStyle = t.Glyphs.TodoDone, lipgloss.NewStyle().Foreground(t.Success)
				} else if x.Status == "in_progress" {
					icon, iStyle = t.Glyphs.Bullet, lipgloss.NewStyle().Foreground(t.Warning)
				}
				row := fmt.Sprintf("  %s %s  [%s]", iStyle.Render(icon), terminalutil.SanitizeText(x.Subject), terminalutil.SanitizeText(string(x.Status)))
				lines = append(lines, row)
				regs = append(regs, Region{Rect: Rect{0, len(lines) - 1, max(1, w-4), 1}, Kind: ActionTodo, Value: x.ID})
			}
		}
	case overlayPlan:
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Plan"), "")
		steps := domain.PlanFromMetadata(m.session.Metadata)
		if len(steps) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("  No plan recorded for this session."))
		} else {
			for _, x := range steps {
				icon, iStyle := t.Glyphs.TodoOpen, lipgloss.NewStyle().Foreground(t.Muted)
				if x.Status == "completed" {
					icon, iStyle = t.Glyphs.TodoDone, lipgloss.NewStyle().Foreground(t.Success)
				} else if x.Status == "in_progress" {
					icon, iStyle = t.Glyphs.Bullet, lipgloss.NewStyle().Foreground(t.Warning)
				}
				row := fmt.Sprintf("  %s %s  [%s]", iStyle.Render(icon), terminalutil.SanitizeText(x.Subject), terminalutil.SanitizeText(string(x.Status)))
				lines = append(lines, row)
				if x.Details != "" {
					lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("      "+terminalutil.SanitizeText(x.Details)))
				}
			}
		}
	case overlayHistory:
		title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("History search")
		query := lipgloss.NewStyle().Foreground(t.Primary).Render(t.Glyphs.User+" ") +
			lipgloss.NewStyle().Foreground(t.Text).Bold(true).Render(terminalutil.SanitizeText(m.historyQuery)) +
			lipgloss.NewStyle().Foreground(t.Muted).Render(t.Glyphs.Cursor)
		lines = append(lines, title, query, "")
		if len(m.historyMatches) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("  no matches"))
		} else {
			visible := max(1, min(8, h-6))
			for i := 0; i < visible && i < len(m.historyMatches); i++ {
				entry := terminalutil.SanitizeText(singleLine(m.historyMatches[i]))
				row := lipgloss.NewStyle().Foreground(t.Text).Render("   " + truncWidth(entry, max(10, w-6)))
				if i == m.historySel {
					row = lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(" " + t.Glyphs.Bullet + " " + truncWidth(entry, max(10, w-6)))
				}
				lines = append(lines, row)
			}
		}
		lines = append(lines, "", lipgloss.NewStyle().Foreground(t.Muted).Render("↑/↓ to move · enter to insert · esc to close"))
	case overlayConnect:
		labels := []string{"Provider name", "Base URL (include /v1)", "API Key (optional)"}
		step := m.connect.Step
		if step < 0 || step >= len(labels) {
			step = 0
		}
		title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("Connect Provider")
		stepInfo := lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("Step %d of %d", step+1, len(labels)))
		lines = []string{
			title + "  " + stepInfo,
			"",
			lipgloss.NewStyle().Bold(true).Foreground(t.Text).Render(labels[step]),
			lipgloss.NewStyle().Foreground(t.Primary).Render(maskConnectInput(m.connect.Input, step)),
			"",
			lipgloss.NewStyle().Foreground(t.Muted).Render("enter Next · esc Cancel"),
		}
	case overlayConfirmRestore:
		lines = []string{
			lipgloss.NewStyle().Bold(true).Foreground(t.Error).Render("Restore Factory Settings"),
			"",
			"This will delete all sessions and configuration data.",
			"",
			"Press y/Enter to confirm · n/Esc to cancel",
		}
	case overlayConfirmDelete:
		lines = []string{
			lipgloss.NewStyle().Bold(true).Foreground(t.Error).Render("Delete Session"),
			"",
			terminalutil.SanitizeText(m.infoText),
			"",
			"Press y/Enter to confirm · n/Esc to cancel",
		}
	case overlayApproval:
		lines, regs = m.renderApproval(t, w)
	default:
		title := m.infoTitle
		if title == "" {
			title = "Info"
		}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(title), "")
		lines = append(lines, wrapPlain(m.infoText, max(10, w-4))...)
	}
	innerH := max(3, h-2)
	scrollable := m.overlay == overlayHelp || m.overlay == overlayTodos || m.overlay == overlayPlan || m.overlay == overlayContext || m.overlay == overlayInfo || (m.overlay == overlayApproval && m.isPlanApproval())
	if scrollable && len(lines) > innerH {
		maxOff := max(0, len(lines)-innerH)
		if m.overlayScroll > maxOff {
			m.overlayScroll = maxOff
		}
		start := max(0, m.overlayScroll)
		end := min(len(lines), start+innerH)
		lines = lines[start:end]
		if len(regs) > 0 {
			filtered := regs[:0]
			for _, r := range regs {
				r.Rect.Y -= start
				if r.Rect.Y >= 0 && r.Rect.Y < innerH {
					filtered = append(filtered, r)
				}
			}
			regs = filtered
		}
	} else if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).Padding(0, 1).Width(w - 2).Height(h - 2)
	body := boxStyle.Render(strings.Join(lines, "\n"))
	top := m.layout.overlay.Y
	left := m.layout.overlay.X
	out := make([]string, 0, m.height)
	for i := 0; i < top; i++ {
		out = append(out, strings.Repeat(" ", m.width))
	}
	for _, l := range strings.Split(body, "\n") {
		out = append(out, strings.Repeat(" ", left)+l)
	}
	for len(out) < m.height {
		out = append(out, "")
	}
	if len(out) > m.height {
		out = out[:m.height]
	}
	// convert picker/diff-local regions to terminal coordinates. +2 due border/padding.
	for _, r := range regs {
		r.Rect.X += left + 2
		r.Rect.Y += top + 1
		m.regions = append(m.regions, r)
	}
	return strings.Join(out, "\n")
}

// renderApproval draws the approval prompt: tool identity, a one-line argument
// preview, the numbered choices, and the inline esc hint. The overlay frame is
// drawn by renderOverlay; only the inner content lives here.
func (m *Model) renderApproval(t Theme, w int) ([]string, []Region) {
	a := m.approval
	if a == nil {
		return []string{"No pending approval."}, nil
	}
	if m.isPlanApproval() {
		return m.renderPlanApproval(t, w), nil
	}
	inner := max(10, w-4)
	title := lipgloss.NewStyle().Bold(true).Foreground(t.Warning).Render("Approval required")
	tool := lipgloss.NewStyle().Bold(true).Foreground(t.Tool).Render("⚡ " + terminalutil.SanitizeText(a.ToolName))
	lines := []string{title, "", tool}
	if s := terminalutil.SanitizeText(a.Summary); s != "" {
		lines = append(lines, truncWidth(s, inner))
	} else if len(a.Arguments) > 0 {
		lines = append(lines, truncWidth(terminalutil.SanitizeText(string(a.Arguments)), inner))
	}
	if a.SessionID != m.session.ID && a.SessionID != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(t.Muted).Render("from a subagent session"))
	}
	lines = append(lines, "")
	options := []string{"Allow once", "Always allow " + a.ToolName + " this session", "Deny, and tell the model what to do"}
	if m.approvalFeedback {
		lines = append(lines,
			lipgloss.NewStyle().Bold(true).Foreground(t.Text).Render("Reason for the model:"),
			lipgloss.NewStyle().Foreground(t.Primary).Render("> "+terminalutil.SanitizeText(m.approvalInput))+t.Glyphs.Cursor,
			"",
			lipgloss.NewStyle().Foreground(t.Muted).Render("enter to deny with this reason · esc to go back"))
		return lines, nil
	}
	for i, o := range options {
		marker, style := "  ", lipgloss.NewStyle().Foreground(t.Text)
		if i == m.approvalChoice {
			marker, style = t.Glyphs.Bullet+" ", lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
		}
		lines = append(lines, style.Render(fmt.Sprintf("%s%d. %s", marker, i+1, o)))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(t.Muted).Render("1/2/3 to choose · esc to deny"))
	return lines, nil
}

// renderPlanApproval is the Claude Code-style gate out of plan mode: the
// recorded plan, then auto-accept / manual / keep-planning.
func (m *Model) renderPlanApproval(t Theme, w int) []string {
	inner := max(10, w-4)
	title := lipgloss.NewStyle().Bold(true).Foreground(t.Warning).Render("Ready to implement?")
	hint := lipgloss.NewStyle().Foreground(t.Muted).Render("Review the plan, then choose how edits should run.")
	lines := []string{title, hint, ""}
	if a := m.approval; a != nil && a.Summary != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render(truncWidth(terminalutil.SanitizeText(a.Summary), inner)), "")
	}
	steps := domain.PlanFromMetadata(m.session.Metadata)
	if len(steps) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("No structured plan recorded — the transcript above is the plan."), "")
	} else {
		const maxSteps = 8
		shown := steps
		extra := 0
		if len(shown) > maxSteps {
			extra = len(shown) - maxSteps
			shown = shown[:maxSteps]
		}
		for i, x := range shown {
			row := fmt.Sprintf("  %d. %s", i+1, terminalutil.SanitizeText(x.Subject))
			lines = append(lines, lipgloss.NewStyle().Foreground(t.Text).Render(truncWidth(row, inner)))
			if x.Details != "" {
				for _, d := range wrapPlain("     "+terminalutil.SanitizeText(x.Details), inner) {
					lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render(d))
				}
			}
		}
		if extra > 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("  …and %d more (/plan to review)", extra)))
		}
		lines = append(lines, "")
	}
	if m.approvalFeedback {
		return append(lines,
			lipgloss.NewStyle().Bold(true).Foreground(t.Text).Render("What should change in the plan?"),
			lipgloss.NewStyle().Foreground(t.Primary).Render("> "+terminalutil.SanitizeText(m.approvalInput))+t.Glyphs.Cursor,
			"",
			lipgloss.NewStyle().Foreground(t.Muted).Render("enter to keep planning with this note · esc to go back"),
		)
	}
	options := []string{
		"Auto-accept edits",
		"Manually approve edits",
		"Keep planning, and tell the model what to change",
	}
	for i, o := range options {
		marker, style := "  ", lipgloss.NewStyle().Foreground(t.Text)
		if i == m.approvalChoice {
			marker, style = t.Glyphs.Bullet+" ", lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
		}
		lines = append(lines, style.Render(fmt.Sprintf("%s%d. %s", marker, i+1, o)))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(t.Muted).Render("1 auto-accept · 2 approve each edit · 3 keep planning · esc to stay in plan"))
	return lines
}

func maskConnectInput(s string, step int) string {
	if step != 3 {
		return "> " + s
	}
	if s == "" {
		return "> "
	}
	return "> " + strings.Repeat("*", min(48, len([]rune(s))))
}

// busyIndicator renders the right-hand side of the status row: elapsed time and
// an explicit interrupt hint while a turn runs, a transient toast, or the
// shortcuts hint when idle.
func (m *Model) busyIndicator(t Theme) string {
	if m.busy {
		spin := t.Glyphs.spinner(m.activityFrame)
		elapsed := ""
		if !m.busySince.IsZero() {
			if d := time.Since(m.busySince); d >= time.Second {
				elapsed = " " + d.Round(time.Second).String()
			}
		}
		return lipgloss.NewStyle().Foreground(t.Warning).Render(fmt.Sprintf("%s Working%s · esc to interrupt", spin, elapsed))
	}
	if m.toast != "" {
		return lipgloss.NewStyle().Foreground(t.Accent).Render(terminalutil.SanitizeText(m.toast))
	}
	if m.errorText != "" {
		return lipgloss.NewStyle().Foreground(t.Error).Render("× " + terminalutil.SanitizeText(m.errorText))
	}
	if m.statusCmdText != "" {
		return lipgloss.NewStyle().Foreground(t.Muted).Render(m.statusCmdText)
	}
	// Idle shows nothing. A permanent "? for shortcuts" on every frame is
	// clutter; the composer placeholder carries that hint instead.
	return ""
}

func fitLine(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := lipgloss.Width(s)
	if sw > w {
		return truncANSI(s, w)
	}
	if sw < w {
		return s + strings.Repeat(" ", w-sw)
	}
	return s
}

// truncANSI clips to a display width. Counting runes instead would silently
// fail to shrink a line of double-width CJK, which is what makes long localized
// lines overrun the terminal.
func truncANSI(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// clampLines trims or pads to exactly h rows. Input lines are already fitted to
// w by their builders, so this only has to fill the tail — re-measuring every
// line here would double the per-frame width scanning for no benefit.
func clampLines(lines []string, w, h int) string {
	if len(lines) > h {
		lines = lines[:h]
	}
	if pad := h - len(lines); pad > 0 {
		blank := strings.Repeat(" ", max(0, w))
		for i := 0; i < pad; i++ {
			lines = append(lines, blank)
		}
	}
	return strings.Join(lines, "\n")
}
