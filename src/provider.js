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

import { loadConfig, getActiveProvider, getProviderApiKey } from "./config.js";

export class ApiError extends Error {
  constructor(message, type, status) {
    super(message);
    this.type = type;
    this.status = status;
  }
}

function activeCfg() {
  const cfg = loadConfig();
  const provider = getActiveProvider();
  return { cfg, provider };
}

/* 解析 OpenAI 兼容的流式 SSE 行 */
function parseSSELine(line, state) {
  if (!line.startsWith("data:")) return null;
  const data = line.slice(5).trim();
  if (data === "[DONE]") return { done: true };
  try {
    const json = JSON.parse(data);
    const delta = json.choices?.[0]?.delta || {};
    const usage = json.usage;
    return {
      done: false,
      content: typeof delta.content === "string" ? delta.content : "",
      reasoning: typeof delta.reasoning_content === "string" ? delta.reasoning_content : "",
      toolCalls: delta.tool_calls || null,
      finishReason: json.choices?.[0]?.finish_reason || null,
      usage,
      model: json.model,
    };
  } catch {
    return null;
  }
}

/* 聊天:支持流式与非流式。
 * 流式时返回 async generator,逐步产出 {content, reasoning, toolCalls, finishReason} */
export async function* chatStream(messages, { model, tools, signal, stream = true } = {}) {
  const { cfg, provider } = activeCfg();
  const apiKey = getProviderApiKey(provider.id);
  /* 非 thinking 提供商:剥离 reasoning_content,避免上游报错 */
  let outMessages = messages;
  if (!provider.supportsThinking) {
    outMessages = messages.map((m) => {
      if (m.role !== "assistant") return m;
      const { reasoning_content, ...rest } = m;
      return rest;
    });
  }
  const body = {
    model: model || cfg.model,
    messages: outMessages,
    stream,
  };
  if (tools && tools.length) body.tools = tools;
  /* DeepSeek 需要 thinking 参数才能启用推理模式 */
  if (provider.id === "deepseek") {
    body.thinking = { type: "enabled" };
  }

  const res = await fetch(`${provider.baseUrl}/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
    },
    body: JSON.stringify(body),
    signal,
  });

  /* 非流式响应(上游不支持 stream 时) */
  const ct = res.headers.get("content-type") || "";
  if (!ct.includes("text/event-stream")) {
    let data = {};
    try { data = await res.json(); } catch {
      throw new ApiError(`上游返回非 JSON（HTTP ${res.status}）`, "bad_response", res.status);
    }
    if (!res.ok || data.error) {
      const err = data.error || {};
      const msg = typeof err === "string" ? err : err.message || `HTTP ${res.status}`;
      const type = typeof err === "object" && err.type ? err.type : null;
      throw new ApiError(msg, type, res.status);
    }
    const msg = data.choices?.[0]?.message || {};
    yield {
      content: msg.content || "",
      reasoning: msg.reasoning_content || "",
      toolCalls: msg.tool_calls || [],
      finishReason: data.choices?.[0]?.finish_reason || "stop",
      usage: data.usage,
      model: data.model,
    };
    return;
  }

  /* 流式响应 */
  if (!res.ok || !res.body) {
    throw new ApiError(`流式请求失败（HTTP ${res.status}）`, "bad_response", res.status);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  let acc = { content: "", reasoning: "", toolCalls: [], finishReason: null, usage: null, model: null };
  const flush = (parsed) => {
    if (!parsed) return null;
    if (parsed.content) acc.content += parsed.content;
    if (parsed.reasoning) acc.reasoning += parsed.reasoning;
    if (parsed.finishReason) acc.finishReason = parsed.finishReason;
    if (parsed.usage) acc.usage = parsed.usage;
    if (parsed.model) acc.model = parsed.model;
    if (parsed.toolCalls) {
      for (const tc of parsed.toolCalls) {
        /* 增量 tool_calls: index 合并 */
        const idx = tc.index ?? 0;
        if (!acc.toolCalls[idx]) {
          acc.toolCalls[idx] = { id: tc.id || `call_${idx + 1}`, type: "function", function: { name: "", arguments: "" } };
        }
        if (tc.id) acc.toolCalls[idx].id = tc.id;
        if (tc.function?.name) acc.toolCalls[idx].function.name += tc.function.name;
        if (tc.function?.arguments) acc.toolCalls[idx].function.arguments += tc.function.arguments;
      }
    }
    return {
      content: parsed.content || "",
      reasoning: parsed.reasoning || "",
      toolCalls: parsed.toolCalls || [],
      finishReason: parsed.finishReason || null,
      partial: true,
    };
  };

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    const lines = buf.split("\n");
    buf = lines.pop();
    for (const line of lines) {
      const parsed = parseSSELine(line.trim(), acc);
      if (parsed && parsed.done) {
        /* done 标记:content/reasoning 已通过增量 yield 交付,不能重复携带 */
        yield { done: true, toolCalls: acc.toolCalls, finishReason: acc.finishReason || "stop", usage: acc.usage, model: acc.model };
        return;
      }
      const out = flush(parsed);
      if (out && (out.content || out.reasoning || out.toolCalls.length)) yield out;
    }
  }
  if (buf.trim()) {
    const parsed = parseSSELine(buf.trim(), acc);
    const out = flush(parsed);
    if (out && (out.content || out.reasoning || out.toolCalls.length)) yield out;
  }
  yield { done: true, toolCalls: acc.toolCalls, finishReason: acc.finishReason || "stop", usage: acc.usage, model: acc.model };
}

/* 非流式便捷调用 */
export async function chat(messages, opts = {}) {
  let last = null;
  for await (const chunk of chatStream(messages, { ...opts, stream: false })) {
    last = chunk;
  }
  return {
    content: last?.content || "",
    reasoning: last?.reasoning || "",
    toolCalls: last?.toolCalls || [],
    finishReason: last?.finishReason || "stop",
    usage: last?.usage || { prompt_tokens: 0, completion_tokens: 0 },
    model: last?.model || opts.model || loadConfig().model,
  };
}

export async function listModels() {
  const { provider } = activeCfg();
  /* 本地网关从 API 拉模型列表,其他提供商用配置 */
  if (provider.id === "ux-gateway") {
    try {
      const apiKey = getProviderApiKey(provider.id);
      const res = await fetch(`${provider.baseUrl}/../api/models`, {
        headers: apiKey ? { Authorization: `Bearer ${apiKey}` } : {},
      });
      if (res.ok) {
        const data = await res.json();
        if (data.models && data.models.length) {
          return data.models.map((m) => m.id || m.name);
        }
      }
    } catch {}
  }
  return provider.models || [];
}

export async function getProfile() {
  const { cfg, provider } = activeCfg();
  /* 本地网关支持 /me;外部提供商没有账户接口 */
  if (provider.id === "ux-gateway") {
    const apiKey = getProviderApiKey(provider.id);
    const res = await fetch(`${provider.baseUrl}/me`, {
      method: "GET",
      headers: apiKey ? { Authorization: `Bearer ${apiKey}` } : {},
    });
    if (res.ok) {
      try {
        const data = await res.json();
        return data.user || null;
      } catch { return null; }
    }
    return null;
  }
  return null;
}

export async function checkApiKey(apiKey, providerId) {
  const { cfg, provider } = activeCfg();
  const target = providerId || provider.id;
  const p = loadConfig().providers.find((x) => x.id === target) || provider;
  try {
    const res = await fetch(`${p.baseUrl}/chat/completions`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${apiKey}` },
      body: JSON.stringify({
        model: p.defaultModel || p.models[0] || "default",
        messages: [{ role: "user", content: "ping" }],
        max_tokens: 1,
      }),
    });
    return res.ok;
  } catch {
    return false;
  }
}

export { DEFAULT_BASE_URL } from "./config.js";
