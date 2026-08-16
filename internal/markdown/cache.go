package markdown

import (
	"fmt"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

type key struct {
	ID      string
	Version int
	Width   int
	Theme   string
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
func (c *Cache) Render(id string, version, width int, theme, content string) (string, error) {
	if width < 20 {
		width = 20
	}
	content = terminalutil.SanitizeText(content)
	k := key{id, version, width, theme}
	c.mu.RLock()
	v, ok := c.items[k]
	c.mu.RUnlock()
	if ok {
		return v, nil
	}
	style := "dark"
	if theme == "light" {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(glamour.WithStylePath(style), glamour.WithWordWrap(width))
	if err != nil {
		return "", err
	}
	out, err := r.Render(content)
	if err != nil {
		return "", err
	}
	out = strings.TrimRight(out, "\n")
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
func PlainFallback(content string, width int) string {
	content = terminalutil.SanitizeText(content)
	if width <= 0 {
		return content
	}
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		r := []rune(line)
		for len(r) > width {
			fmt.Fprintln(&b, string(r[:width]))
			r = r[width:]
		}
		b.WriteString(string(r))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
