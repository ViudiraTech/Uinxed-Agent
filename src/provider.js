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

/* ---- OpenAI Responses API (wire_api = "responses", 如 gpt-5.x) ---- */

/* chat 消息 → responses API input */
function toResponsesInput(messages) {
  return messages.map((m) => {
    if (m.role === "system") {
      return { role: "system", content: [{ type: "input_text", text: m.content }] };
    }
    if (m.role === "user") {
      return { role: "user", content: m.content || "" };
    }
    if (m.role === "tool") {
      return { role: "tool", tool_call_id: m.tool_call_id, content: String(m.content || "").slice(0, 100000) };
    }
    const out = { role: "assistant", content: m.content || "" };
    if (m.tool_calls && m.tool_calls.length) {
      out.tool_calls = m.tool_calls.map((tc) => ({
        id: tc.id || tc.call_id,
        type: "function",
        function: { name: tc.function?.name || "", arguments: tc.function?.arguments || "" },
      }));
    }
    if (m.reasoning_content) out.reasoning = { summary: [String(m.reasoning_content)] };
    return out;
  });
}

/* 解析 responses API 流式 SSE 事件 */
function parseResponseLine(line) {
  if (!line.startsWith("data:")) return null;
  const data = line.slice(5).trim();
  if (!data) return null;
  let ev;
  try { ev = JSON.parse(data); } catch { return null; }
  switch (ev.type) {
    case "response.output_text.delta":
      return { content: ev.delta || "" };
    case "response.reasoning_summary_text.delta":
    case "response.reasoning.delta":
      return { reasoning: ev.delta || "" };
    case "response.output_item.added":
      if (ev.item?.type === "function_call") {
        return { tcInit: { id: ev.item.id || ev.item.call_id, name: ev.item.name || ev.item.function?.name || "" } };
      }
      return null;
    case "response.function_call_arguments.delta":
      return { tcDelta: { id: ev.item_id, arguments: ev.delta || "" } };
    case "response.function_call_arguments.done":
      return { tcDone: { id: ev.item_id, arguments: ev.arguments || "" } };
    case "response.completed": {
      const u = ev.response?.usage || {};
      return {
        done: true,
        finishReason: ev.response?.status === "incomplete" ? "max_tokens" : "stop",
        usage: u ? { prompt_tokens: u.input_tokens || 0, completion_tokens: u.output_tokens || 0 } : null,
      };
    }
    case "response.incomplete":
      return { done: true, finishReason: "max_tokens" };
    case "error":
      throw new ApiError(ev.message || "Responses API 错误", "api_error", 400);
    default:
      return null;
  }
}

/* 按 item_id 合并增量 function call */
function applyTcDelta(acc, parsed) {
  if (parsed.tcInit) {
    const id = parsed.tcInit.id;
    if (!acc.toolCalls[id]) acc.toolCalls[id] = { id, type: "function", function: { name: "", arguments: "" } };
    acc.toolCalls[id].function.name = parsed.tcInit.name;
  }
  if (parsed.tcDelta && acc.toolCalls[parsed.tcDelta.id]) {
    acc.toolCalls[parsed.tcDelta.id].function.arguments += parsed.tcDelta.arguments;
  }
  if (parsed.tcDone && acc.toolCalls[parsed.tcDone.id]) {
    acc.toolCalls[parsed.tcDone.id].function.arguments = parsed.tcDone.arguments;
  }
}

/* 聊天:支持流式与非流式。
 * 流式时返回 async generator,逐步产出 {content, reasoning, toolCalls, finishReason}
 * wire_api = "responses" 的提供商(如 gpt-5.x)走 Responses API,其余走 /chat/completions。 */
export async function* chatStream(messages, { model, tools, signal, stream = true } = {}) {
  const { cfg, provider } = activeCfg();
  if (provider.wireApi === "responses") {
    yield* responsesStream(messages, { model: model || cfg.model, tools, signal, stream });
    return;
  }
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
  /* 支持 effort 的提供商:透传全局 reasoning effort(low/medium/high/xhigh/max)。
   * supercode 为客户端编排模式,API 层映射为 max */
  if (provider.supportsEffort && cfg.effort) {
    body.reasoning_effort = cfg.effort === "supercode" ? "max" : cfg.effort;
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

/* Responses API 流式(OpenAI gpt-5.x / wire_api = "responses") */
async function* responsesStream(messages, { model, tools, signal, stream = true }) {
  const { cfg, provider } = activeCfg();
  const apiKey = getProviderApiKey(provider.id);
  const body = {
    model,
    input: toResponsesInput(messages),
    stream,
    store: false,
  };
  if (tools && tools.length) body.tools = tools;
  if (cfg.effort) body.reasoning = { effort: cfg.effort === "supercode" ? "max" : cfg.effort };
  else if (provider.reasoningEffort) body.reasoning = { effort: provider.reasoningEffort };
  if (apiKey) body.auth = { type: "bearer", key: apiKey };

  const res = await fetch(`${provider.baseUrl}/responses`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
      ...(provider.headers || {}),
    },
    body: JSON.stringify(body),
    signal,
  });

  /* 非流式响应 */
  const ct = res.headers.get("content-type") || "";
  if (!ct.includes("text/event-stream")) {
    let data = {};
    try { data = await res.json(); } catch {
      throw new ApiError(`上游返回非 JSON（HTTP ${res.status}）`, "bad_response", res.status);
    }
    if (!res.ok || data.error) {
      const err = data.error || {};
      const msg = typeof err === "string" ? err : err.message || `HTTP ${res.status}`;
      throw new ApiError(msg, typeof err === "object" && err.type ? err.type : null, res.status);
    }
    const u = data.usage || {};
    const toolsOut = (data.output || [])
      .filter((o) => o.type === "function_call")
      .map((o) => ({ id: o.id || o.call_id, type: "function", function: { name: o.name || "", arguments: o.arguments || "" } }));
    const textOut = (data.output || [])
      .filter((o) => o.type === "message")
      .map((o) => (o.content || []).filter((c) => c.type === "output_text").map((c) => c.text).join(""))
      .join("");
    yield {
      content: textOut || "",
      reasoning: "",
      toolCalls: toolsOut,
      finishReason: data.status === "incomplete" ? "max_tokens" : "stop",
      usage: { prompt_tokens: u.input_tokens || 0, completion_tokens: u.output_tokens || 0 },
      model: data.model,
    };
    return;
  }

  if (!res.ok || !res.body) {
    throw new ApiError(`流式请求失败（HTTP ${res.status}）`, "bad_response", res.status);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  const acc = { content: "", reasoning: "", toolCalls: {}, finishReason: null, usage: null };

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    const lines = buf.split("\n");
    buf = lines.pop();
    for (const line of lines) {
      const parsed = parseResponseLine(line.trim());
      if (!parsed) continue;
      if (parsed.done) {
        yield { done: true, toolCalls: Object.values(acc.toolCalls), finishReason: acc.finishReason || "stop", usage: acc.usage };
        return;
      }
      if (parsed.content) acc.content += parsed.content;
      if (parsed.reasoning) acc.reasoning += parsed.reasoning;
      if (parsed.finishReason) acc.finishReason = parsed.finishReason;
      if (parsed.usage) acc.usage = parsed.usage;
      applyTcDelta(acc, parsed);
      if (parsed.content || parsed.reasoning) yield { content: parsed.content, reasoning: parsed.reasoning, partial: true };
    }
  }
  if (buf.trim()) {
    const parsed = parseResponseLine(buf.trim());
    if (parsed && !parsed.done) {
      if (parsed.content) acc.content += parsed.content;
      if (parsed.reasoning) acc.reasoning += parsed.reasoning;
      applyTcDelta(acc, parsed);
      if (parsed.content || parsed.reasoning) yield { content: parsed.content, reasoning: parsed.reasoning, partial: true };
    }
  }
  yield { done: true, toolCalls: Object.values(acc.toolCalls), finishReason: acc.finishReason || "stop", usage: acc.usage };
}
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
  if (p.requiresAuth === false) return true;
  /* Responses API 提供商用 /responses 端点验证 */
  if (p.wireApi === "responses") {
    try {
      const res = await fetch(`${p.baseUrl}/responses`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${apiKey}`,
          ...(p.headers || {}),
        },
        body: JSON.stringify({
          model: p.defaultModel || p.models[0] || "default",
          input: [{ role: "user", content: "ping" }],
          stream: false,
          store: false,
          max_output_tokens: 1,
        }),
      });
      return res.ok;
    } catch {
      return false;
    }
  }
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
