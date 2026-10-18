package contextmgr

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

const (
	DefaultContextWindow = 131072
	CompactRatio         = 0.62
	RequestHistoryRatio  = 0.72
	WarnRatio            = 0.50
)

var contextWindows = map[string]int{
	"deepseek-v4-pro":   1_000_000,
	"deepseek-v4-flash": 1_000_000,
	"deepseek-v4.5":     1_000_000,
	"deepseek-reasoner": 1_000_000,
	"glm-4-flash":       131072,
	"glm-4-flash-proxy": 131072,
	"glm-4.6":           131072,
	"glm-4.5":           131072,
	"glm-4":             131072,
}

var suffixWindow = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([mk])\b`)

func Window(model string) int {
	key := strings.ToLower(strings.TrimSpace(model))
	if v := contextWindows[key]; v > 0 {
		return v
	}
	m := suffixWindow.FindStringSubmatch(key)
	if len(m) == 3 {
		var n float64
		for _, r := range m[1] {
			if r == '.' {
				continue
			}
			n = n*10 + float64(r-'0')
		}
		if strings.Contains(m[1], ".") {
			parts := strings.SplitN(m[1], ".", 2)
			var whole, frac float64
			for _, r := range parts[0] {
				whole = whole*10 + float64(r-'0')
			}
			div := 1.0
			for _, r := range parts[1] {
				frac = frac*10 + float64(r-'0')
				div *= 10
			}
			n = whole + frac/div
		}
		if m[2] == "m" || m[2] == "M" {
			return int(math.Round(n * 1_000_000))
		}
		return int(math.Round(n * 1000))
	}
	return DefaultContextWindow
}

func CompactThreshold(model string) int { return int(float64(Window(model)) * CompactRatio) }
func HistoryBudget(model string) int    { return int(float64(Window(model)) * RequestHistoryRatio) }
func WarnTokens(model string) int       { return int(float64(Window(model)) * WarnRatio) }

func EstimateText(s string) int {
	if s == "" {
		return 0
	}
	var cjk, other int
	for _, r := range s {
		if isCJKish(r) {
			cjk++
		} else {
			other++
		}
	}
	v := math.Round(float64(cjk)*1.1+float64(other)*0.32) + 2
	if v < 0 {
		return 0
	}
	return int(v)
}

func isCJKish(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
		(r >= 0xFF00 && r <= 0xFFEF) || (r >= 0x3000 && r <= 0x303F)
}

func EstimateMessages(msgs []domain.Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateText(m.Content) + 4
		for _, tc := range m.ToolCalls {
			b, _ := json.Marshal(tc.Function)
			total += EstimateText(string(b)) + 8
		}
	}
	return total
}

// FitMessages returns the newest suffix of msgs that fits the budget. The newest
// message is always kept even when it alone exceeds maxTokens: sending one long
// message is the only way to make progress, while sending nothing is not.
func FitMessages(msgs []domain.Message, maxTokens int) []domain.Message {
	if maxTokens <= 0 {
		return nil
	}
	used := 0
	start := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		t := EstimateMessages(msgs[i : i+1])
		// start < len(msgs) means at least one message is already kept.
		if start < len(msgs) && used+t > maxTokens {
			break
		}
		used += t
		start = i
	}
	// A suffix must not begin with a tool result: the provider pairs every tool
	// message with the assistant call that requested it. Move the cut back to that
	// assistant so the round stays valid; when it cannot be found, drop the orphan
	// rather than send an unpaired result.
	if start < len(msgs) && msgs[start].Role == domain.RoleTool {
		if at := assistantForTool(msgs, start); at >= 0 {
			start = at
		} else {
			for start < len(msgs) && msgs[start].Role == domain.RoleTool {
				start++
			}
		}
	}
	if start >= len(msgs) {
		return nil
	}
	return append([]domain.Message(nil), msgs[start:]...)
}

// assistantForTool finds the assistant message that owns the tool call at index at.
func assistantForTool(msgs []domain.Message, at int) int {
	id := msgs[at].ToolCallID
	for i := at - 1; i >= 0; i-- {
		if msgs[i].Role != domain.RoleAssistant {
			continue
		}
		for _, tc := range msgs[i].ToolCalls {
			if tc.ID == id {
				return i
			}
		}
	}
	return -1
}
