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

/* ============ 上下文窗口与 Token 估算(支持自动压缩) ============ */

/* 模型上下文窗口(约值)。deepseek-v4 pro/flash 均为 1M,GLM 系列未知按 128k 兜底。 */
export const DEFAULT_CONTEXT_WINDOW = 131072;

const CONTEXT_WINDOWS = {
  "deepseek-v4-pro": 1_000_000,
  "deepseek-v4-flash": 1_000_000,
  "deepseek-v4.5": 1_000_000,
  "deepseek-reasoner": 1_000_000,
  "glm-4-flash": 131072,
  "glm-4-flash-proxy": 131072,
  "glm-4.6": 131072,
  "glm-4.5": 131072,
  "glm-4": 131072,
};

/* 自动压缩阈值:达到上下文窗口的该比例时触发摘要压缩 */
export const COMPACT_RATIO = 0.62;
/* 请求历史上限:窗口的该比例(预留 token 给工具定义/输出) */
export const REQUEST_HISTORY_RATIO = 0.72;
/* 显示高水位警告阈值 */
export const WARN_RATIO = 0.5;

export function getContextWindow(model) {
  const key = String(model || "").toLowerCase().trim();
  if (CONTEXT_WINDOWS[key]) return CONTEXT_WINDOWS[key];
  /* 数字后缀解析: 128k / 32k / 1m / 2m 等 */
  const m = key.match(/(\d+(?:\.\d+)?)\s*(m|k)\b/);
  if (m) {
    const n = parseFloat(m[1]);
    return m[2] === "m" ? Math.round(n * 1_000_000) : Math.round(n * 1000);
  }
  return DEFAULT_CONTEXT_WINDOW;
}

export function compactThreshold(model) {
  return Math.floor(getContextWindow(model) * COMPACT_RATIO);
}

export function requestHistoryBudget(model) {
  return Math.floor(getContextWindow(model) * REQUEST_HISTORY_RATIO);
}

export function warnTokens(model) {
  return Math.floor(getContextWindow(model) * WARN_RATIO);
}

/* 简易 token 估算:CJK 约 1 字 1 token,其他字符约 3.1 字 1 token(与字符数成正比) */
export function estimateTokenCount(input) {
  const s = String(input == null ? "" : input);
  if (!s) return 0;
  const cjk = (s.match(/[\u2E80-\u9FFF\uF900-\uFAFF\uFF00-\uFFEF\u3000-\u303F]/g) || []).length;
  const other = s.length - cjk;
  return Math.max(0, Math.round(cjk * 1.1 + other * 0.32) + 2);
}

export function estimateMessagesTokens(msgs) {
  if (!Array.isArray(msgs)) return 0;
  let t = 0;
  for (const m of msgs) {
    const content = typeof m === "string" ? m : m.content || "";
    t += estimateTokenCount(content) + 4;
    if (m?.tool_calls) {
      for (const tc of m.tool_calls) {
        t += estimateTokenCount(JSON.stringify(tc.function || tc.function_name || "")) + 8;
      }
    }
  }
  return t;
}

/* 从后往前裁剪出 ≤ maxTokens 的历史(按 token 预算贪心) */
export function fitConversation(msgs, maxTokens) {
  const out = [];
  let used = 0;
  for (let i = msgs.length - 1; i >= 0; i--) {
    const t = estimateMessagesTokens([msgs[i]]);
    if (out.length && used + t > maxTokens) break;
    out.unshift(msgs[i]);
    used += t;
  }
  /* 修复:若截断后首条是 tool 消息,补上对应的 assistant(tool_calls) 消息,避免 API 报错 */
  if (out.length && out[0].role === "tool") {
    const firstToolId = out[0].tool_call_id;
    for (let i = 0; i < msgs.length; i++) {
      if (msgs[i].role === "assistant" && msgs[i].tool_calls?.some((tc) => tc.id === firstToolId)) {
        out.unshift(msgs[i]);
        break;
      }
    }
  }
  return out;
}

/* 压缩提示(CLaude Code 风格的九段式结构化摘要) */
export const COMPACTION_INSTRUCTIONS = `请把上面的对话压缩成一份结构化摘要,用于替换全部历史并继续任务。

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
4. 尽量完整,摘要本身可以长(它会替代全部历史,只受模型上下文限制)。
5. 用中文。`;

export function buildCompactionConversation(conversation) {
  return [
    {
      role: "system",
      content:
        "你是会话压缩器。下一条是待压缩的完整对话历史,请按给定要求输出结构化摘要。",
    },
    {
      role: "user",
      content: COMPACTION_INSTRUCTIONS + "\n\n—— 历史对话开始 ——\n\n" +
        conversation
          .map((m) => `[${m.role}]${m.content || "(工具调用)"}`)
          .join("\n\n") +
        "\n\n—— 历史对话结束 ——",
    },
  ];
}