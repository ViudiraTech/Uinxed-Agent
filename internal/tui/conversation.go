package tui

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	md "github.com/ViudiraTech/Uinxed-Agent/internal/markdown"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

type convBlock struct {
	ID        string
	Role      domain.Role
	Content   string
	Reasoning string
	ToolCalls []domain.ToolCall
	Estimate  int
	Version   int
}

type renderLine struct {
	Text   string
	Action ActionKind
	Value  string
}

type Conversation struct {
	blocks           []convBlock
	cache            *md.Cache
	scroll           int
	expandedTools    map[string]bool
	expandedThinking map[string]bool
	sessionID        string
	width            int
	theme            string
}

func NewConversation() *Conversation {
	return &Conversation{
		cache:            md.NewCache(1024),
		expandedTools:    map[string]bool{},
		expandedThinking: map[string]bool{},
	}
}

func (c *Conversation) SetSession(s domain.Session, width int) {
	c.width = width
	sameSession := c.sessionID != "" && c.sessionID == s.ID
	if !sameSession {
		c.expandedThinking = map[string]bool{}
		c.scroll = 0
	}
	c.sessionID = s.ID
	c.blocks = c.blocks[:0]
	for i, m := range s.Messages {
		if m.Role == domain.RoleSystem || m.Role == domain.RoleTool {
			continue
		}
		id := m.ID
		if id == "" {
			id = fmt.Sprintf("%s-%d", m.Role, i)
		}
		c.blocks = append(c.blocks, convBlock{ID: id, Role: m.Role, Content: m.Content, Reasoning: m.ReasoningContent, ToolCalls: m.ToolCalls, Estimate: estimateBlockWithReasoning(m.Content, m.ReasoningContent, width, len(m.ToolCalls), c.expandedThinking[id]), Version: len(m.Content) + len(m.ReasoningContent)})
	}
}
func (c *Conversation) SetTheme(t string) {
	if c.theme != t {
		c.theme = t
		c.cache.Clear()
	}
}
func (c *Conversation) SetWidth(w int) {
	if w < 20 {
		w = 20
	}
	if c.width != w {
		c.width = w
		for i := range c.blocks {
			b := &c.blocks[i]
			b.Estimate = estimateBlockWithReasoning(b.Content, b.Reasoning, w, len(b.ToolCalls), c.expandedThinking[b.ID])
		}
	}
}
func (c *Conversation) ScrollUp(n int) {
	if n < 1 {
		n = 1
	}
	c.scroll += n
	max := c.totalEstimate()
	if c.scroll > max {
		c.scroll = max
	}
}
func (c *Conversation) ScrollDown(n int) {
	if n < 1 {
		n = 1
	}
	c.scroll -= n
	if c.scroll < 0 {
		c.scroll = 0
	}
}
func (c *Conversation) GotoBottom()          { c.scroll = 0 }
func (c *Conversation) ToggleTool(id string) { c.expandedTools[id] = !c.expandedTools[id] }

// ToggleThinking expands/collapses one reasoning block and keeps the viewport
// anchored. Without scroll compensation, adding the reasoning lines increases
// total conversation height and a bottom-anchored viewport immediately jumps
// past the block on the next render, which looks like a one-frame flash.
func (c *Conversation) ToggleThinking(id, streamReasoning string) {
	if id == "" {
		return
	}
	open := !c.expandedThinking[id]
	delta := c.reasoningHeight(id, streamReasoning)
	c.expandedThinking[id] = open
	c.adjustReasoningEstimate(id, delta, open)
	if delta <= 0 {
		return
	}
	if open {
		c.scroll += delta
	} else {
		c.scroll -= delta
		if c.scroll < 0 {
			c.scroll = 0
		}
	}
}

func (c *Conversation) ToggleAllThinking(streamReasoning string) {
	ids := make([]string, 0, len(c.blocks)+1)
	anyCollapsed := false
	for _, b := range c.blocks {
		if strings.TrimSpace(b.Reasoning) == "" {
			continue
		}
		ids = append(ids, b.ID)
		if !c.expandedThinking[b.ID] {
			anyCollapsed = true
		}
	}
	if strings.TrimSpace(streamReasoning) != "" {
		ids = append(ids, "__stream__")
		if !c.expandedThinking["__stream__"] {
			anyCollapsed = true
		}
	}
	for _, id := range ids {
		if c.expandedThinking[id] == anyCollapsed {
			continue
		}
		delta := c.reasoningHeight(id, streamReasoning)
		c.expandedThinking[id] = anyCollapsed
		c.adjustReasoningEstimate(id, delta, anyCollapsed)
		if anyCollapsed {
			c.scroll += delta
		} else {
			c.scroll -= delta
		}
	}
	if c.scroll < 0 {
		c.scroll = 0
	}
}

func (c *Conversation) reasoningHeight(id, streamReasoning string) int {
	reasoning := ""
	if id == "__stream__" {
		reasoning = streamReasoning
	} else {
		for _, b := range c.blocks {
			if b.ID == id {
				reasoning = b.Reasoning
				break
			}
		}
	}
	reasoning = terminalutil.SanitizeText(reasoning)
	if strings.TrimSpace(reasoning) == "" {
		return 0
	}
	return len(wrapPlain(reasoning, max(1, c.width-2)))
}

func (c *Conversation) adjustReasoningEstimate(id string, delta int, open bool) {
	if id == "__stream__" || delta <= 0 {
		return
	}
	for i := range c.blocks {
		if c.blocks[i].ID != id {
			continue
		}
		if open {
			c.blocks[i].Estimate += delta
		} else {
			c.blocks[i].Estimate = max(1, c.blocks[i].Estimate-delta)
		}
		return
	}
}

func (c *Conversation) ToggleAllTools() {
	var ids []string
	anyCollapsed := false
	for _, b := range c.blocks {
		for _, tc := range b.ToolCalls {
			ids = append(ids, tc.ID)
			if !c.expandedTools[tc.ID] {
				anyCollapsed = true
			}
		}
	}
	for _, id := range ids {
		c.expandedTools[id] = anyCollapsed
	}
}

func (c *Conversation) Render(height int, t Theme, streamContent, streamReasoning string, activities []domain.ToolActivity, hoverValue ...string) []renderLine {
	hover := ""
	if len(hoverValue) > 0 {
		hover = hoverValue[0]
	}
	if height <= 0 {
		return nil
	}
	width := c.width
	if width < 20 {
		width = 20
	}
	total := c.totalEstimate()
	streamEst := 0
	if streamContent != "" || streamReasoning != "" {
		streamEst = estimateBlockWithReasoning(streamContent, streamReasoning, width, 0, c.expandedThinking["__stream__"])
	}
	total += streamEst
	bottom := total - c.scroll
	if bottom < 0 {
		bottom = 0
	}
	top := bottom - height - 12
	if top < 0 {
		top = 0
	}
	wantBottom := bottom + 12
	pos := 0
	var lines []renderLine
	acts := map[string]domain.ToolActivity{}
	for _, a := range activities {
		acts[a.CallID] = a
	}
	for i := range c.blocks {
		b := &c.blocks[i]
		end := pos + b.Estimate
		if end >= top && pos <= wantBottom {
			rendered := c.renderBlock(*b, t, acts, hover)
			if len(rendered) != b.Estimate {
				delta := len(rendered) - b.Estimate
				b.Estimate = len(rendered)
				total += delta
				bottom += delta
			}
			lines = append(lines, rendered...)
		} else if end >= top-20 && pos <= wantBottom+20 {
			// Small overscan gets rendered once so future height estimates become exact.
			rendered := c.renderBlock(*b, t, acts, hover)
			b.Estimate = len(rendered)
		}
		pos += b.Estimate
	}
	if streamContent != "" || streamReasoning != "" {
		b := convBlock{ID: "__stream__", Role: domain.RoleAssistant, Content: streamContent, Reasoning: streamReasoning, Estimate: streamEst, Version: len(streamContent) + len(streamReasoning)}
		lines = append(lines, c.renderBlock(b, t, acts, hover)...)
	}
	// Slice from bottom using actual visible line list. When scrolled far into lazily skipped blocks,
	// estimates keep the location stable while only overscan blocks are materialized.
	if len(lines) > height {
		cut := len(lines) - height - c.scroll
		if cut < 0 {
			cut = 0
		}
		end := cut + height
		if end > len(lines) {
			end = len(lines)
			cut = max(0, end-height)
		}
		lines = lines[cut:end]
	}
	for len(lines) < height {
		lines = append(lines, renderLine{})
	}
	return lines
}

func (c *Conversation) renderBlock(b convBlock, t Theme, acts map[string]domain.ToolActivity, hover string) []renderLine {
	var out []renderLine
	label := "USER"
	color := t.User
	if b.Role == domain.RoleAssistant {
		label = "ASSISTANT"
		color = t.Assistant
	}
	out = append(out, renderLine{Text: lipgloss.NewStyle().Bold(true).Foreground(color).Render(label)})
	b.Reasoning = terminalutil.SanitizeText(b.Reasoning)
	if b.Reasoning != "" {
		if c.expandedThinking[b.ID] {
			out = append(out, renderLine{Text: lipgloss.NewStyle().Foreground(t.Muted).Bold(hover == "thinking:"+b.ID).Render("◇ Reasoning"), Action: ActionThinking, Value: "thinking:" + b.ID})
			for _, l := range wrapPlain(b.Reasoning, c.width-2) {
				out = append(out, renderLine{Text: lipgloss.NewStyle().Foreground(t.Muted).Render("  " + l)})
			}
		} else {
			out = append(out, renderLine{Text: lipgloss.NewStyle().Foreground(t.Muted).Bold(hover == "thinking:"+b.ID).Render("◇ Reasoning · Enter/Click to expand"), Action: ActionThinking, Value: "thinking:" + b.ID})
		}
	}
	if strings.TrimSpace(b.Content) != "" {
		rendered, err := c.cache.Render(b.ID, b.Version, c.width, c.theme, b.Content)
		if err != nil {
			rendered = md.PlainFallback(b.Content, c.width)
		}
		for _, l := range strings.Split(rendered, "\n") {
			out = append(out, renderLine{Text: l})
		}
	}
	for _, tc := range b.ToolCalls {
		a := acts[tc.ID]
		state := "○"
		fg := t.Muted
		switch a.State {
		case "running":
			state = "◉"
			fg = t.Warning
		case "success":
			state = "●"
			fg = t.Success
		case "failed":
			state = "×"
			fg = t.Error
		}
		name := terminalutil.SanitizeText(tc.Function.Name)
		summary := toolSummary(name, terminalutil.SanitizeText(tc.Function.Arguments))
		line := fmt.Sprintf("%s %-9s %s", state, name, summary)
		style := lipgloss.NewStyle().Foreground(fg)
		if hover == tc.ID {
			style = style.Bold(true)
		}
		out = append(out, renderLine{Text: style.Render(line), Action: ActionTool, Value: tc.ID})
		if c.expandedTools[tc.ID] {
			if tc.Function.Arguments != "" {
				for _, l := range wrapPlain("args: "+tc.Function.Arguments, c.width-4) {
					out = append(out, renderLine{Text: lipgloss.NewStyle().Foreground(t.Muted).Render("  " + l)})
				}
			}
			if a.Output != "" {
				for _, l := range wrapPlain(terminalutil.SanitizeText(a.Output), c.width-4) {
					out = append(out, renderLine{Text: "  " + l})
				}
			}
			if a.Error != "" {
				for _, l := range wrapPlain(terminalutil.SanitizeText(a.Error), c.width-4) {
					out = append(out, renderLine{Text: lipgloss.NewStyle().Foreground(t.Error).Render("  " + l)})
				}
			}
		}
	}
	out = append(out, renderLine{})
	return out
}

func (c *Conversation) totalEstimate() int {
	n := 0
	for _, b := range c.blocks {
		n += b.Estimate
	}
	return n
}
func estimateBlockWithReasoning(content, reasoning string, width, tools int, expanded bool) int {
	n := estimateBlock(content, width, tools)
	reasoning = terminalutil.SanitizeText(reasoning)
	if strings.TrimSpace(reasoning) == "" {
		return n
	}
	n++ // reasoning header
	if expanded {
		n += len(wrapPlain(reasoning, max(1, width-2)))
	}
	return n
}

func estimateBlock(content string, width, tools int) int {
	if width < 20 {
		width = 20
	}
	lines := 2 + tools
	for _, l := range strings.Split(content, "\n") {
		lines += max(1, int(math.Ceil(float64(max(1, len([]rune(l))))/float64(width))))
	}
	return lines
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func wrapPlain(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, l := range strings.Split(s, "\n") {
		r := []rune(l)
		if len(r) == 0 {
			out = append(out, "")
			continue
		}
		for len(r) > width {
			out = append(out, string(r[:width]))
			r = r[width:]
		}
		out = append(out, string(r))
	}
	return out
}
func toolSummary(name, args string) string {
	args = strings.ReplaceAll(strings.TrimSpace(args), "\n", " ")
	if len([]rune(args)) > 70 {
		args = string([]rune(args)[:70]) + "…"
	}
	if args == "" {
		return ""
	}
	return args
}
