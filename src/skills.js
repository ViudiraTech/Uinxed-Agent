/*
 * Copyright 2026 Uinxed Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/* ============ Skills 系统(遵循 Agent Skills 开放标准,兼容 opencode / Claude Code / Codex) ============
 * 格式标准(agentskills.io,Anthropic 发布,Claude Code / opencode / Codex CLI / Cursor 等通用):
 *   每个技能 = 一个与技能名同名的目录,内含 SKILL.md:
 *     ---
 *     name: skill-name         # 必填,小写 kebab-case,≤64 字符,须与目录名一致
 *     description: 一句话        # 必填,≤1024 字符,描述做什么/何时用(注入 system prompt)
 *     license: MIT             # 可选
 *     compatibility: ...       # 可选
 *     metadata: {...}          # 可选,任意字符串键值
 *     ---
 *     (markdown 正文,激活时完整提供给模型)
 *   可选附带目录:scripts/、references/、assets/(渐进式披露,按需加载)。
 *
 * 发现路径(项目级优先,同名覆盖;最后并入内置技能):
 *   项目: .ux-agent/skills/ 、.opencode/skills/ 、.claude/skills/ 、.agents/skills/
 *   全局: ~/.config/ux-agent/skills/ 、~/.config/opencode/skills/ 、~/.claude/skills/ 、~/.agents/skills/
 */

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const SKILL_FILE = "SKILL.md";

/* 技能发现顺序:项目级四目录(自身 .ux-agent 优先)-> 全局级四目录 */
const PROJECT_ROOTS = [
  path.join(".ux-agent", "skills"),
  path.join(".opencode", "skills"),
  path.join(".claude", "skills"),
  path.join(".agents", "skills"),
];
const GLOBAL_ROOTS = [
  path.join(".config", "ux-agent", "skills"),
  path.join(".config", "opencode", "skills"),
  path.join(".claude", "skills"),
  path.join(".agents", "skills"),
];

/* ===== 内置技能(硬编码,免目录/文件依赖)。同名用户技能优先于内置。 ===== */
const BUILTIN_SKILLS = [
  {
    name: "skill-creator",
    description: "当用户要求创建/新增一个技能(skill)时使用。包含技能目录结构、SKILL.md 格式规范与示例模板。",
    dir: "(内置)",
    body: `# Skill Creator

用户要求"创建技能"时，按本指南完成。技能(Skill)遵循 Agent Skills 开放标准(agentskills.io)，格式在 Claude Code、opencode、Codex、Cursor 等 20+ 工具通用。

## 创建位置(统一格式,一处编写到处可用)

- 项目级(随项目走,默认):
  - \`<项目>/.ux-agent/skills/<name>/SKILL.md\`  (ux-agent 专有,优先)
  - \`<项目>/.opencode/skills/<name>/SKILL.md\`
  - \`<项目>/.claude/skills/<name>/SKILL.md\`
  - \`<项目>/.agents/skills/<name>/SKILL.md\`
- 全局(所有项目可用):
  - \`~/.config/ux-agent/skills/<name>/SKILL.md\`
  - \`~/.config/opencode/skills/<name>/SKILL.md\`
  - \`~/.claude/skills/<name>/SKILL.md\`
  - \`~/.agents/skills/<name>/SKILL.md\`

同名技能:项目级覆盖全局,再覆盖内置(.ux-agent > .opencode > .claude > .agents)。创建完成无需重启,/skills 立即可见。

## SKILL.md 格式

\`\`\`markdown
---
name: skill-name          # 必填,kebab-case 小写,≤64 字符,须与目录名一致
description: 一两句话        # 必填,≤1024 字符,说明做什么与何时用(会注入 system prompt)
license: MIT              # 可选,SPDX 标识
# compatibility: ...        # 可选:对运行环境的要求
# metadata:                # 可选,任意键值
#   author: example
---
(技能正文,markdown,加载后完整提供给模型。可用 \`references/\`、\`assets/\`、\`scripts/\` 目录被引用的附加文件)
\`\`\`

## 规范

1. name:小写字母/数字/连字符,格式 \`^[a-z0-9]+(-[a-z0-9]+)*$\`,必须与所在目录名一致。
2. description 以"当用户……时使用"或动词开头,写清触发词/触发场景,≤1024 字符,让主 agent 能准确判断何时加载。
3. 正文直接给可执行指令:步骤、命令(相对路径)、模板、示例,避免空话;推荐结构:目的 / 何时触发 / 输入 / 步骤 / 输出 / 常见坑。
4. 正文建议控制在 500 行以内;长参考内容放到 \`references/*.md\`、\`scripts/\`、\`assets/\` 并在正文里注明何时加载。
5. 正文不超过 20000 字符(超长会被截断)。
6. 创建后验证:终端输入 /skills <名称> 能加载;必要时让用户跑一次真实任务确认效果。

## 示例模板(新技能)

\`\`\`markdown
---
name: <skill-name>
description: 当用户<触发场景>时使用。简要说明做什么。
---

# <技能名>

## 目的
(一句话:何时使用)

## 步骤
1. ...
2. ...

## 示例
(可选:模板/命令示例)

## 注意
(常见坑)
\`\`\`

## 工作流程

1. 先与用户确认技能名与用途(一句话)。
2. 决定放项目级还是全局(便携优先 .ux-agent 或 .claude/.agents 目录)。
3. 用 write_file 创建 SKILL.md(含 frontmatter 与正文)。
4. 用 list_dir/read_file 复核内容,必要时参考既有技能。
5. 让用户在 TUI 输入 /skills 验证加载,或在终端 /skills <名称> 确认。`,
  },
];

/* 解析 frontmatter(name/description/…,YAML 标量) + 正文 */
export function parseSkillFile(text) {
  const s = String(text ?? "");
  let name = "";
  let description = "";
  let body = s;
  const m = s.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (m) {
    body = m[2];
    for (const line of m[1].split("\n")) {
      const kv = line.match(/^\s*([a-zA-Z-]+):\s*(.*?)\s*$/);
      if (kv && kv[1] !== "metadata") {
        const val = kv[2].trim().replace(/^['"]|['"]$/g, "");
        if (kv[1] === "name") name = val;
        if (kv[1] === "description") description = val;
      }
    }
  }
  return { name, description, body: body.trim() };
}

/* 候选技能目录(项目在前,全局在后;ux-agent 自身目录优先) */
function skillRoots(cwd) {
  const dirs = [];
  for (const rel of PROJECT_ROOTS) {
    if (cwd) dirs.push(path.join(cwd, rel));
  }
  for (const rel of GLOBAL_ROOTS) {
    dirs.push(path.join(os.homedir(), rel));
  }
  return dirs;
}

/* 扫描所有可用技能:先项目后全局,同名首个生效(项目 → 全局);最后并入内置技能 */
export function listSkills(cwd) {
  const map = {};
  for (const root of skillRoots(cwd)) {
    let entries;
    try { entries = fs.readdirSync(root, { withFileTypes: true }); } catch { continue; }
    for (const e of entries) {
      let st;
      try { st = fs.statSync(path.join(root, e.name)); } catch { continue; }
      if (!st.isDirectory()) continue;
      const dirName = e.name;
      const file = path.join(root, dirName, SKILL_FILE);
      let raw;
      try { raw = fs.readFileSync(file, "utf8"); } catch { continue; }
      const skill = parseSkillFile(raw);
      const sn = skill.name || dirName;
      if (!map[sn]) map[sn] = { ...skill, name: sn, dir: path.join(root, dirName) };
    }
  }
  for (const b of BUILTIN_SKILLS) {
    if (!map[b.name]) map[b.name] = { ...b };
  }
  return Object.values(map);
}

/* 获取单个技能内容(找不到返回 null) */
export function getSkill(name, cwd) {
  const skills = listSkills(cwd);
  return skills.find((s) => s.name === name) || null;
}

/* 生成 System prompt 中的技能描述段 */
export function skillPromptBlock(cwd) {
  const skills = listSkills(cwd);
  if (!skills.length) return "";
  const lines = skills.map((s) => `- ${s.name}: ${s.description || "(无描述)"}`);
  return "\n\n[可用技能]\n你拥有以下技能(遵循 Agent Skills 开放标准),任务匹配时用 use_skill 工具加载完整指令后再执行:\n" + lines.join("\n");
}