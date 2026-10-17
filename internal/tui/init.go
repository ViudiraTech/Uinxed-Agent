package tui

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/ViudiraTech/Uinxed-Agent/internal/approval"
)

// initPrompt is the turn /init starts. It is written as a task rather than as a
// template dump, because the value of /init is that the agent reads the repository
// it is standing in: a generic skeleton is exactly what the project instructions
// are supposed to replace.
const initPrompt = `请为当前仓库编写项目级指令文件 AGENTS.md，放在仓库根目录，供后续所有会话自动加载。

步骤：
1. 实际阅读仓库：README、Makefile / 构建脚本、go.mod 或 package.json、CI 配置、docs/ 与主要源码目录。只写你验证过的事实。
2. 如果已存在 CLAUDE.md、AGENTS.md 或 .ux-agent/ 下的指令文件，先读它们：保留仍然有效的约定，修正过时内容，不要重复或自相矛盾。
3. 内容结构固定为：项目一句话定位；构建 / 测试 / 格式 / 检查命令；架构分层与目录职责；不可打破的硬约束（边界规则、安全不变量）；代码约定；常见坑。
4. 每条命令都要有出处（Makefile target、CI step 或脚本路径）；没有验证过的命令不要写。
5. 目标 40–100 行。不要徽章、不要营销语、不要逐文件罗列。
6. 写入 AGENTS.md 后，用 3 行以内说明写了什么、哪些内容需要人工确认。`

// runInit backs /init. It dispatches a turn instead of writing the file itself:
// the commands and invariants worth recording can only come from reading the
// repository, which is what the agent is for.
func (m *Model) runInit() tea.Cmd {
	if approval.Normalize(m.currentMode()) == approval.ModePlan {
		m.showToast("plan mode is read-only; leave it before /init")
		return nil
	}
	if m.busy {
		m.showToast("a turn is already running; esc or ctrl+c to cancel")
		return nil
	}
	target := filepath.Join(m.session.CWD, "AGENTS.md")
	if _, err := os.Stat(target); err == nil {
		m.showToast("AGENTS.md exists — asking the agent to refresh it")
	} else {
		m.showToast("writing AGENTS.md from this repository")
	}
	return m.sendTurn(initPrompt)
}
