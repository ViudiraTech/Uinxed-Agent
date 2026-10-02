package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

// effortGuide pairs each level with what it is for. The guide line is the point
// of the slider: Claude Code swaps it as you move along the scale, which is what
// makes a row of five words a choice rather than a vocabulary test.
var effortGuide = map[string]string{
	"low":    "Quick exchanges you review as you go — brainstorming, a first sketch, a rename",
	"medium": "Day-to-day work with a clear scope, such as implementing a described feature",
	"high":   "Work where verification matters or edge cases are likely, such as fixing a bug",
	"xhigh":  "Deeper reasoning at higher token spend",
	"max":    "Hard problems worked through without you. May show diminishing returns and overthink",
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func effortIndex(level string) int {
	for i, l := range config.EffortLevels() {
		if l == level {
			return i
		}
	}
	return -1
}

// supercodeOn reports the concurrent-orchestration toggle, session override
// first. Like Claude Code's ultracode it is independent of the effort level.
func (m *Model) supercodeOn() bool {
	if v, ok := m.session.Metadata["supercode"].(bool); ok {
		return v
	}
	return m.cfg.Supercode
}

func (m *Model) openEffortSlider() {
	m.effortSel = effortIndex(m.currentEffort())
	if m.effortSel < 0 {
		m.effortSel = effortIndex("high")
	}
	m.overlay = overlayEffort
	m.setFocus(FocusOverlay)
}

// handleEffortKey drives the slider. Enter saves the level as the default, s
// applies it to this session only, and Tab flips supercode without moving the
// level — the same split Claude Code draws between "save" and "this session".
func (m *Model) handleEffortKey(k tea.KeyPressMsg) tea.Cmd {
	levels := config.EffortLevels()
	switch k.String() {
	case "esc":
		m.closeOverlay()
	case "left", "h":
		m.effortSel = max(0, m.effortSel-1)
	case "right", "l":
		m.effortSel = min(len(levels)-1, m.effortSel+1)
	case "tab":
		return m.toggleSupercode()
	case "enter":
		return m.applyEffort(false)
	case "s":
		return m.applyEffort(true)
	}
	return nil
}

func (m *Model) applyEffort(sessionOnly bool) tea.Cmd {
	levels := config.EffortLevels()
	if m.effortSel < 0 || m.effortSel >= len(levels) {
		return nil
	}
	level := levels[m.effortSel]
	m.closeOverlay()
	sid := m.session.ID
	return asyncOp("set_effort", func() (any, error) {
		if err := m.ctrl.SetEffort(m.ctx, sid, level, sessionOnly); err != nil {
			return nil, err
		}
		return level, nil
	})
}

func (m *Model) toggleSupercode() tea.Cmd {
	sid := m.session.ID
	next := !m.supercodeOn()
	return asyncOp("set_supercode", func() (any, error) {
		if err := m.ctrl.SetSupercode(m.ctx, sid, next, false); err != nil {
			return nil, err
		}
		return next, nil
	})
}

// handleEffortCommand is /effort: no argument opens the slider, a level name sets
// it, auto drops the session's override, and supercode flips the toggle.
func (m *Model) handleEffortCommand(arg string) tea.Cmd {
	sid := m.session.ID
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
		m.openEffortSlider()
		return nil
	case "auto":
		return asyncOp("clear_effort", func() (any, error) { return nil, m.ctrl.ClearEffort(m.ctx, sid) })
	case "supercode":
		return m.toggleSupercode()
	case "supercode on", "supercode off":
		on := strings.HasSuffix(strings.ToLower(strings.TrimSpace(arg)), "on")
		return asyncOp("set_supercode", func() (any, error) {
			if err := m.ctrl.SetSupercode(m.ctx, sid, on, false); err != nil {
				return nil, err
			}
			return on, nil
		})
	}
	level := strings.ToLower(strings.TrimSpace(arg))
	if !config.ValidEffort(level) {
		m.showError(fmt.Errorf("effort must be %s, auto, or supercode",
			strings.Join(config.EffortLevels(), ", ")))
		return nil
	}
	return asyncOp("set_effort", func() (any, error) {
		if err := m.ctrl.SetEffort(m.ctx, sid, level, false); err != nil {
			return nil, err
		}
		return level, nil
	})
}

// renderEffortSlider draws the reasoning scale: the stops in a row with the
// chosen one highlighted and a caret beneath it, that stop's guide line below,
// and the keys last.
//
// There is deliberately no animation here. Claude Code's slider is a highlight
// and a description that changes as you move; the animated part of this feature
// is the composer border, which shimmers while supercode is on.
func (m *Model) renderEffortSlider(t Theme, w int) []string {
	levels := config.EffortLevels()
	sel := min(max(0, m.effortSel), len(levels)-1)

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary)
	on := lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
	off := lipgloss.NewStyle().Foreground(t.Muted)
	guide := lipgloss.NewStyle().Foreground(t.Text)

	head := title.Render("Reasoning effort")
	if m.supercodeOn() {
		head += on.Render("   supercode on")
	}
	out := []string{head, ""}

	// The stops are laid out first so the caret can be placed under the chosen
	// one by column rather than by guessing at the padding.
	const gap = 4
	offsets := make([]int, len(levels))
	var row strings.Builder
	row.WriteString("  ")
	col := 2
	for i, lv := range levels {
		if i > 0 {
			row.WriteString(strings.Repeat(" ", gap))
			col += gap
		}
		offsets[i] = col
		if i == sel {
			row.WriteString(on.Render(lv))
		} else {
			row.WriteString(off.Render(lv))
		}
		col += lipgloss.Width(lv)
	}
	out = append(out, row.String())
	out = append(out, strings.Repeat(" ", offsets[sel])+on.Render(t.Glyphs.Caret))

	body := w - 4
	if body < 10 {
		body = 10
	}
	for _, l := range wrapPlain(effortGuide[levels[sel]], body) {
		out = append(out, "  "+guide.Render(l))
	}

	out = append(out, "")
	flip := "tab supercode"
	if m.supercodeOn() {
		flip = "tab supercode off"
	}
	out = append(out, off.Render("  ← → choose · enter save as default · s this session only · "+flip+" · esc cancel"))
	return out
}
