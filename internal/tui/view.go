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

	// ==================== 1. Top Header ====================
	safeAgent := terminalutil.SanitizeText(m.session.AgentID)
	safeModel := terminalutil.SanitizeText(m.session.Model)
	safeProvider := terminalutil.SanitizeText(m.session.ProviderID)

	// Row 0: Brand and Interactive Pill Badges
	logoPill := lipgloss.NewStyle().Bold(true).Background(t.Primary).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Render("◆ UINXED AGENT")
	verText := lipgloss.NewStyle().Foreground(t.Muted).Render("v2.0")
	leftBrand := " " + logoPill + " " + verText

	agentPill := lipgloss.NewStyle().Background(t.PillBg).Foreground(t.Primary).Bold(true).Padding(0, 1).Render("🤖 " + safeAgent + " ▾")
	modelPill := lipgloss.NewStyle().Background(t.PillBg).Foreground(t.Secondary).Padding(0, 1).Render("󰘧 " + safeModel + " ▾")
	providerPill := lipgloss.NewStyle().Background(t.PillBg).Foreground(t.Accent).Padding(0, 1).Render("󰢏 " + safeProvider + " ▾")
	rightPills := agentPill + " " + modelPill + " " + providerPill + " "

	row0 := fitLine(padBetween(leftBrand, rightPills, width), width)

	// Register header click hitboxes on Row 0
	pillsW := lipgloss.Width(rightPills)
	rightStart := max(0, width-pillsW)
	aW := lipgloss.Width(agentPill)
	mW := lipgloss.Width(modelPill)
	pW := lipgloss.Width(providerPill)
	m.regions = append(m.regions,
		Region{Rect: Rect{rightStart, 0, aW, 1}, Kind: ActionAgent, Value: m.session.AgentID},
		Region{Rect: Rect{rightStart + aW + 1, 0, mW, 1}, Kind: ActionModel, Value: m.session.Model},
		Region{Rect: Rect{rightStart + aW + 1 + mW + 1, 0, pW, 1}, Kind: ActionProvider, Value: m.session.ProviderID},
	)

	// Row 1: Working Directory and Context Usage Progress Bar
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
	ctxColor := t.Success
	if pct > 80 {
		ctxColor = t.Error
	} else if pct > 60 {
		ctxColor = t.Warning
	}
	barW := 6
	filled := min(barW, pct*barW/100)
	bar := strings.Repeat("■", filled) + strings.Repeat("·", max(0, barW-filled))
	ctxMeter := lipgloss.NewStyle().Foreground(ctxColor).Render(fmt.Sprintf("󰓅 Context: %d%% [%s] ", pct, bar))
	dirText := lipgloss.NewStyle().Foreground(t.Muted).Render("  📁 " + truncWidth(cwd, max(10, width-lipgloss.Width(ctxMeter)-6)))
	row1 := fitLine(padBetween(dirText, ctxMeter, width), width)

	// Row 2: Top Separator Line
	var row2 string
	if sidebarW > 0 {
		row2 = lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", sidebarW) + "┬" + strings.Repeat("─", chatW))
	} else {
		row2 = lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", width))
	}
	headerLines := []string{row0, row1, row2}

	// ==================== 2. Chat Conversation Body ====================
	var chatLines []string
	convLines := m.conv.Render(chatH, t, m.streamContent, m.streamReasoning, m.activities, m.hover)
	for row, line := range convLines {
		chatLines = append(chatLines, fitLine(line.Text, chatW))
		if line.Action != "" {
			m.regions = append(m.regions, Region{Rect: Rect{chatX, headerH + row, chatW, 1}, Kind: line.Action, Value: line.Value})
		}
	}
	chatLines = append(chatLines, sugg...)

	// ==================== 3. Input Prompt Card ====================
	promptY := headerH + chatH + len(sugg)
	borderStyle := lipgloss.NewStyle().Foreground(t.Border)
	if m.focus.Current() == FocusPrompt {
		borderStyle = lipgloss.NewStyle().Foreground(t.Primary)
	}
	promptTitle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true).Render("󰋽 Prompt")
	topPromptBorder := borderStyle.Render("╭─ ") + promptTitle + " " + borderStyle.Render(strings.Repeat("─", max(1, chatW-lipgloss.Width(promptTitle)-5))+"╮")
	chatLines = append(chatLines, fitLine(topPromptBorder, chatW))

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
		contentLine := prefix + line
		gap := max(0, chatW-2-lipgloss.Width(contentLine))
		inside := borderStyle.Render("│ ") + contentLine + strings.Repeat(" ", gap) + borderStyle.Render("│")
		chatLines = append(chatLines, fitLine(inside, chatW))
	}

	hints := "[↵ Send] [⇧↵ Line] [^P Menu] [^B Sidebar] [^T Think]"
	if chatW < 60 {
		hints = "[↵ Send] [^P Menu] [^B Side]"
	}
	hintPills := lipgloss.NewStyle().Foreground(t.Muted).Render(hints)
	botDash := max(1, chatW-lipgloss.Width(hintPills)-5)
	botPromptBorder := borderStyle.Render("╰─ ") + hintPills + " " + borderStyle.Render(strings.Repeat("─", botDash)+"╯")
	chatLines = append(chatLines, fitLine(botPromptBorder, chatW))

	m.regions = append(m.regions, Region{Rect: Rect{chatX, promptY, chatW, promptFrameH}, Kind: ActionPrompt, Value: "prompt"})
	m.regions = append(m.regions, Region{Rect: m.layout.chat, Kind: ActionChat, Value: "chat"})

	// ==================== 4. Layout Composition ====================
	if sidebarW == 0 {
		var all []string
		all = append(all, headerLines...)
		all = append(all, chatLines...)
		all = append(all, fitLine(m.statusLine(t, width), width))
		return clampLines(all, width, height)
	}

	sideLines := m.renderSidebar(t, sidebarW, height-headerH-statusH)
	bodyH := height - headerH - statusH
	var all []string
	all = append(all, headerLines...)
	for y := 0; y < bodyH; y++ {
		sl := ""
		if y < len(sideLines) {
			sl = sideLines[y]
		}
		cl := ""
		if y < len(chatLines) {
			cl = chatLines[y]
		}
		bodyRow := fitLine(sl, sidebarW) + lipgloss.NewStyle().Foreground(t.Border).Render("│") + fitLine(cl, chatW)
		all = append(all, bodyRow)
	}
	all = append(all, fitLine(m.statusLine(t, width), width))
	return clampLines(all, width, height)
}

func (m *Model) renderSidebar(t Theme, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	var out []string
	borderStyle := lipgloss.NewStyle().Foreground(t.Border)

	// Section 1: Sessions
	sessHeader := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(" 󰋜 SESSIONS")
	out = append(out, fitLine(sessHeader, w))
	out = append(out, fitLine(borderStyle.Render(strings.Repeat("─", w)), w))

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
		timeText := formatAgo(s.UpdatedAt)
		if isCur {
			mark = "▸ "
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
		}
		sName := terminalutil.SanitizeText(s.Name)
		rowLeft := mark + sName
		line := padBetween(nameStyle.Render(rowLeft), lipgloss.NewStyle().Foreground(t.Muted).Render(timeText), w-1)
		out = append(out, fitLine(line, w))
		m.regions = append(m.regions, Region{Rect: Rect{0, 3 + len(out) - 1, w, 1}, Kind: ActionSession, Value: s.ID})
	}

	// Section 2: Todos
	if len(out) < h-3 {
		done := 0
		for _, x := range m.session.Todos {
			if x.Status == "completed" {
				done++
			}
		}
		out = append(out, "")
		todoHeader := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render(fmt.Sprintf(" 󰄲 TODOS (%d/%d)", done, len(m.session.Todos)))
		out = append(out, fitLine(todoHeader, w))
		out = append(out, fitLine(borderStyle.Render(strings.Repeat("─", w)), w))

		if len(m.session.Todos) == 0 {
			out = append(out, fitLine(lipgloss.NewStyle().Foreground(t.Muted).Render("  no active todos"), w))
		} else {
			for _, x := range m.session.Todos {
				icon := "○"
				iStyle := lipgloss.NewStyle().Foreground(t.Muted)
				if x.Status == "completed" {
					icon = "✓"
					iStyle = lipgloss.NewStyle().Foreground(t.Success)
				} else if x.Status == "in_progress" {
					icon = "◉"
					iStyle = lipgloss.NewStyle().Foreground(t.Warning)
				}
				line := "  " + iStyle.Render(icon) + " " + terminalutil.SanitizeText(x.Subject)
				out = append(out, fitLine(line, w))
				m.regions = append(m.regions, Region{Rect: Rect{0, 3 + len(out) - 1, w, 1}, Kind: ActionTodo, Value: x.ID})
				if len(out) >= h-4 {
					break
				}
			}
		}
	}

	// Section 3: Subagents
	if len(m.subagents) > 0 && len(out) < h-3 {
		out = append(out, "")
		subHeader := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render(" 󰚩 SUBAGENTS")
		out = append(out, fitLine(subHeader, w))
		out = append(out, fitLine(borderStyle.Render(strings.Repeat("─", w)), w))
		for _, a := range m.subagents {
			icon := "○"
			iStyle := lipgloss.NewStyle().Foreground(t.Muted)
			if a.State == "running" {
				icon = "◉"
				iStyle = lipgloss.NewStyle().Foreground(t.Warning)
			} else if a.State == "done" || a.State == "completed" {
				icon = "●"
				iStyle = lipgloss.NewStyle().Foreground(t.Success)
			}
			line := fmt.Sprintf("  %s %s · %s", iStyle.Render(icon), terminalutil.SanitizeText(a.AgentID), terminalutil.SanitizeText(a.State))
			out = append(out, fitLine(line, w))
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
	out := make([]string, 0, n+2)
	borderStyle := lipgloss.NewStyle().Foreground(t.Border)
	title := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("󰘧 Suggestions")
	tDash := max(1, w-lipgloss.Width(title)-5)
	out = append(out, fitLine(borderStyle.Render("╭─ ")+title+" "+borderStyle.Render(strings.Repeat("─", tDash)+"╮"), w))

	for i := 0; i < n; i++ {
		it := items[i]
		rowContent := ""
		if i == 0 {
			label := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(" ▸ " + terminalutil.SanitizeText(it.Label))
			desc := lipgloss.NewStyle().Foreground(t.Text).Render("  " + terminalutil.SanitizeText(it.Description))
			tabHint := lipgloss.NewStyle().Foreground(t.Muted).Render("[Tab]")
			rowContent = padBetween(label+desc, tabHint+" ", w-4)
		} else {
			label := lipgloss.NewStyle().Foreground(t.Text).Render("   " + terminalutil.SanitizeText(it.Label))
			desc := lipgloss.NewStyle().Foreground(t.Muted).Render("  " + terminalutil.SanitizeText(it.Description))
			rowContent = label + desc
		}
		gap := max(0, w-2-lipgloss.Width(rowContent))
		row := borderStyle.Render("│ ") + rowContent + strings.Repeat(" ", gap) + borderStyle.Render("│")
		out = append(out, fitLine(row, w))
	}
	out = append(out, fitLine(borderStyle.Render("╰"+strings.Repeat("─", max(1, w-2))+"╯"), w))
	return out
}

func (m *Model) statusLine(t Theme, w int) string {
	agentPill := lipgloss.NewStyle().Bold(true).Background(t.Primary).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Render("🤖 " + strings.ToUpper(terminalutil.SanitizeText(m.session.AgentID)))
	modelPill := lipgloss.NewStyle().Background(t.PillBg).Foreground(t.Secondary).Padding(0, 1).Render("󰘧 " + terminalutil.SanitizeText(m.session.Model))
	providerPill := lipgloss.NewStyle().Background(t.PillBg).Foreground(t.Muted).Padding(0, 1).Render("󰢏 " + terminalutil.SanitizeText(m.session.ProviderID))
	effortPill := lipgloss.NewStyle().Background(t.PillBg).Foreground(t.Warning).Padding(0, 1).Render("⚡ " + m.currentEffort())

	left := agentPill + " " + modelPill + " " + providerPill + " " + effortPill

	center := ""
	if m.busy {
		frames := []string{"◐", "◓", "◑", "◒"}
		spin := frames[m.activityFrame%len(frames)]
		center = lipgloss.NewStyle().Foreground(t.Warning).Bold(true).Render(fmt.Sprintf(" %s Generating...", spin))
	} else if m.toast != "" {
		center = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(" 󰋽 " + m.toast)
	} else {
		center = lipgloss.NewStyle().Foreground(t.Success).Render(" 󰄴 Ready")
	}

	storagePill := lipgloss.NewStyle().Background(t.PillBg).Foreground(t.Muted).Padding(0, 1).Render("💾 " + m.ctrl.Config.Snapshot().Storage)
	keyHints := lipgloss.NewStyle().Foreground(t.Muted).Render("[^P Menu] [^B Sidebar] [^D Diff] [^C Exit]")

	right := storagePill + " " + keyHints
	if w < 110 {
		right = storagePill
	}
	if w < 80 {
		left = agentPill + " " + modelPill
	}

	return padBetween(left+center, right+" ", w)
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
		lines = append(lines, lipgloss.NewStyle().Bold(true).Background(t.Secondary).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Render("󰄲 Task Todos"), "")
		if len(m.session.Todos) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("  No active todos for this session."))
		} else {
			for _, x := range m.session.Todos {
				icon := "○"
				iStyle := lipgloss.NewStyle().Foreground(t.Muted)
				if x.Status == "completed" {
					icon = "✓"
					iStyle = lipgloss.NewStyle().Foreground(t.Success)
				} else if x.Status == "in_progress" {
					icon = "◉"
					iStyle = lipgloss.NewStyle().Foreground(t.Warning)
				}
				row := fmt.Sprintf("  %s %s  [%s]", iStyle.Render(icon), terminalutil.SanitizeText(x.Subject), terminalutil.SanitizeText(string(x.Status)))
				lines = append(lines, row)
				regs = append(regs, Region{Rect: Rect{0, len(lines) - 1, max(1, w-4), 1}, Kind: ActionTodo, Value: x.ID})
			}
		}
	case overlayConnect:
		labels := []string{"Provider name", "Base URL (include /v1)", "Models (comma separated)", "API Key (optional)"}
		step := m.connect.Step
		if step < 0 || step >= len(labels) {
			step = 0
		}
		title := lipgloss.NewStyle().Bold(true).Background(t.Primary).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Render("󰢏 Connect Provider")
		stepInfo := lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("Step %d of %d", step+1, len(labels)))
		lines = []string{
			title + " " + stepInfo,
			"",
			lipgloss.NewStyle().Bold(true).Foreground(t.Text).Render(labels[step]),
			lipgloss.NewStyle().Foreground(t.Primary).Render(maskConnectInput(m.connect.Input, step)),
			"",
			lipgloss.NewStyle().Foreground(t.Muted).Render("↵ Next · Esc Cancel"),
		}
	case overlayConfirmRestore:
		lines = []string{
			lipgloss.NewStyle().Bold(true).Background(t.Error).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Render("Restore Factory Settings"),
			"",
			"This will delete all sessions and configuration data.",
			"",
			"Press y/Enter to confirm · n/Esc to cancel",
		}
	case overlayConfirmDelete:
		lines = []string{
			lipgloss.NewStyle().Bold(true).Background(t.Error).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Render("Delete Session"),
			"",
			terminalutil.SanitizeText(m.infoText),
			"",
			"Press y/Enter to confirm · n/Esc to cancel",
		}
	default:
		title := m.infoTitle
		if title == "" {
			title = "Info"
		}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Background(t.Primary).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Render(title), "")
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
