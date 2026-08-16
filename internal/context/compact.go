package contextmgr

import (
	"strings"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

const CompactionInstructions = `请把上面的对话压缩成一份结构化摘要,用于替换全部历史并继续任务。

严格遵循:
1. 只输出摘要本身,不要任何解释、前言或后记。
2. 结构建议(酌情合并):
   - 【原始意图】用户最初的要求与整体目标
   - 【关键技术点】涉及的技术/库/接口及已得出的结论
   - 【涉及文件与代码】关键文件路径 + 重要片段或改动点
   - 【已完成的步骤】列出已经做过并确认的事
   - 【遇到的错误与修复】错误信息与解决方案
   - 【未完成/待办】明确列出还没有做完的事
   - 【下一步】接下来应该做什么
3. 保留所有用户提出的具体诉求原文大意;数字、路径、命令要准确,不要丢。
4. 尽量完整,摘要本身可以长。
5. 用中文。`

func CompactionConversation(messages []domain.Message) []domain.Message {
	var b strings.Builder
	b.WriteString(CompactionInstructions)
	b.WriteString("\n\n—— 历史对话开始 ——\n\n")
	for _, m := range messages {
		b.WriteByte('[')
		b.WriteString(string(m.Role))
		b.WriteString("]")
		if m.Content == "" && len(m.ToolCalls) > 0 {
			b.WriteString("(工具调用)")
		} else {
			b.WriteString(m.Content)
		}
		b.WriteString("\n\n")
	}
	b.WriteString("—— 历史对话结束 ——")
	return []domain.Message{
		{Role: domain.RoleSystem, Content: "你是会话压缩器。下一条是待压缩的完整对话历史,请按给定要求输出结构化摘要。"},
		{Role: domain.RoleUser, Content: b.String()},
	}
}

func ReplaceWithSummary(summary string) []domain.Message {
	return []domain.Message{
		{Role: domain.RoleSystem, Content: "以下是先前对话的自动压缩摘要。继续任务时把它视为可靠的会话历史。"},
		{Role: domain.RoleAssistant, Content: summary},
	}
}
