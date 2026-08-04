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

/* ============ Skills 系统(仿 opencode):项目级技能包 ============
 * 每个技能 = 一个目录,内含 SKILL.md:
 *   .ux-agent/skills/<name>/SKILL.md   (项目级)
 *   ~/.config/ux-agent/skills/<name>/SKILL.md (全局)
 *
 * SKILL.md 格式:
 *   ---
 *   name: skill-name
 *   description: 一句话说明该技能适用场景(会注入 system prompt)
 *   ---
 *   (markdown 技能正文,加载后完整提供给模型)
 */

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const SKILL_FILE = "SKILL.md";

/* ===== 内置技能(硬编码,免目录/文件依赖)。项目/全局同名技能优先于内置。 ===== */
const BUILTIN_SKILLS = [
  {
    name: "skill-creator",
    description: "当用户要求创建/新增一个技能(skill)时使用。包含技能目录结构、SKILL.md 格式规范与示例模板。",
    dir: "(内置)",
    body: `# Skill Creator

用户要求"创建技能"时，按本指南完成。技能让 agent 在特定任务上具备可复用的专项指令。

## 创建位置

- 项目级（随项目走）：\`<项目>/.ux-agent/skills/<name>/SKILL.md\`
- 全局（所有项目可用）：\`~/.config/ux-agent/skills/<name>/SKILL.md\`

同名技能全局被项目级覆盖。创建完成后无需重启，/skills 立即可见。

## SKILL.md 格式

\`\`\`markdown
---
name: skill-name          # 必填，kebab-case 小写，目录名一致
description: 一句话说明     # 必填，说明何时使用本技能（会注入 system prompt）
---
（技能正文，markdown，加载后完整提供给模型）
\`\`\`

## 规范

1. description 以"当用户……时使用"开头，写清触发场景，让主 agent 能准确判断何时加载。
2. name 用 kebab-case（如 code-review、api-design）。
3. 正文直接给可执行指令：步骤、命令、模板、示例，避免空话。
4. 正文可引用项目内文件/脚本（用相对路径）。
5. 正文不超过 20000 字符（超长会被截断）。
6. 创建后验证：终端输入 /skills <名称> 能加载；必要时让用户跑一次真实任务确认效果。

## 示例模板（新技能）

\`\`\`markdown
---
name: <skill-name>
description: 当用户<触发场景>时使用。简要说明做什么。
---

# <技能名>

## 适用场景
（何时使用、何时不用）

## 步骤
1. ...
2. ...

## 示例
（可选：模板/命令示例）

## 注意
（常见坑）
\`\`\`

## 工作流程

1. 先与用户确认技能名与用途（一句话）。
2. 决定放项目级还是全局。
3. 用 write_file 创建 SKILL.md（含 frontmatter 与正文）。
4. 用 list_dir/read_file 复核内容，必要时参考同类既有技能。
5. 用 /skills（如可用）或提示用户在 TUI 输入 /skills 验证加载。`,
  },
];

/* 解析 frontmatter(name/description) + 正文 */
export function parseSkillFile(text) {
  const s = String(text ?? "");
  let name = "";
  let description = "";
  let body = s;
  const m = s.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (m) {
    body = m[2];
    for (const line of m[1].split("\n")) {
      const kv = line.match(/^\s*([a-zA-Z-]+):\s*(.*)\s*$/);
      if (kv) {
        const val = kv[2].trim().replace(/^['"]|['"]$/g, "");
        if (kv[1] === "name") name = val;
        if (kv[1] === "description") description = val;
      }
    }
  }
  return { name: name || path.basename(path.dirname(path.dirname(SKILL_FILE))), description, body: body.trim() };
}

/* 候选技能目录 */
function skillRoots(cwd) {
  const dirs = [];
  if (cwd) dirs.push(path.join(cwd, ".ux-agent", "skills"));
  dirs.push(path.join(os.homedir(), ".config", "ux-agent", "skills"));
  return dirs;
}

/* 扫描所有可用技能(项目级优先,同名覆盖全局;最后并入内置技能) */
export function listSkills(cwd) {
  const map = {};
  for (const root of skillRoots(cwd)) {
    let entries;
    try { entries = fs.readdirSync(root, { withFileTypes: true }); } catch { continue; }
    for (const e of entries) {
      if (!e.isDirectory()) continue;
      const file = path.join(root, e.name, SKILL_FILE);
      try {
        const raw = fs.readFileSync(file, "utf8");
        const skill = parseSkillFile(raw);
        if (skill.name) map[skill.name] = { ...skill, dir: path.join(root, e.name) };
      } catch {}
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

/* 生成 system prompt 中的技能描述段 */
export function skillPromptBlock(cwd) {
  const skills = listSkills(cwd);
  if (!skills.length) return "";
  const lines = skills.map((s) => `- ${s.name}: ${s.description || "(无描述)"}`);
  return "\n\n[可用技能]\n你拥有以下项目技能,任务匹配时用 use_skill 工具加载完整指令后再执行:\n" + lines.join("\n");
}
