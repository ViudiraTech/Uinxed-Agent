package agent

import (
	"strings"
	"testing"
	"time"
)

func TestSystemPromptInjectsAuthoritativeRuntimeDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, time.August, 17, 14, 36, 12, 0, loc)

	prompt := systemPromptAt(Get("build"), "test-model", "", "", now)

	for _, want := range []string{
		"Current date: 2026-08-17",
		"Current year: 2026",
		"Current local time: 14:36:12",
		"Timezone: CST (UTC+08:00)",
		"不得因为模型训练时间、知识截止时间或先验记忆而声称其他年份",
		"无需调用任何时间工具",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q\n%s", want, prompt)
		}
	}
}
