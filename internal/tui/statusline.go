package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/approval"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	ctxutil "github.com/ViudiraTech/Uinxed-Agent/internal/context"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

// statusSegment is one rendered piece of the status bar. Width is captured at
// build time so the layout pass does not have to re-measure styled strings.
type statusSegment struct {
	id    string
	text  string
	width int
	kind  ActionKind // non-empty makes the segment clickable
	value string
}

// dropOrder lists segments by how readily they may be discarded when the
// terminal is narrow. Later entries survive longer; "model" is never dropped.
// "mode" is dropped first: it is the most recoverable via Shift+Tab.
var statusDropOrder = []string{"mode", "session", "storage", "provider", "effort", "agent", "cwd", "context", "model"}

func dropRank(id string) int {
	for i, x := range statusDropOrder {
		if x == id {
			return i
		}
	}
	return -1
}

// statusLine renders the single dim row under the composer and returns the
// clickable regions it registered, already in terminal coordinates.
func (m *Model) statusLine(t Theme, w int) (string, []Region) {
	sep := lipgloss.NewStyle().Foreground(t.Border).Render(" " + t.Glyphs.Sep + " ")

	items := m.cfg.StatusItems
	if len(items) == 0 {
		items = config.DefaultStatusItems()
	}

	segs := make([]statusSegment, 0, len(items))
	for _, id := range items {
		if s, ok := m.statusSegment(t, id); ok {
			segs = append(segs, s)
		}
	}

	sepW := lipgloss.Width(sep)
	// Budget: one leading space, plus separators between surviving segments.
	fit := func(n int) bool {
		total := 1
		for i := 0; i < n; i++ {
			total += segs[i].width
			if i > 0 {
				total += sepW
			}
		}
		return total <= w
	}

	// Discard the least important segments until the row fits. Lower ranks are
	// discarded first; "model" ranks last and so survives until only it remains.
	for len(segs) > 1 && !fit(len(segs)) {
		worst := 0
		worstRank := -1
		for i, s := range segs {
			r := dropRank(s.id)
			if worstRank == -1 || r < worstRank {
				worstRank, worst = r, i
			}
		}
		segs = append(segs[:worst], segs[worst+1:]...)
	}

	var b strings.Builder
	var regions []Region
	b.WriteString(" ")
	x := 1
	for i, s := range segs {
		if i > 0 {
			b.WriteString(sep)
			x += sepW
		}
		b.WriteString(s.text)
		if s.kind != "" {
			regions = append(regions, Region{Rect: Rect{x, m.height - 1, s.width, 1}, Kind: s.kind, Value: s.value})
		}
		x += s.width
	}

	line := b.String()
	if right := m.busyIndicator(t); right != "" {
		if lipgloss.Width(line)+lipgloss.Width(right)+2 <= w {
			line = padBetween(line, right+" ", w)
		}
	}
	return fitLine(line, w), regions
}

func (m *Model) statusSegment(t Theme, id string) (statusSegment, bool) {
	muted := lipgloss.NewStyle().Foreground(t.Muted)
	switch id {
	case "model":
		text := terminalutil.SanitizeText(m.session.Model)
		if text == "" {
			return statusSegment{}, false
		}
		st := muted.Render(text)
		return statusSegment{id: id, text: st, width: lipgloss.Width(st), kind: ActionModel, value: m.session.Model}, true

	case "cwd":
		text := shortenHome(m.session.CWD)
		if text == "" {
			return statusSegment{}, false
		}
		text = terminalutil.SanitizeText(text)
		st := muted.Render(text)
		return statusSegment{id: id, text: st, width: lipgloss.Width(st)}, true

	case "context":
		used := ctxutil.EstimateMessages(m.session.Messages)
		win := ctxutil.Window(m.session.Model)
		if win <= 0 {
			return statusSegment{}, false
		}
		pct := used * 100 / win
		// Stay dim until the context is actually worth worrying about. A green
		// "0% ctx" pulls the eye for no reason.
		style := muted
		if pct > 80 {
			style = lipgloss.NewStyle().Foreground(t.Error)
		} else if pct > 60 {
			style = lipgloss.NewStyle().Foreground(t.Warning)
		}
		st := style.Render(fmt.Sprintf("%d%% ctx", pct))
		return statusSegment{id: id, text: st, width: lipgloss.Width(st)}, true

	case "agent":
		text := terminalutil.SanitizeText(m.session.AgentID)
		if text == "" {
			return statusSegment{}, false
		}
		st := muted.Render(text)
		return statusSegment{id: id, text: st, width: lipgloss.Width(st), kind: ActionAgent, value: m.session.AgentID}, true

	case "provider":
		text := terminalutil.SanitizeText(m.session.ProviderID)
		if text == "" {
			return statusSegment{}, false
		}
		st := muted.Render(text)
		return statusSegment{id: id, text: st, width: lipgloss.Width(st), kind: ActionProvider, value: m.session.ProviderID}, true

	case "effort":
		st := muted.Render("effort " + m.currentEffort())
		return statusSegment{id: id, text: st, width: lipgloss.Width(st)}, true

	case "storage":
		st := muted.Render(m.cfg.Storage)
		return statusSegment{id: id, text: st, width: lipgloss.Width(st)}, true

	case "mode":
		mode := approval.Normalize(m.currentMode())
		// plan gets a distinct pause glyph; the other modes share the
		// fast-forward mark so the ring position reads at a glance.
		text := "⏵⏵ " + string(mode)
		style := muted
		if mode == approval.ModePlan {
			text = "⏸ plan"
			style = lipgloss.NewStyle().Foreground(t.Warning)
		}
		st := style.Render(text)
		return statusSegment{id: id, text: st, width: lipgloss.Width(st)}, true

	case "session":
		text := terminalutil.SanitizeText(m.session.Name)
		if text == "" {
			return statusSegment{}, false
		}
		st := muted.Render(truncWidth(text, 28))
		return statusSegment{id: id, text: st, width: lipgloss.Width(st)}, true
	}
	return statusSegment{}, false
}

// shortenHome collapses the home prefix to ~ and, when the result is still too
// long for a status bar, keeps the trailing path components.
func shortenHome(p string) string {
	if p == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if strings.HasPrefix(p, home+string(filepath.Separator)) {
			p = "~" + p[len(home):]
		}
	}
	const maxLen = 36
	if len(p) <= maxLen {
		return p
	}
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) <= 2 {
		return p
	}
	return "…" + string(filepath.Separator) + strings.Join(parts[len(parts)-2:], string(filepath.Separator))
}
