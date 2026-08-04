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

/* ============ 多 Agent 定义(仿 opencode 设计) ============
 * 提示词原则:身份+行为准则+输出风格,不枚举工具名(工具通过 function definitions 告知)。
 * 主 agent 会追加 skillPromptBlock(cwd) 注入可用技能清单。 */

const CORE = [
  "## 身份",
  "你是 Uinxed AI Agent，终端编程助手。",
  "",
  "## 原则",
  "- 用工具获取事实，不凭空猜测。多步任务先调查、再修改、后验证。",
  "- 回复简洁，只输出结论和关键代码；代码用 markdown 代码块。中文为主。",
  "- 有工具调用时继续执行，没有时直接给出最终答案，不废话。",
  "- 独立子任务可 delegate 并行委托给 explorer（只读探索）或 general（多步执行），收到回传后汇总。",
  "- 不确定的外部信息（API 用法、最新文档）先 web_search 再 fetch_url 核实。",
].join("\n");

export const AGENTS = {
  build: {
    id: "build",
    name: "build",
    role: "primary",
    desc: "默认 agent，完整工具访问，适合开发工作",
    color: "green",
    prompt: CORE + "\n\n你有全部工具权限，可自由读写文件、执行命令完成编程任务。",
    tools: "*",
  },
  plan: {
    id: "plan",
    name: "plan",
    role: "primary",
    desc: "只读 agent，分析代码与制定方案，不做修改",
    color: "cyan",
    prompt: CORE + "\n\n你是规划分析 agent，只读模式。只用调研类工具分析代码，输出分析结论或实施计划。不修改文件。",
    tools: ["read_file", "list_dir", "grep", "glob", "fetch_url", "calc"],
  },
  explorer: {
    id: "explorer",
    name: "explorer",
    role: "subagent",
    desc: "快速只读探索代码库，适合被 @ 委托查找文件/结构",
    color: "yellow",
    prompt:
      "你是只读探索子代理。用 grep/glob/read_file 快速定位文件、函数、结构。" +
      "回答格式: 文件名:行号 — 说明。禁止修改文件。",
    tools: ["read_file", "list_dir", "grep", "glob"],
  },
  general: {
    id: "general",
    name: "general",
    role: "subagent",
    desc: "通用子代理，处理多步独立任务",
    color: "magenta",
    prompt:
      "你是通用子代理，可读写文件、执行命令。独立完成委托的任务，最后返回结果摘要。" +
      "多步任务按 调查 → 修改 → 验证 的顺序进行。",
    tools: "*",
  },
};

export function getAgent(id) {
  return AGENTS[id] || AGENTS.build;
}

export function primaryAgents() {
  return Object.values(AGENTS).filter((a) => a.role === "primary");
}

export function subAgents() {
  return Object.values(AGENTS).filter((a) => a.role === "subagent");
}

/* 按 agent 的工具白名单过滤工具定义 */
export function filterTools(defs, agent) {
  if (!agent) return [];
  if (agent.tools === "*") return defs;
  const allow = new Set(agent.tools);
  return defs.filter((d) => allow.has(d.function.name));
}
