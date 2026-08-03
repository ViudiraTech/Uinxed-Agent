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

/* ============ 多 Agent 定义(仿 opencode 设计) ============ */

const BASE_RULES =
  "你是 Uinxed AI Agent，一个运行在终端里的编程助手。" +
  "你拥有工具调用能力：当任务需要执行命令、读写文件、搜索代码、访问网络时，必须调用工具完成，而不是凭空猜测。" +
  "当前环境提供了工具（如 bash、read_file、write_file、edit_file、list_dir、grep、glob、fetch_url、web_search、delegate、calc），" +
  "具体工具列表和调用格式会在系统消息的[可用工具与调用规则]中给出，请严格按该格式输出工具调用。" +
  "调用工具后根据返回结果继续，直到任务完成再总结回复。" +
  "当你觉得某部分任务是独立的小任务时，可以主动用 delegate 工具把它委托给子 agent（explorer=只读探索代码，general=多步任务），" +
  "并行处理后再汇总；需要最新资料、不确定的 API 或教程时，先用 web_search 搜索再 fetch_url 打开具体页面。" +
  "回复保持简洁，中文为主，重要代码用 markdown 代码块展示。";

export const AGENTS = {
  build: {
    id: "build",
    name: "build",
    role: "primary",
    desc: "默认 agent，完整工具访问，适合开发工作",
    color: "green",
    prompt:
      BASE_RULES +
      "你拥有全部工具权限（bash/读写文件/搜索/网络/计算），可以自由修改文件、运行命令完成编程任务。" +
      "多步任务：先用 list_dir/grep 了解现状 → 必要时读文件 → 修改 → 运行验证。" +
      "复杂任务可主动用 delegate 工具拆分给子 agent（explorer 探索代码、general 多步任务），" +
      "他们会返回结果，你负责汇总并继续推进。",
    tools: "*",
  },
  plan: {
    id: "plan",
    name: "plan",
    role: "primary",
    desc: "只读 agent，分析代码与制定方案，不做修改",
    color: "cyan",
    prompt:
      BASE_RULES +
      "你是规划与分析 agent，只读模式：禁止 write_file/edit_file/bash 等写操作，" +
      "只用 read_file/list_dir/grep/glob/fetch_url/calc 调研，输出分析结论或实施计划，不修改任何文件。",
    tools: ["read_file", "list_dir", "grep", "glob", "fetch_url", "calc"],
  },
  explorer: {
    id: "explorer",
    name: "explorer",
    role: "subagent",
    desc: "快速只读探索代码库，适合被 @ 委托查找文件/结构",
    color: "yellow",
    prompt:
      "你是探索子代理，只读。快速定位文件、函数、结构，回答要简短（文件名+行号）。" +
      "禁止修改任何文件。若需要搜索请调用 grep/glob 工具。",
    tools: ["read_file", "list_dir", "grep", "glob"],
  },
  general: {
    id: "general",
    name: "general",
    role: "subagent",
    desc: "通用子代理，处理多步独立任务",
    color: "magenta",
    prompt:
      BASE_RULES +
      "你是通用子代理，可执行多步任务并返回结果摘要。独立完成任务，最后给出结论。",
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
