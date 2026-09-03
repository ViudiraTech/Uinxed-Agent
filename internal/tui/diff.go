package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	gitutil "github.com/ViudiraTech/Uinxed-Agent/internal/git"
)

type DiffView struct {
	Snapshot gitutil.Snapshot
	Selected int
	Scroll   int
	FileText string
}

func (d *DiffView) Set(s gitutil.Snapshot) {
	d.Snapshot = s
	d.Selected = 0
	d.Scroll = 0
	d.FileText = ""
	if len(s.Files) > 0 {
		d.FileText = s.Unified
	}
}
func (d *DiffView) ScrollBy(n int) {
	d.Scroll += n
	if d.Scroll < 0 {
		d.Scroll = 0
	}
}
func (d *DiffView) Render(width, height int, t Theme) ([]string, []Region) {
	if height < 4 {
		return []string{"Diff"}, nil
	}
	var lines []string
	var regs []Region
	borderStyle := lipgloss.NewStyle().Foreground(t.Border)

	if width >= 90 {
		left := min(36, width/3)
		right := width - left - 3
		fileLines := make([]string, 0, height)
		fileTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(fmt.Sprintf("Changed Files (%d)", len(d.Snapshot.Files)))
		fileLines = append(fileLines, fitLine(fileTitle, left))
		fileLines = append(fileLines, fitLine(borderStyle.Render(strings.Repeat("─", left)), left))

		for i, f := range d.Snapshot.Files {
			isSel := i == d.Selected
			mark := "  "
			if isSel {
				mark = "▸ "
			}
			fName := f.Path
			statStyled := lipgloss.NewStyle().Foreground(t.Success).Render(fmt.Sprintf("+%d", f.Added)) + " " + lipgloss.NewStyle().Foreground(t.Error).Render(fmt.Sprintf("-%d", f.Deleted))
			fStyle := lipgloss.NewStyle().Foreground(t.Text)
			if isSel {
				fStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
			}
			row := padBetween(fStyle.Render(mark+truncWidth(fName, max(6, left-10))), statStyled, left)
			fileLines = append(fileLines, row)
			regs = append(regs, Region{Rect: Rect{0, i + 2, left, 1}, Kind: ActionDiffFile, Value: f.Path})
		}
		for len(fileLines) < height {
			fileLines = append(fileLines, "")
		}

		diff := d.renderDiff(right, height, t)
		for i := 0; i < height; i++ {
			a, b := "", ""
			if i < len(fileLines) {
				a = fileLines[i]
			}
			if i < len(diff) {
				b = diff[i]
			}
			lines = append(lines, fitLine(a, left)+borderStyle.Render(" │ ")+fitLine(b, right))
		}
	} else {
		head := "Diff"
		if len(d.Snapshot.Files) > 0 {
			head += " · " + d.Snapshot.Files[d.Selected].Path
		}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(truncWidth(head, width)))
		lines = append(lines, d.renderDiff(width, height-1, t)...)
	}
	return lines, regs
}
func (d *DiffView) renderDiff(width, height int, t Theme) []string {
	text := d.FileText
	if text == "" {
		text = d.Snapshot.Unified
	}
	src := strings.Split(text, "\n")
	if d.Scroll > len(src) {
		d.Scroll = max(0, len(src)-1)
	}
	end := min(len(src), d.Scroll+height)
	var out []string
	for _, l := range src[d.Scroll:end] {
		st := lipgloss.NewStyle().Foreground(t.Text)
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			st = st.Foreground(t.DiffAdd)
		} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			st = st.Foreground(t.DiffDelete)
		} else if strings.HasPrefix(l, "@@") {
			st = st.Foreground(t.Primary)
		}
		out = append(out, st.Render(truncWidth(l, width)))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}
