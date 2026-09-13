package tui

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	md "github.com/ViudiraTech/Uinxed-Agent/internal/markdown"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
	"github.com/charmbracelet/x/ansi"
)

type convBlock struct {
	ID string
	// StreamID identifies the live message, so the incremental wrapper can tell
	// one model round from the next. Empty for everything but the live block.
	StreamID  string
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
	mdStyle          md.Style
	mdKey            string
	rendered         map[string]renderCacheEntry
	stream           streamWrap
}

// renderCacheEntry memoizes one block's rendered lines. Streaming only mutates
// the newest block, so without this every frame would rebuild every visible
// block: hundreds of lipgloss style constructions and string joins per repaint,
// which is what makes streaming feel sluggish.
type renderCacheEntry struct {
	key   renderKey
	lines []renderLine
}

// renderKey is every input that can change a block's rendered output. Anything
// missing here would show up as a stale line, so it deliberately includes the
// hover target (it brightens one element) and a tool-activity signature.
type renderKey struct {
	id      string
	version int
	width   int
	style   string
	think   bool
	toolSig string
	hover   string
	running bool
	// frame is only part of the key while something is animating, so an idle
	// block keeps a stable key instead of missing the cache on every frame.
	frame int
}

func NewConversation() *Conversation {
	return &Conversation{
		cache:            md.NewCache(1024),
		expandedTools:    map[string]bool{},
		expandedThinking: map[string]bool{},
		rendered:         map[string]renderCacheEntry{},
	}
}

// renderCached returns the memoized rendering of a block, recomputing only when
// one of its inputs changed.
func (c *Conversation) renderCached(b *convBlock, t Theme, acts map[string]domain.ToolActivity, hover string, frame int) []renderLine {
	key := renderKey{
		id:      b.ID,
		version: b.Version,
		width:   c.width,
		style:   c.mdKey,
		think:   c.expandedThinking[b.ID],
		toolSig: toolSignature(b.ToolCalls, acts, c.expandedTools),
		hover:   hover,
		running: hasRunningTool(b.ToolCalls, acts),
	}
	if key.running {
		key.frame = frame
	}
	if e, ok := c.rendered[b.ID]; ok && e.key == key {
		return e.lines
	}
	lines := c.renderBlock(*b, t, acts, hover, frame)
	if len(c.rendered) >= renderCacheMax {
		clear(c.rendered)
	}
	c.rendered[b.ID] = renderCacheEntry{key: key, lines: lines}
	return lines
}

const renderCacheMax = 1024

// hasRunningTool reports whether any call in the block is mid-flight, which is
// what makes its rendered output depend on the animation frame.
func hasRunningTool(calls []domain.ToolCall, acts map[string]domain.ToolActivity) bool {
	for _, tc := range calls {
		if strings.TrimSpace(tc.Function.Name) == "" {
			continue
		}
		if acts[tc.ID].State == "running" {
			return true
		}
	}
	return false
}

// toolSignature captures the per-call state a tool line depends on: its status
// glyph, how much output it has, and whether it is expanded.
func toolSignature(calls []domain.ToolCall, acts map[string]domain.ToolActivity, expanded map[string]bool) string {
	calls = sanitizeToolCalls(calls)
	if len(calls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, tc := range calls {
		a := acts[tc.ID]
		fmt.Fprintf(&b, "%s|%s|%d|%d|%t;", tc.ID, a.State, len(a.Output), len(a.Error), expanded[tc.ID])
	}
	return b.String()
}

// sanitizeToolCalls drops nameless calls so a ghost `unknown tool ""` never
// reaches the transcript. Providers occasionally stream a placeholder slot
// (index gap / heartbeat fragment) that the accumulator cannot always merge;
// older sessions may already have such calls persisted.
func sanitizeToolCalls(calls []domain.ToolCall) []domain.ToolCall {
	if len(calls) == 0 {
		return calls
	}
	out := make([]domain.ToolCall, 0, len(calls))
	for _, tc := range calls {
		if strings.TrimSpace(tc.Function.Name) == "" {
			continue
		}
		out = append(out, tc)
	}
	return out
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
		clean := sanitizeToolCalls(m.ToolCalls)
		c.blocks = append(c.blocks, convBlock{ID: id, Role: m.Role, Content: m.Content, Reasoning: m.ReasoningContent, ToolCalls: clean, Estimate: estimateBlockWithReasoning(m.Content, m.ReasoningContent, width, len(clean), c.expandedThinking[id]), Version: len(m.Content) + len(m.ReasoningContent)})
	}
}

// applyTheme syncs the Markdown renderer with the theme of the current frame.
// Deriving it here rather than caching it in a setter keeps Render's theme
// argument the single source of truth, so a glyph or color change can never
// leave stale Markdown in the cache.
func (c *Conversation) applyTheme(t Theme) {
	key := mdStyleKey(t)
	if c.mdKey != key {
		c.cache.Clear()
		c.mdKey = key
	}
	c.mdStyle = mdStyle(t)
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
func (c *Conversation) ToggleThinking(id string) {
	if id == "" {
		return
	}
	open := !c.expandedThinking[id]
	delta := c.reasoningHeight(id)
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

func (c *Conversation) ToggleAllThinking() {
	ids := make([]string, 0, len(c.blocks))
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
	// The live streaming block is always expanded while it is being written, so
	// it takes no part in the toggle.
	for _, id := range ids {
		if c.expandedThinking[id] == anyCollapsed {
			continue
		}
		delta := c.reasoningHeight(id)
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

func (c *Conversation) reasoningHeight(id string) int {
	reasoning := ""
	for _, b := range c.blocks {
		if b.ID == id {
			reasoning = b.Reasoning
			break
		}
	}
	reasoning = terminalutil.SanitizeText(reasoning)
	if strings.TrimSpace(reasoning) == "" {
		return 0
	}
	// Expanding replaces the single collapsed line with a header plus the
	// wrapped body, so the net height delta is exactly the body line count.
	return len(wrapPlain(reasoning, max(8, c.width-4)))
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
			if strings.TrimSpace(tc.Function.Name) == "" {
				continue
			}
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

// RenderOptions carries the per-frame state that is not part of the
// conversation itself: the live streaming buffers, tool activity, the current
// hover target, and the animation frame used for in-flight markers.
type RenderOptions struct {
	StreamContent   string
	StreamReasoning string
	StreamMessageID string
	Activities      []domain.ToolActivity
	Hover           string
	Frame           int
}

func (c *Conversation) Render(height int, t Theme, o RenderOptions) []renderLine {
	if height <= 0 {
		return nil
	}
	streamContent, streamReasoning := o.StreamContent, o.StreamReasoning
	activities, hover := o.Activities, o.Hover
	c.applyTheme(t)
	width := c.width
	if width < 20 {
		width = 20
	}
	total := c.totalEstimate()
	streamEst := 0
	if streamContent != "" || streamReasoning != "" {
		streamEst = estimateBlockWithReasoning(streamContent, streamReasoning, width, 0, c.reasoningExpanded("__stream__"))
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
			rendered := c.renderCached(b, t, acts, hover, o.Frame)
			if len(rendered) != b.Estimate {
				delta := len(rendered) - b.Estimate
				b.Estimate = len(rendered)
				total += delta
				bottom += delta
			}
			lines = append(lines, rendered...)
		} else if end >= top-20 && pos <= wantBottom+20 {
			// Small overscan gets rendered once so future height estimates become exact.
			rendered := c.renderCached(b, t, acts, hover, o.Frame)
			b.Estimate = len(rendered)
		}
		pos += b.Estimate
	}
	if streamContent != "" || streamReasoning != "" {
		b := convBlock{ID: "__stream__", StreamID: o.StreamMessageID, Role: domain.RoleAssistant, Content: streamContent, Reasoning: streamReasoning, Estimate: streamEst, Version: len(streamContent) + len(streamReasoning)}
		lines = append(lines, c.renderCached(&b, t, acts, hover, o.Frame)...)
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
	// Pad to the viewport height. At the bottom of the transcript the newest
	// content must sit just above the composer, so short conversations pad at
	// the top; while scrolled back the remaining content belongs below.
	if pad := height - len(lines); pad > 0 {
		blank := make([]renderLine, pad)
		if c.scroll == 0 {
			lines = append(blank, lines...)
		} else {
			lines = append(lines, blank...)
		}
	}
	return lines
}

// Tool results are indented under their call; expanded argument and output
// bodies use a deeper gutter so the tree stays readable.
const expandedGut = "      "

func resultGutter(g Glyphs) string { return "   " + g.Result + "  " }

func (c *Conversation) renderBlock(b convBlock, t Theme, acts map[string]domain.ToolActivity, hover string, frame int) []renderLine {
	var out []renderLine
	width := c.width
	if width < 20 {
		width = 20
	}
	g := t.Glyphs

	if b.Role == domain.RoleUser {
		out = append(out, c.renderUserBlock(b, t, width)...)
		out = append(out, renderLine{})
		return out
	}

	// 1. Assistant turn. Reasoning is indented under the turn; the marker leads
	// the actual answer so the two never stack up on one line.
	marker := lipgloss.NewStyle().Foreground(t.Gutter).Render(g.Assistant)
	if b.ID == "__stream__" {
		marker = lipgloss.NewStyle().Foreground(t.Primary).Bold(true).Render(g.Assistant)
	}
	contentW := max(16, width-2)
	emitted := false

	b.Reasoning = terminalutil.SanitizeText(b.Reasoning)
	if strings.TrimSpace(b.Reasoning) != "" {
		out = append(out, c.renderReasoning(b, t, width, hover)...)
	}

	// 2. Answer content.
	if strings.TrimSpace(b.Content) != "" {
		var contentLines []string
		if b.ID == "__stream__" {
			// The streaming path wraps incrementally; see streamWrap.
			contentLines = c.stream.wrap(b.Content, contentW, b.StreamID)
		} else {
			rendered, err := c.cache.Render(b.ID, b.Version, contentW, c.mdKey, b.Content, c.mdStyle)
			if err != nil {
				rendered = md.PlainFallback(b.Content, contentW)
			}
			contentLines = strings.Split(rendered, "\n")
		}
		for _, l := range contentLines {
			if !emitted {
				out = append(out, renderLine{Text: marker + " " + l})
				emitted = true
				continue
			}
			out = append(out, renderLine{Text: "  " + l})
		}
	}

	if !emitted && len(out) == 0 {
		// An empty turn still needs a marker, but a turn that has shown
		// reasoning must not trail a bare marker with nothing after it.
		out = append(out, renderLine{Text: marker})
	}

	// 3. Tool calls render as a nested tree under the turn.
	for _, tc := range b.ToolCalls {
		if strings.TrimSpace(tc.Function.Name) == "" {
			continue
		}
		out = append(out, c.renderToolCall(tc, acts[tc.ID], t, width, hover, frame)...)
	}

	out = append(out, renderLine{})
	return out
}

func (c *Conversation) renderUserBlock(b convBlock, t Theme, width int) []renderLine {
	g := t.Glyphs
	marker := lipgloss.NewStyle().Foreground(t.User).Bold(true).Render(g.User)
	style := lipgloss.NewStyle().Foreground(t.Text)
	var out []renderLine
	first := true
	for _, l := range wrapPlain(b.Content, max(8, width-2)) {
		if first {
			out = append(out, renderLine{Text: marker + " " + style.Render(l)})
			first = false
			continue
		}
		out = append(out, renderLine{Text: "  " + style.Render(l)})
	}
	if first {
		out = append(out, renderLine{Text: marker})
	}
	return out
}

// reasoningExpanded reports whether a block's reasoning should be shown in
// full. The live streaming block is always expanded: collapsing it to a
// one-line summary while it is still being written leaves the screen looking
// frozen, because neither the text nor the line count visibly changes.
// Collapsing is only right for history, where the answer is what matters.
func (c *Conversation) reasoningExpanded(id string) bool {
	return id == "__stream__" || c.expandedThinking[id]
}

// renderReasoning returns the reasoning subtree. Collapsed it is a single dim
// line so long thinking does not push the answer off screen; expanded it shows
// the wrapped text in the reasoning color.
func (c *Conversation) renderReasoning(b convBlock, t Theme, width int, hover string) []renderLine {
	g := t.Glyphs
	live := b.ID == "__stream__"
	style := lipgloss.NewStyle().Foreground(t.Reasoning)
	if hover == "thinking:"+b.ID {
		style = style.Bold(true)
	}
	if !c.reasoningExpanded(b.ID) {
		lineCount := len(wrapPlain(b.Reasoning, max(8, width-4)))
		label := fmt.Sprintf("Thinking… (%d lines %s ctrl+t)", lineCount, g.Sep)
		return []renderLine{{
			Text:   "  " + style.Render(g.Thinking+" "+label),
			Action: ActionThinking,
			Value:  "thinking:" + b.ID,
		}}
	}
	header := "Thinking (ctrl+t to collapse)"
	if live {
		// No toggle hint while streaming: this state is transient, not a choice.
		header = "Thinking…"
	}
	out := []renderLine{{
		Text:   "  " + style.Render(g.Thinking+" "+header),
		Action: ActionThinking,
		Value:  "thinking:" + b.ID,
	}}
	for _, l := range wrapPlain(b.Reasoning, max(8, width-4)) {
		out = append(out, renderLine{Text: "    " + style.Render(l)})
	}
	return out
}

func (c *Conversation) renderToolCall(tc domain.ToolCall, a domain.ToolActivity, t Theme, width int, hover string, frame int) []renderLine {
	g := t.Glyphs
	icon, fg := g.Pending, t.Muted
	statusLabel := "queued"
	switch a.State {
	case "running":
		icon, fg = g.spinner(frame), t.Warning
		statusLabel = "running"
	case "success":
		icon, fg = g.Success, t.Success
		statusLabel = "done"
	case "failed":
		icon, fg = g.Failure, t.Error
		statusLabel = "failed"
	}

	name := terminalutil.SanitizeText(tc.Function.Name)
	head := c.renderToolHead(tc, name, t, hover, g)
	if a.State == "failed" {
		// Failed calls read as errors first, tool second.
		head = lipgloss.NewStyle().Foreground(t.Error).Render(ansi.Strip(head))
		if hover == tc.ID {
			head = lipgloss.NewStyle().Foreground(t.Error).Bold(true).Render(ansi.Strip(head))
		}
	}

	// Status meta stays dim and fixed-position so the spinner can animate
	// without shifting the action text. The marker already communicates motion;
	// only elapsed time or an exceptional state needs a text label.
	metaText := toolStatusMeta(a, statusLabel)
	meta := ""
	if metaText != "" {
		meta = lipgloss.NewStyle().Foreground(t.Muted).Render(" · " + metaText)
	}

	marker := lipgloss.NewStyle().Foreground(fg).Render(icon)
	line := " " + marker + " " + head + meta
	if c.expandedTools[tc.ID] {
		line = " " + marker + " " + head + meta + lipgloss.NewStyle().Foreground(t.Muted).Render("  · details")
	}

	out := []renderLine{{Text: line, Action: ActionTool, Value: tc.ID}}

	if c.expandedTools[tc.ID] {
		if tc.Function.Arguments != "" {
			for _, l := range wrapPlain(tc.Function.Arguments, max(8, width-lipgloss.Width(expandedGut)-2)) {
				out = append(out, renderLine{Text: expandedGut + lipgloss.NewStyle().Foreground(t.Muted).Render(l)})
			}
		}
		out = append(out, c.renderExpandedResult(a, t, width)...)
		return out
	}

	// Collapsed: one dim result line, or the first line of the error so a
	// failure is visible without expanding.
	resultStyle := lipgloss.NewStyle().Foreground(t.Muted)
	result := toolResultSummary(a)
	if a.State == "failed" {
		resultStyle = lipgloss.NewStyle().Foreground(t.Error)
		if a.Error != "" {
			result = firstLine(a.Error)
		}
	}
	if result != "" {
		if d := toolDuration(a); d != "" && a.State != "running" {
			result += " · " + d
		}
		gutter := resultGutter(g)
		out = append(out, renderLine{Text: gutter + resultStyle.Render(truncWidth(result, max(6, width-lipgloss.Width(gutter))))})
	} else if a.State == "running" {
		// A running call with no output yet still gets a result row so the
		// card keeps a stable two-line height instead of popping when the
		// first byte arrives.
		gutter := resultGutter(g)
		hint := lipgloss.NewStyle().Foreground(t.Muted).Render("…")
		out = append(out, renderLine{Text: gutter + hint})
	}
	return out
}

// renderToolHead turns an internal function call into a compact, human-readable
// action. The raw function name remains available in the expanded arguments, but
// the collapsed transcript should read like a conversation rather than a trace.
func (c *Conversation) renderToolHead(tc domain.ToolCall, name string, t Theme, hover string, g Glyphs) string {
	nameStyle := lipgloss.NewStyle().Foreground(t.Tool).Bold(true)
	argStyle := lipgloss.NewStyle().Foreground(t.Muted)
	if hover == tc.ID {
		nameStyle = nameStyle.Underline(true)
	}

	label := toolDisplayName(name)
	if name == "delegate" {
		if agent, task := parseDelegateArgs(tc.Function.Arguments); agent != "" {
			head := nameStyle.Render(label) + argStyle.Render(" "+g.Bullet+" "+agent)
			if task != "" {
				head += argStyle.Render(" · " + truncWidth(singleLine(terminalutil.SanitizeText(task)), 48))
			}
			return head
		}
	}
	summary := toolSummary(name, terminalutil.SanitizeText(tc.Function.Arguments))
	if summary == "" {
		return nameStyle.Render(label)
	}
	return nameStyle.Render(label) + argStyle.Render(" · "+summary)
}

// toolDisplayName is intentionally phrased as an action. It gives every tool a
// stable visual vocabulary while allowing newly added tools to fall back to a
// readable title derived from their function name.
func toolDisplayName(name string) string {
	labels := map[string]string{
		"bash":        "Run command",
		"read_file":   "Read file",
		"write_file":  "Write file",
		"edit_file":   "Edit file",
		"list_dir":    "List folder",
		"grep":        "Search files",
		"glob":        "Find files",
		"fetch_url":   "Open webpage",
		"web_search":  "Search web",
		"use_skill":   "Load skill",
		"calc":        "Calculate",
		"delegate":    "Delegate task",
		"todo_update": "Update task",
	}
	if label, ok := labels[name]; ok {
		return label
	}
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' })
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	if len(parts) == 0 {
		return "Tool"
	}
	return strings.Join(parts, " ")
}

func parseDelegateArgs(args string) (agent, task string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return "", ""
	}
	if v, _ := raw["agent"].(string); v != "" {
		agent = v
	}
	if v, _ := raw["task"].(string); v != "" {
		task = v
	}
	return agent, task
}

// toolStatusMeta is the dim suffix on the header row: live elapsed while
// running, total duration once finished, plain state when timing is unknown.
func toolStatusMeta(a domain.ToolActivity, label string) string {
	switch a.State {
	case "running":
		if d := toolElapsed(a); d != "" {
			return "running " + d
		}
		return "running"
	case "success":
		if d := toolDuration(a); d != "" {
			return d
		}
		return label
	case "failed":
		if d := toolDuration(a); d != "" {
			return "failed · " + d
		}
		return "failed"
	default:
		return "queued"
	}
}

func toolElapsed(a domain.ToolActivity) string {
	if a.StartedAt.IsZero() {
		return ""
	}
	d := time.Since(a.StartedAt)
	if d < time.Second {
		return ""
	}
	return formatToolDuration(d)
}

func toolDuration(a domain.ToolActivity) string {
	if a.StartedAt.IsZero() || a.EndedAt.IsZero() {
		return ""
	}
	d := a.EndedAt.Sub(a.StartedAt)
	if d < 0 {
		return ""
	}
	return formatToolDuration(d)
}

func formatToolDuration(d time.Duration) string {
	if d < time.Millisecond*50 {
		return "0ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", int(d.Milliseconds()))
	}
	s := d.Seconds()
	if s < 10 {
		return fmt.Sprintf("%.1fs", s)
	}
	if s < 60 {
		return fmt.Sprintf("%ds", int(s))
	}
	m := int(s) / 60
	if r := int(s) % 60; r > 0 {
		return fmt.Sprintf("%dm%ds", m, r)
	}
	return fmt.Sprintf("%dm", m)
}

const maxExpandedOutputLines = 12

// renderExpandedResult shows arguments output capped to a fixed preview so one
// large tool result cannot push the whole transcript or change the card height
// on every streamed chunk.
func (c *Conversation) renderExpandedResult(a domain.ToolActivity, t Theme, width int) []renderLine {
	var out []renderLine
	bodyW := max(8, width-lipgloss.Width(expandedGut)-2)
	if a.Output != "" {
		lines := wrapPlain(terminalutil.SanitizeText(a.Output), bodyW)
		if len(lines) > maxExpandedOutputLines {
			kept := lines[:maxExpandedOutputLines]
			for _, l := range kept {
				out = append(out, renderLine{Text: expandedGut + lipgloss.NewStyle().Foreground(t.Text).Render(l)})
			}
			more := lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("… +%d more", len(lines)-maxExpandedOutputLines))
			out = append(out, renderLine{Text: expandedGut + more})
		} else {
			for _, l := range lines {
				out = append(out, renderLine{Text: expandedGut + lipgloss.NewStyle().Foreground(t.Text).Render(l)})
			}
		}
	}
	if a.Error != "" {
		for _, l := range wrapPlain(terminalutil.SanitizeText(a.Error), bodyW) {
			out = append(out, renderLine{Text: expandedGut + lipgloss.NewStyle().Foreground(t.Error).Render(l)})
		}
	}
	return out
}

// toolResultSummary condenses a completed call into one line. Byte counts and
// line counts are cheaper for a reader than the first line of compiler output.
func toolResultSummary(a domain.ToolActivity) string {
	if a.State == "running" {
		return ""
	}
	if a.Output == "" && a.Error == "" {
		return ""
	}
	text := a.Output
	if text == "" {
		text = a.Error
	}
	lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
	if lines <= 1 {
		return truncWidth(firstLine(text), 100)
	}
	return fmt.Sprintf("%d lines", lines)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimRight(s[:i], "\r")
	}
	return strings.TrimRight(s, "\r")
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
	n++ // the collapsed "Thinking…" line, always present
	if expanded {
		n += len(wrapPlain(reasoning, max(8, width-4)))
	}
	return n
}

// estimateBlock approximates the height of one turn. Block rendering corrects
// the estimate to the real height for any block it materializes, so this only
// needs to be close enough to keep the off-screen scroll position stable.
func estimateBlock(content string, width, tools int) int {
	if width < 20 {
		width = 20
	}
	lines := 1 + tools*2 // trailing blank + one line per tool call and result
	contentW := max(16, width-2)
	for _, l := range strings.Split(content, "\n") {
		lines += max(1, int(math.Ceil(float64(displayWidth(l))/float64(contentW))))
	}
	return lines
}

// wrapPlain hard-wraps plain text to a display width. It must measure display
// columns rather than runes: CJK and emoji occupy two columns each, so a
// rune-counting wrap overflows the terminal for exactly the messages this app
// sees most often.
func wrapPlain(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Hardwrap(para, width, false), "\n")...)
	}
	return out
}

// displayWidth is the rendered column count, ignoring escape sequences and
// accounting for double-width characters.
func displayWidth(s string) int { return ansi.StringWidth(s) }

// toolArgKeys lists, per tool, the arguments worth showing in a collapsed
// call line, in display order. Anything not listed falls back to argFallbackKeys
// so a newly added tool still renders a useful one-line summary.
var toolArgKeys = map[string][]string{
	"bash":        {"cmd"},
	"read_file":   {"path", "offset"},
	"write_file":  {"path"},
	"edit_file":   {"path"},
	"list_dir":    {"path"},
	"grep":        {"pattern", "include"},
	"glob":        {"pattern"},
	"fetch_url":   {"url"},
	"web_search":  {"query"},
	"use_skill":   {"skill"},
	"calc":        {"expr"},
	"delegate":    {"agent", "task"},
	"todo_update": {"subject", "index"},
}

var argFallbackKeys = []string{"path", "cmd", "query", "url", "pattern", "expr", "skill", "subject", "task", "name"}

// toolSummary renders the parenthesized part of a collapsed tool-call line.
// Showing the salient argument beats dumping raw JSON: `Read(src/app.go)` is
// readable at a glance, `Read({"path":"src/app.go","offset":1})` is not.
func toolSummary(name, args string) string {
	args = strings.TrimSpace(args)
	if args == "" || args == "{}" {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return truncWidth(singleLine(args), 70)
	}
	keys := toolArgKeys[name]
	for _, k := range append(append([]string(nil), keys...), argFallbackKeys...) {
		v, ok := raw[k]
		if !ok {
			continue
		}
		s := formatArgValue(v)
		if s == "" {
			continue
		}
		return truncWidth(singleLine(s), 70)
	}
	return ""
}

func formatArgValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			if s := formatArgValue(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}
