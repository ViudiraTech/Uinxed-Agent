package markdown

import (
	"strings"
	"sync"

	"charm.land/glamour/v2"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
	"github.com/charmbracelet/x/ansi"
)

type key struct {
	ID      string
	Version int
	Width   int
	Style   string
}
type Cache struct {
	mu    sync.RWMutex
	items map[key]string
	order []key
	max   int
}

func NewCache(max int) *Cache {
	if max < 64 {
		max = 64
	}
	return &Cache{items: map[key]string{}, max: max}
}
func (c *Cache) Render(id string, version, width int, styleKey, content string, style Style) (string, error) {
	if width < 20 {
		width = 20
	}
	k := key{id, version, width, styleKey}
	c.mu.RLock()
	v, ok := c.items[k]
	c.mu.RUnlock()
	if ok {
		return v, nil
	}
	out, err := RenderOnce(width, content, style)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[k]; !ok {
		c.order = append(c.order, k)
	}
	c.items[k] = out
	for len(c.order) > c.max {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.items, old)
	}
	return out, nil
}

// RenderOnce glams content to a styled string without memoizing it.
//
// The streaming path renders the still-open tail of a message every frame, and
// those partial documents never recur. Routing them through Cache would let a
// single long answer evict every finished paragraph from its LRU while buying
// nothing in return, so the tail renders directly and only the closed prefixes
// (which do repeat) go through the cache.
func RenderOnce(width int, content string, style Style) (string, error) {
	if width < 20 {
		width = 20
	}
	content = terminalutil.SanitizeText(content)
	r, err := glamour.NewTermRenderer(glamour.WithStyles(ansiConfig(style)), glamour.WithWordWrap(width))
	if err != nil {
		return "", err
	}
	out, err := r.Render(content)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

func (c *Cache) InvalidateWidth(width int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.items {
		if k.Width == width {
			delete(c.items, k)
		}
	}
}
func (c *Cache) Clear() { c.mu.Lock(); c.items = map[key]string{}; c.order = nil; c.mu.Unlock() }

// PlainFallback hard-wraps Markdown-free text for the streaming path, where
// rendering a partial document through glamour is too slow. Wrapping measures
// display columns so CJK content does not overrun the pane.
func PlainFallback(content string, width int) string {
	content = terminalutil.SanitizeText(content)
	if width <= 0 {
		return content
	}
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(ansi.Hardwrap(line, width, false))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
