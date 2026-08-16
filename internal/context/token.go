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

func FitMessages(msgs []domain.Message, maxTokens int) []domain.Message {
	if maxTokens <= 0 {
		return nil
	}
	var out []domain.Message
	used := 0
	start := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		t := EstimateMessages(msgs[i : i+1])
		if len(out) > 0 && used+t > maxTokens {
			break
		}
		used += t
		start = i
	}
	if start < len(msgs) {
		out = append(out, msgs[start:]...)
	}
	if len(out) > 0 && out[0].Role == domain.RoleTool {
		id := out[0].ToolCallID
		for i := start - 1; i >= 0; i-- {
			if msgs[i].Role != domain.RoleAssistant {
				continue
			}
			for _, tc := range msgs[i].ToolCalls {
				if tc.ID == id {
					out = append([]domain.Message{msgs[i]}, out...)
					return out
				}
			}
		}
	}
	return out
}
