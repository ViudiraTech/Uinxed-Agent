package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

type PickerItem struct{ ID, Label, Description, Shortcut string }
type Picker struct {
	Title    string
	Kind     ActionKind
	Items    []PickerItem
	Filtered []int
	Query    string
	Index    int
	Scroll   int
}

func (p *Picker) Reset(title string, kind ActionKind, items []PickerItem) {
	p.Title = title
	p.Kind = kind
	p.Items = items
	p.Query = ""
	p.Index = 0
	p.Scroll = 0
	p.filter()
}
func (p *Picker) SetQuery(q string) { p.Query = q; p.Index = 0; p.Scroll = 0; p.filter() }
func (p *Picker) filter() {
	p.Filtered = p.Filtered[:0]
	q := strings.ToLower(strings.TrimSpace(p.Query))
	for i, it := range p.Items {
		if q == "" || fuzzy(strings.ToLower(it.Label+" "+terminalutil.SanitizeText(it.Description)), q) >= 0 {
			p.Filtered = append(p.Filtered, i)
		}
	}
}
func (p *Picker) Move(d int) {
	if len(p.Filtered) == 0 {
		return
	}
	p.Index += d
	if p.Index < 0 {
		p.Index = 0
	}
	if p.Index >= len(p.Filtered) {
		p.Index = len(p.Filtered) - 1
	}
}
func (p *Picker) Selected() (PickerItem, bool) {
	if len(p.Filtered) == 0 || p.Index < 0 || p.Index >= len(p.Filtered) {
		return PickerItem{}, false
	}
	return p.Items[p.Filtered[p.Index]], true
}
func (p *Picker) Render(width, height int, t Theme, hover string) ([]string, []Region) {
	if width < 30 {
		width = 30
	}
	if height < 5 {
		height = 5
	}
	visible := height - 3
	if visible < 1 {
		visible = 1
	}
	if p.Index < p.Scroll {
		p.Scroll = p.Index
	}
	if p.Index >= p.Scroll+visible {
		p.Scroll = p.Index - visible + 1
	}
	var lines []string
	var regions []Region
	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("󰍉 " + terminalutil.SanitizeText(p.Title))
	search := lipgloss.NewStyle().Foreground(t.Secondary).Render("🔍 ") + lipgloss.NewStyle().Foreground(t.Text).Bold(true).Render(terminalutil.SanitizeText(p.Query)) + lipgloss.NewStyle().Foreground(t.Muted).Render("▏")
	lines = append(lines, title, search)
	end := min(len(p.Filtered), p.Scroll+visible)
	for row, j := range p.Filtered[p.Scroll:end] {
		it := p.Items[j]
		sel := p.Scroll+row == p.Index || hover == it.ID
		label := terminalutil.SanitizeText(it.Label)
		desc := terminalutil.SanitizeText(it.Description)
		shortcut := terminalutil.SanitizeText(it.Shortcut)

		if sel {
			left := " ▸ " + label
			if desc != "" {
				left += "  " + desc
			}
			right := ""
			if shortcut != "" {
				right = "[" + shortcut + "]"
			}
			rowLine := padBetween(left, right, width)
			styled := lipgloss.NewStyle().Bold(true).Background(t.SelectionBg).Foreground(t.SelectionFg).Render(truncWidth(rowLine, width))
			lines = append(lines, styled)
		} else {
			left := "   " + label
			if desc != "" {
				left += "  " + lipgloss.NewStyle().Foreground(t.Muted).Render(desc)
			}
			right := ""
			if shortcut != "" {
				right = lipgloss.NewStyle().Foreground(t.Muted).Render("[" + shortcut + "]")
			}
			rowLine := padBetween(left, right, width)
			styled := lipgloss.NewStyle().Foreground(t.Text).Render(truncWidth(rowLine, width))
			lines = append(lines, styled)
		}
		regions = append(regions, Region{Rect: Rect{0, row + 2, width, 1}, Kind: ActionPicker, Value: it.ID})
	}
	return lines, regions
}
func fuzzy(s, q string) int {
	if q == "" {
		return 0
	}
	qi, score, last := 0, 0, -2
	for i := 0; i < len(s) && qi < len(q); i++ {
		if s[i] == q[qi] {
			score += 10
			if i == last+1 {
				score += 5
			}
			last = i
			qi++
		}
	}
	if qi < len(q) {
		return -1
	}
	return score - (len(s)-len(q))/8
}
func truncWidth(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w < 2 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}
