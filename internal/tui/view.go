package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	ctxutil "github.com/ViudiraTech/Uinxed-Agent/internal/context"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

func (m *Model) View() tea.View {
	m.cfg = m.ctrl.Config.Snapshot()
	m.regions = m.regions[:0]
	t := theme(m.cfg.Theme)
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

func (m *Model) renderBase(t Theme) string {
	width, height := m.width, m.height
	sidebarW := 0
	if width >= 120 && m.cfg.Sidebar != "off" {
		sidebarW = 28
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
	promptH := m.prompt.Height()
	if promptH < 1 {
		promptH = 1
	}
	if promptH > 6 {
		promptH = 6
	}
	headerH := 3
	statusH := 1
	promptFrameH := promptH + 2
	chatH := height - headerH - statusH - promptFrameH - len(sugg)
	if chatH < 4 {
		chatH = 4
	}
	maxTotal := headerH + statusH + promptFrameH + len(sugg) + chatH
	if maxTotal > height {
		chatH = max(1, chatH-(maxTotal-height))
	}

	m.layout.chat = Rect{chatX, headerH, chatW, chatH}
	m.layout.sidebar = Rect{0, headerH, sidebarW, max(0, height-headerH-statusH)}
	m.layout.prompt = Rect{chatX, headerH + chatH + len(sugg), chatW, promptFrameH}
	m.layout.status = Rect{0, height - 1, width, 1}
	m.layout.chatX = chatX

	var main []string
	// Header: deliberately text-first, minimal borders.
	safeAgent := terminalutil.SanitizeText(m.session.AgentID)
	safeModel := terminalutil.SanitizeText(m.session.Model)
	safeProvider := terminalutil.SanitizeText(m.session.ProviderID)
	agentText := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(safeAgent)
	modelText := lipgloss.NewStyle().Foreground(t.Secondary).Render(safeModel)
	left := " Uinxed Agent"
	right := agentText + " · " + modelText
	main = append(main, fitLine(padBetween(left, right, chatW), chatW))
	cwd := terminalutil.SanitizeText(m.session.CWD)
	if cwd == "" {
		cwd = "."
	}
	ctxUsed := ctxutil.EstimateMessages(m.session.Messages)
	ctxWin := ctxutil.Window(m.session.Model)
	pct := 0
	if ctxWin > 0 {
		pct = ctxUsed * 100 / ctxWin
	}
	main = append(main, fitLine(padBetween(" "+cwd, fmt.Sprintf("%s · ctx %d%%", safeProvider, pct), chatW), chatW))
	main = append(main, lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", max(1, chatW))))

	convLines := m.conv.Render(chatH, t, m.streamContent, m.streamReasoning, m.activities, m.hover)
	for row, line := range convLines {
		main = append(main, fitLine(line.Text, chatW))
		if line.Action != "" {
			m.regions = append(m.regions, Region{Rect: Rect{chatX, headerH + row, chatW, 1}, Kind: line.Action, Value: line.Value})
		}
	}
	main = append(main, sugg...)

	promptY := headerH + chatH + len(sugg)
	border := lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", max(1, chatW)))
	main = append(main, border)
	pview := strings.TrimSuffix(m.prompt.View(), "\n")
	plines := strings.Split(pview, "\n")
	for i := 0; i < promptH; i++ {
		line := ""
		if i < len(plines) {
			line = plines[i]
		}
		prefix := "  "
		if i == 0 {
			prefix = lipgloss.NewStyle().Foreground(t.Primary).Render("› ")
		}
		main = append(main, fitLine(prefix+line, chatW))
	}
	main = append(main, border)
	m.regions = append(m.regions, Region{Rect: Rect{chatX, promptY, chatW, promptFrameH}, Kind: ActionPrompt, Value: "prompt"})

	status := m.statusLine(t, chatW)
	main = append(main, fitLine(status, chatW))

	// clickable header regions
	rightStart := max(0, chatW-visibleLen(stripANSI(right))-1)
	m.regions = append(m.regions,
		Region{Rect: Rect{chatX + rightStart, 0, max(5, len(safeAgent)), 1}, Kind: ActionAgent, Value: m.session.AgentID},
		Region{Rect: Rect{chatX + rightStart + max(5, len(safeAgent)) + 3, 0, max(6, len(safeModel)), 1}, Kind: ActionModel, Value: m.session.Model},
		Region{Rect: Rect{chatX + max(0, chatW-len(safeProvider)-10), 1, max(8, len(safeProvider)), 1}, Kind: ActionProvider, Value: m.session.ProviderID},
		Region{Rect: m.layout.chat, Kind: ActionChat, Value: "chat"},
	)

	if sidebarW == 0 {
		return clampLines(main, width, height)
	}
	side := m.renderSidebar(t, sidebarW, height-headerH-statusH)
	// Build header first then horizontally compose sidebar with main body after 3 rows.
	out := make([]string, 0, height)
	for y := 0; y < height; y++ {
		ml := ""
		if y < len(main) {
			ml = main[y]
		}
		if y < headerH || y == height-1 {
			out = append(out, fitLine(ml, width))
			continue
		}
		si := y - headerH
		sl := ""
		if si < len(side) {
			sl = side[si]
		}
		out = append(out, fitLine(sl, sidebarW)+lipgloss.NewStyle().Foreground(t.Border).Render("│")+fitLine(ml, chatW))
	}
	return strings.Join(out, "\n")
}

func (m *Model) renderSidebar(t Theme, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	var out []string
	out = append(out, lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(" Sessions"))
	start := m.sidebarOffset
	if start < 0 {
		start = 0
	}
	end := min(len(m.sessions), start+max(1, h/2-1))
	for i := start; i < end; i++ {
		s := m.sessions[i]
		mark := "  "
		if s.ID == m.session.ID {
			mark = "› "
		}
		line := fmt.Sprintf("%s%s  %s", mark, terminalutil.SanitizeText(s.Name), formatAgo(s.UpdatedAt))
		out = append(out, fitLine(line, w))
		m.regions = append(m.regions, Region{Rect: Rect{0, 3 + len(out) - 1, w, 1}, Kind: ActionSession, Value: s.ID})
	}
	out = append(out, "")
	out = append(out, lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(" Todos"))
	done := 0
	for _, x := range m.session.Todos {
		if x.Status == "completed" {
			done++
		}
	}
	if len(m.session.Todos) == 0 {
		out = append(out, lipgloss.NewStyle().Foreground(t.Muted).Render("  no todos"))
	} else {
		out = append(out, lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("  %d/%d complete", done, len(m.session.Todos))))
		for _, x := range m.session.Todos {
			icon := "○"
			if x.Status == "completed" {
				icon = "✓"
			} else if x.Status == "in_progress" {
				icon = "◉"
			}
			out = append(out, fitLine("  "+icon+" "+terminalutil.SanitizeText(x.Subject), w))
			m.regions = append(m.regions, Region{Rect: Rect{0, 3 + len(out) - 1, w, 1}, Kind: ActionTodo, Value: x.ID})
			if len(out) >= h {
				break
			}
		}
	}
	if len(m.subagents) > 0 && len(out) < h-2 {
		out = append(out, "")
		out = append(out, lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(" Subagents"))
		for _, a := range m.subagents {
			icon := "○"
			if a.State == "running" {
				icon = "◉"
			} else if a.State == "done" || a.State == "completed" {
				icon = "●"
			}
			out = append(out, fitLine(fmt.Sprintf("  %s %s · %s", icon, terminalutil.SanitizeText(a.AgentID), terminalutil.SanitizeText(a.State)), w))
			if a.SessionID != "" {
				m.regions = append(m.regions, Region{Rect: Rect{0, 3 + len(out) - 1, w, 1}, Kind: ActionSession, Value: a.SessionID})
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
	n := min(5, len(items))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		it := items[i]
		prefix := "  "
		if i == 0 {
			prefix = "› "
		}
		line := prefix + terminalutil.SanitizeText(it.Label)
		if it.Description != "" {
			line += "  " + terminalutil.SanitizeText(it.Description)
		}
		out = append(out, fitLine(lipgloss.NewStyle().Foreground(t.Muted).Render(line), w))
	}
	return out
}

func (m *Model) statusLine(t Theme, w int) string {
	left := fmt.Sprintf(" %s │ %s │ %s", terminalutil.SanitizeText(m.session.AgentID), terminalutil.SanitizeText(m.session.Model), terminalutil.SanitizeText(m.session.ProviderID))
	if m.busy {
		activity := "◉"
		if m.cfg.Animations {
			frames := []string{"◐", "◓", "◑", "◒"}
			activity = frames[m.activityFrame%len(frames)]
		}
		left = lipgloss.NewStyle().Foreground(t.Warning).Render(" "+activity+" running") + " │ " + left
	}
	right := fmt.Sprintf("%s │ %s", m.currentEffort(), m.ctrl.Config.Snapshot().Storage)
	if m.toast != "" {
		right = m.toast
	}
	if len([]rune(right)) > w/2 {
		right = truncWidth(right, max(8, w/2))
	}
	return padBetween(left, right, w)
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
	m.layout.overlay = Rect{max(0, (m.width-w)/2), max(0, (m.height-h)/2), w, h}
	var lines []string
	var regs []Region
	switch m.overlay {
	case overlayPicker:
		lines, regs = m.picker.Render(max(20, w-4), max(5, h-4), t, m.hover)
	case overlayDiff:
		lines, regs = m.diff.Render(max(20, w-4), max(4, h-4), t)
	case overlayTodos:
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("Todos"))
		if len(m.session.Todos) == 0 {
			lines = append(lines, "No todos.")
		} else {
			for _, x := range m.session.Todos {
				icon := "○"
				if x.Status == "completed" {
					icon = "✓"
				} else if x.Status == "in_progress" {
					icon = "◉"
				}
				lines = append(lines, fmt.Sprintf("%s %s  [%s]", icon, terminalutil.SanitizeText(x.Subject), terminalutil.SanitizeText(string(x.Status))))
				regs = append(regs, Region{Rect: Rect{0, len(lines) - 1, max(1, w-4), 1}, Kind: ActionTodo, Value: x.ID})
			}
		}
	case overlayConnect:
		labels := []string{"Provider name", "Base URL (include /v1)", "Models (comma separated)", "API Key (optional)"}
		step := m.connect.Step
		if step < 0 || step >= len(labels) {
			step = 0
		}
		lines = []string{lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("Connect Provider"), "", labels[step], maskConnectInput(m.connect.Input, step), "", "Enter next · Esc cancel"}
	case overlayConfirmRestore:
		lines = []string{lipgloss.NewStyle().Bold(true).Foreground(t.Error).Render("Restore factory settings"), "", "This deletes all sessions and config data.", "Press y/Enter to confirm · n/Esc cancel"}
	case overlayConfirmDelete:
		lines = []string{lipgloss.NewStyle().Bold(true).Foreground(t.Error).Render("Delete Session"), "", terminalutil.SanitizeText(m.infoText), "Press y/Enter to confirm · n/Esc cancel"}
	default:
		title := m.infoTitle
		if title == "" {
			title = "Info"
		}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(title), "")
		lines = append(lines, wrapPlain(m.infoText, max(10, w-4))...)
	}
	innerH := max(3, h-2)
	scrollable := m.overlay == overlayHelp || m.overlay == overlayTodos || m.overlay == overlayContext || m.overlay == overlayInfo
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

func maskConnectInput(s string, step int) string {
	if step != 3 {
		return "> " + s
	}
	if s == "" {
		return "> "
	}
	return "> " + strings.Repeat("•", min(48, len([]rune(s))))
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
func truncANSI(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	plain := stripANSI(s)
	r := []rune(plain)
	if len(r) > w {
		if w > 1 {
			r = r[:w-1]
			return string(r) + "…"
		}
		return string(r[:w])
	}
	return string(r)
}
func clampLines(lines []string, w, h int) string {
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = fitLine(lines[i], w)
	}
	return strings.Join(lines, "\n")
}
