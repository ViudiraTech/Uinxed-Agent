#!/usr/bin/env node
var __defProp = Object.defineProperty;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __esm = (fn, res, err) => function __init() {
  if (err) throw err[0];
  try {
    return fn && (res = (0, fn[__getOwnPropNames(fn)[0]])(fn = 0)), res;
  } catch (e) {
    throw err = [e], e;
  }
};
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};

// src/config.js
var config_exports = {};
__export(config_exports, {
  BUILTIN_PROVIDERS: () => BUILTIN_PROVIDERS,
  DEFAULT_BASE_URL: () => DEFAULT_BASE_URL,
  DEFAULT_MODEL: () => DEFAULT_MODEL,
  clearHistory: () => clearHistory,
  getActiveProvider: () => getActiveProvider,
  getProviderApiKey: () => getProviderApiKey,
  loadConfig: () => loadConfig,
  removeProvider: () => removeProvider,
  saveConfig: () => saveConfig,
  setActiveProvider: () => setActiveProvider,
  setApiKey: () => setApiKey,
  setBaseUrl: () => setBaseUrl,
  setHistory: () => setHistory,
  setModel: () => setModel,
  setProviderApiKey: () => setProviderApiKey,
  setThinking: () => setThinking,
  upsertProvider: () => upsertProvider
});
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
function loadConfig() {
  try {
    const raw = fs.readFileSync(CONFIG_FILE, "utf8");
    const cfg2 = JSON.parse(raw);
    const providers = ensureProviders(cfg2.providers);
    return {
      apiKey: null,
      // 废弃:全部走 provider.apiKey
      baseUrl: cfg2.baseUrl || DEFAULT_BASE_URL,
      model: cfg2.model || DEFAULT_MODEL,
      history: Array.isArray(cfg2.history) ? cfg2.history : [],
      providers,
      activeProvider: cfg2.activeProvider || providers[0]?.id || "ux-gateway",
      thinking: cfg2.thinking !== false,
      cwd: cfg2.cwd || null
    };
  } catch {
    const providers = ensureProviders();
    return {
      apiKey: null,
      baseUrl: DEFAULT_BASE_URL,
      model: DEFAULT_MODEL,
      history: [],
      providers,
      activeProvider: providers[0].id,
      thinking: true,
      cwd: null
    };
  }
}
function ensureProviders(existing) {
  if (Array.isArray(existing) && existing.length) {
    const merged = existing.map((p) => {
      const tpl = BUILTIN_PROVIDERS.find((b) => b.id === p.id);
      return { ...tpl || {}, ...p };
    });
    for (const b of BUILTIN_PROVIDERS) {
      if (!merged.some((m) => m.id === b.id)) merged.push({ ...b });
    }
    return merged;
  }
  return BUILTIN_PROVIDERS.map((p) => ({ ...p }));
}
function saveConfig(partial) {
  const cur = loadConfig();
  const next = { ...cur, ...partial };
  fs.mkdirSync(CONFIG_DIR, { recursive: true });
  fs.writeFileSync(CONFIG_FILE, JSON.stringify(next, null, 2), "utf8");
  return next;
}
function getActiveProvider() {
  const cfg2 = loadConfig();
  return cfg2.providers.find((p) => p.id === cfg2.activeProvider) || cfg2.providers[0];
}
function setApiKey(apiKey) {
  saveConfig({ apiKey });
}
function setBaseUrl(baseUrl) {
  saveConfig({ baseUrl: baseUrl.replace(/\/+$/, "") });
}
function setModel(model) {
  saveConfig({ model });
}
function setProviderApiKey(providerId, apiKey) {
  const cfg2 = loadConfig();
  const providers = cfg2.providers.map(
    (p) => p.id === providerId ? { ...p, apiKey: apiKey || null } : p
  );
  saveConfig({ providers, apiKey: providerId === cfg2.activeProvider ? apiKey || cfg2.apiKey : cfg2.apiKey });
}
function getProviderApiKey(providerId) {
  const cfg2 = loadConfig();
  const p = cfg2.providers.find((x) => x.id === providerId);
  return p && p.apiKey || null;
}
function setActiveProvider(providerId) {
  const cfg2 = loadConfig();
  const p = cfg2.providers.find((x) => x.id === providerId);
  if (!p) return null;
  const next = {
    ...cfg2,
    activeProvider: providerId,
    baseUrl: p.baseUrl,
    model: p.defaultModel || cfg2.model
  };
  fs.mkdirSync(CONFIG_DIR, { recursive: true });
  fs.writeFileSync(CONFIG_FILE, JSON.stringify(next, null, 2), "utf8");
  return next;
}
function upsertProvider(provider) {
  const cfg2 = loadConfig();
  const id = provider.id || provider.name.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  const exists = cfg2.providers.some((p) => p.id === id);
  const newProvider = {
    id,
    name: provider.name,
    baseUrl: (provider.baseUrl || "").replace(/\/+$/, ""),
    apiKey: provider.apiKey || null,
    models: Array.isArray(provider.models) && provider.models.length ? provider.models : ["default"],
    defaultModel: provider.defaultModel || (Array.isArray(provider.models) ? provider.models[0] : "default"),
    builtin: false
  };
  const providers = exists ? cfg2.providers.map((p) => p.id === id ? { ...p, ...newProvider } : p) : [...cfg2.providers, newProvider];
  saveConfig({ providers });
  return newProvider;
}
function removeProvider(providerId) {
  const cfg2 = loadConfig();
  if (providerId === cfg2.activeProvider) return { error: "\u4E0D\u80FD\u5220\u9664\u5F53\u524D\u6D3B\u52A8\u7684\u63D0\u4F9B\u5546" };
  const providers = cfg2.providers.filter((p) => p.id !== providerId);
  saveConfig({ providers });
  return { ok: true };
}
function setHistory(history) {
  saveConfig({ history: history.slice(-200) });
}
function clearHistory() {
  saveConfig({ history: [] });
}
function setThinking(enabled) {
  saveConfig({ thinking: !!enabled });
}
var CONFIG_DIR, CONFIG_FILE, DEFAULT_BASE_URL, DEFAULT_MODEL, BUILTIN_PROVIDERS;
var init_config = __esm({
  "src/config.js"() {
    CONFIG_DIR = path.join(os.homedir(), ".config", "ux-agent");
    CONFIG_FILE = path.join(CONFIG_DIR, "config.json");
    DEFAULT_BASE_URL = "http://localhost:8080/v1";
    DEFAULT_MODEL = "glm-4-flash";
    BUILTIN_PROVIDERS = [
      {
        id: "ux-gateway",
        name: "\u672C\u5730\u7F51\u5173",
        baseUrl: DEFAULT_BASE_URL,
        apiKey: null,
        models: ["glm-4-flash", "glm-4-flash-proxy"],
        defaultModel: "glm-4-flash",
        builtin: true
      },
      {
        id: "deepseek",
        name: "DeepSeek",
        baseUrl: "https://api.deepseek.com/v1",
        apiKey: null,
        models: ["deepseek-v4-pro", "deepseek-v4-flash"],
        defaultModel: "deepseek-v4-flash",
        builtin: true,
        supportsThinking: true
      }
    ];
  }
});

// src/index.js
import React4 from "react";
import { render } from "ink";

// src/App.jsx
import React3, { useEffect, useRef, useState, useCallback } from "react";
import { Box as Box3, Text as Text3, useApp, useInput, useStdout } from "ink";
import TextInput from "ink-text-input";

// src/Markdown.jsx
import React from "react";
import { Box, Text } from "ink";
function parseInline(seg, baseColor = "white") {
  const parts = [];
  const re = /(\*\*[^*]+\*\*|`[^`]+`|\*[^*]+\*)/g;
  let last = 0;
  let m;
  while (m = re.exec(seg)) {
    if (m.index > last) parts.push({ text: seg.slice(last, m.index), bold: false, code: false, italic: false });
    const token = m[0];
    if (token.startsWith("**")) {
      parts.push({ text: token.slice(2, -2), bold: true, code: false, italic: false });
    } else if (token.startsWith("`")) {
      parts.push({ text: token.slice(1, -1), bold: false, code: true, italic: false });
    } else {
      parts.push({ text: token.slice(1, -1), bold: false, code: false, italic: true });
    }
    last = m.index + token.length;
  }
  if (last < seg.length) parts.push({ text: seg.slice(last), bold: false, code: false, italic: false });
  if (!parts.length) parts.push({ text: seg, bold: false, code: false, italic: false });
  return parts.map((p) => /* @__PURE__ */ React.createElement(
    Text,
    {
      key: Math.random(),
      bold: p.bold,
      italic: p.italic,
      color: p.code ? "yellow" : baseColor,
      backgroundColor: p.code ? "#1a1a2e" : void 0
    },
    p.text
  ));
}
function markdownToLines(md, width) {
  const lines = [];
  const src = String(md || "").replace(/\r\n/g, "\n").split("\n");
  let i = 0;
  while (i < src.length) {
    const line = src[i];
    if (/^```/.test(line.trim())) {
      const lang = line.trim().slice(3).trim();
      const code = [];
      i++;
      while (i < src.length && !/^```/.test(src[i].trim())) {
        code.push(src[i]);
        i++;
      }
      i++;
      lines.push({ type: "code", text: code.join("\n"), lang });
      continue;
    }
    const h = /^(#{1,4})\s+(.*)$/.exec(line);
    if (h) {
      lines.push({ type: "heading", level: h[1].length, text: h[2] });
      i++;
      continue;
    }
    const li = /^(\s*)[-*+]\s+(.*)$/.exec(line);
    if (li) {
      lines.push({ type: "list", indent: li[1].length, text: li[2] });
      i++;
      continue;
    }
    const oi = /^(\s*)\d+[.)]\s+(.*)$/.exec(line);
    if (oi) {
      lines.push({ type: "list", indent: oi[1].length, ordered: true, text: oi[2] });
      i++;
      continue;
    }
    const qt = /^>\s?(.*)$/.exec(line);
    if (qt) {
      lines.push({ type: "quote", text: qt[1] });
      i++;
      continue;
    }
    if (/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      lines.push({ type: "hr" });
      i++;
      continue;
    }
    lines.push({ type: "text", text: line });
    i++;
  }
  return lines;
}
function Markdown({ content, width = 100 }) {
  const lines = markdownToLines(content, width);
  return /* @__PURE__ */ React.createElement(Box, { flexDirection: "column" }, lines.map((l, idx) => {
    if (l.type === "code") {
      const codeLines = l.text.split("\n");
      return /* @__PURE__ */ React.createElement(Box, { key: idx, flexDirection: "column", marginY: 1, borderStyle: "round", borderColor: "gray" }, l.lang && /* @__PURE__ */ React.createElement(Text, { dimColor: true, backgroundColor: "#1a1a2e" }, " ", l.lang, " "), codeLines.map((cl, ci) => /* @__PURE__ */ React.createElement(Text, { key: ci, color: "yellow", backgroundColor: "#1a1a2e", wrap: "wrap" }, cl || " ")));
    }
    if (l.type === "heading") {
      const color = l.level === 1 ? "cyan" : l.level === 2 ? "blue" : "white";
      return /* @__PURE__ */ React.createElement(Text, { key: idx, bold: true, color }, "#".repeat(l.level), " ", l.text);
    }
    if (l.type === "list") {
      return /* @__PURE__ */ React.createElement(Text, { key: idx, color: "white", wrap: "wrap" }, "  ".repeat(Math.min(l.indent, 4) / 2 || 0), "  ", l.ordered ? "" : "\u2022 ", parseInline(l.text));
    }
    if (l.type === "quote") {
      return /* @__PURE__ */ React.createElement(Text, { key: idx, color: "gray", wrap: "wrap" }, "  ", "\u258D", parseInline(l.text, "gray"));
    }
    if (l.type === "hr") {
      return /* @__PURE__ */ React.createElement(Text, { key: idx, dimColor: true }, "\u2500".repeat(Math.min(width - 6, 40)));
    }
    return /* @__PURE__ */ React.createElement(Text, { key: idx, color: "white", wrap: "wrap" }, parseInline(l.text));
  }));
}

// src/Thinking.jsx
import React2 from "react";
import { Box as Box2, Text as Text2 } from "ink";
function ThinkingBlock({ reasoning, expanded, onToggle }) {
  if (!reasoning) return null;
  const lines = String(reasoning).trim().split("\n");
  const seconds = Math.max(1, Math.round(lines.length / 20));
  return /* @__PURE__ */ React2.createElement(Box2, { flexDirection: "column", width: "100%" }, /* @__PURE__ */ React2.createElement(Text2, { bold: true, color: "#9aa3c2", wrap: "wrap" }, /* @__PURE__ */ React2.createElement(Text2, { color: "yellow" }, "\u25C8 "), /* @__PURE__ */ React2.createElement(Text2, { color: expanded ? "cyan" : "#9aa3c2", bold: expanded }, expanded ? `Thought for ${seconds}s \u25BE` : `Thought for ${seconds}s \u25B8`)), expanded && /* @__PURE__ */ React2.createElement(
    Box2,
    {
      flexDirection: "column",
      marginTop: 0,
      marginLeft: 3,
      borderStyle: "round",
      borderColor: "#3d4460",
      paddingX: 1
    },
    lines.slice(0, 120).map((l, i) => /* @__PURE__ */ React2.createElement(Text2, { key: i, color: "#8a92b0", wrap: "wrap" }, l || " ")),
    lines.length > 120 && /* @__PURE__ */ React2.createElement(Text2, { color: "#6b7392" }, "\u2026\uFF08\u63A8\u7406\u8FC7\u957F\uFF0C\u5DF2\u622A\u65AD\uFF09")
  ));
}

// src/provider.js
init_config();
init_config();
var ApiError = class extends Error {
  constructor(message, type, status) {
    super(message);
    this.type = type;
    this.status = status;
  }
};
function activeCfg() {
  const cfg2 = loadConfig();
  const provider = getActiveProvider();
  return { cfg: cfg2, provider };
}
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
      model: json.model
    };
  } catch {
    return null;
  }
}
async function* chatStream(messages, { model, tools, signal, stream = true } = {}) {
  const { cfg: cfg2, provider } = activeCfg();
  const apiKey = getProviderApiKey(provider.id);
  let outMessages = messages;
  if (!provider.supportsThinking) {
    outMessages = messages.map((m) => {
      if (m.role !== "assistant") return m;
      const { reasoning_content, ...rest } = m;
      return rest;
    });
  }
  const body = {
    model: model || cfg2.model,
    messages: outMessages,
    stream
  };
  if (tools && tools.length) body.tools = tools;
  if (provider.id === "deepseek") {
    body.thinking = { type: "enabled" };
  }
  const res = await fetch(`${provider.baseUrl}/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...apiKey ? { Authorization: `Bearer ${apiKey}` } : {}
    },
    body: JSON.stringify(body),
    signal
  });
  const ct = res.headers.get("content-type") || "";
  if (!ct.includes("text/event-stream")) {
    let data = {};
    try {
      data = await res.json();
    } catch {
      throw new ApiError(`\u4E0A\u6E38\u8FD4\u56DE\u975E JSON\uFF08HTTP ${res.status}\uFF09`, "bad_response", res.status);
    }
    if (!res.ok || data.error) {
      const err = data.error || {};
      const msg2 = typeof err === "string" ? err : err.message || `HTTP ${res.status}`;
      const type = typeof err === "object" && err.type ? err.type : null;
      throw new ApiError(msg2, type, res.status);
    }
    const msg = data.choices?.[0]?.message || {};
    yield {
      content: msg.content || "",
      reasoning: msg.reasoning_content || "",
      toolCalls: msg.tool_calls || [],
      finishReason: data.choices?.[0]?.finish_reason || "stop",
      usage: data.usage,
      model: data.model
    };
    return;
  }
  if (!res.ok || !res.body) {
    throw new ApiError(`\u6D41\u5F0F\u8BF7\u6C42\u5931\u8D25\uFF08HTTP ${res.status}\uFF09`, "bad_response", res.status);
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
      partial: true
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
        yield { ...acc, done: true };
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
  yield { ...acc, done: true };
}
async function getProfile() {
  const { cfg: cfg2, provider } = activeCfg();
  if (provider.id === "ux-gateway") {
    const apiKey = getProviderApiKey(provider.id);
    const res = await fetch(`${provider.baseUrl}/me`, {
      method: "GET",
      headers: apiKey ? { Authorization: `Bearer ${apiKey}` } : {}
    });
    if (res.ok) {
      try {
        const data = await res.json();
        return data.user || null;
      } catch {
        return null;
      }
    }
    return null;
  }
  return null;
}
async function checkApiKey(apiKey, providerId) {
  const { cfg: cfg2, provider } = activeCfg();
  const target = providerId || provider.id;
  const p = loadConfig().providers.find((x) => x.id === target) || provider;
  try {
    const res = await fetch(`${p.baseUrl}/chat/completions`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${apiKey}` },
      body: JSON.stringify({
        model: p.defaultModel || p.models[0] || "default",
        messages: [{ role: "user", content: "ping" }],
        max_tokens: 1
      })
    });
    return res.ok;
  } catch {
    return false;
  }
}

// src/tools.js
import { execSync } from "node:child_process";
import fs2 from "node:fs";
import path2 from "node:path";
var TOOL_DEFS = [
  {
    type: "function",
    function: {
      name: "bash",
      description: "\u5728\u6307\u5B9A\u5DE5\u4F5C\u76EE\u5F55\u6267\u884C shell \u547D\u4EE4\uFF0C\u8F93\u51FA stdout/stderr\u3002\u9002\u5408\u8FD0\u884C\u6784\u5EFA\u3001\u6D4B\u8BD5\u3001git \u64CD\u4F5C\u7B49\u3002",
      parameters: {
        type: "object",
        properties: {
          cmd: { type: "string", description: "\u8981\u6267\u884C\u7684 shell \u547D\u4EE4" },
          timeout: { type: "number", description: "\u8D85\u65F6\u79D2\u6570\uFF0C\u9ED8\u8BA4 30" }
        },
        required: ["cmd"]
      }
    }
  },
  {
    type: "function",
    function: {
      name: "read_file",
      description: "\u8BFB\u53D6\u6587\u672C\u6587\u4EF6\u5185\u5BB9\uFF08\u6700\u591A 5000 \u884C\uFF09\u3002",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "\u6587\u4EF6\u8DEF\u5F84" },
          offset: { type: "number", description: "\u8D77\u59CB\u884C\u53F7\uFF081 \u5F00\u59CB\uFF09" },
          limit: { type: "number", description: "\u8BFB\u53D6\u884C\u6570\uFF0C\u9ED8\u8BA4 500" }
        },
        required: ["path"]
      }
    }
  },
  {
    type: "function",
    function: {
      name: "write_file",
      description: "\u8986\u76D6\u5199\u5165\u6587\u4EF6\uFF08\u4F1A\u521B\u5EFA\u76EE\u5F55\uFF09\u3002\u7528\u4E8E\u521B\u5EFA\u6216\u6574\u4F53\u91CD\u5199\u6587\u4EF6\u3002",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "\u6587\u4EF6\u8DEF\u5F84" },
          content: { type: "string", description: "\u6587\u4EF6\u5B8C\u6574\u5185\u5BB9" }
        },
        required: ["path", "content"]
      }
    }
  },
  {
    type: "function",
    function: {
      name: "edit_file",
      description: "\u5728\u6587\u4EF6\u4E2D\u7CBE\u786E\u66FF\u6362\u4E00\u6BB5\u6587\u672C\uFF08old \u5FC5\u987B\u552F\u4E00\uFF09\u3002\u7528\u4E8E\u4FEE\u6539\u5DF2\u6709\u6587\u4EF6\u3002",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "\u6587\u4EF6\u8DEF\u5F84" },
          old: { type: "string", description: "\u8981\u66FF\u6362\u7684\u539F\u6587" },
          new: { type: "string", description: "\u66FF\u6362\u540E\u7684\u5185\u5BB9" }
        },
        required: ["path", "old", "new"]
      }
    }
  },
  {
    type: "function",
    function: {
      name: "list_dir",
      description: "\u5217\u51FA\u76EE\u5F55\u5185\u5BB9\uFF08\u542B\u5927\u5C0F\u4E0E\u7C7B\u578B\uFF09\u3002",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "\u76EE\u5F55\u8DEF\u5F84\uFF0C\u9ED8\u8BA4\u5F53\u524D\u5DE5\u4F5C\u76EE\u5F55" }
        }
      }
    }
  },
  {
    type: "function",
    function: {
      name: "grep",
      description: "\u5728\u76EE\u5F55\u4E2D\u6309\u6B63\u5219\u641C\u7D22\u6587\u4EF6\u5185\u5BB9\uFF0C\u8FD4\u56DE\u5339\u914D\u7684\u6587\u4EF6\u4E0E\u884C\u53F7\u3002",
      parameters: {
        type: "object",
        properties: {
          pattern: { type: "string", description: "\u6B63\u5219\u8868\u8FBE\u5F0F" },
          path: { type: "string", description: "\u641C\u7D22\u76EE\u5F55\uFF0C\u9ED8\u8BA4\u5F53\u524D\u5DE5\u4F5C\u76EE\u5F55" },
          include: { type: "string", description: "\u6587\u4EF6\u901A\u914D\uFF0C\u5982 *.js" }
        },
        required: ["pattern"]
      }
    }
  },
  {
    type: "function",
    function: {
      name: "glob",
      description: "\u6309\u901A\u914D\u6A21\u5F0F\u67E5\u627E\u6587\u4EF6\u8DEF\u5F84\u3002",
      parameters: {
        type: "object",
        properties: {
          pattern: { type: "string", description: "\u901A\u914D\u6A21\u5F0F\uFF0C\u5982 **/*.js" },
          path: { type: "string", description: "\u641C\u7D22\u6839\u76EE\u5F55\uFF0C\u9ED8\u8BA4\u5F53\u524D\u5DE5\u4F5C\u76EE\u5F55" }
        },
        required: ["pattern"]
      }
    }
  },
  {
    type: "function",
    function: {
      name: "fetch_url",
      description: "\u6293\u53D6\u7F51\u9875\u5185\u5BB9\uFF08markdown \u7B80\u5316\u6587\u672C\uFF09\u3002\u7528\u4E8E\u67E5\u6587\u6863\u3001\u770B\u63A5\u53E3\u8FD4\u56DE\u7B49\u3002",
      parameters: {
        type: "object",
        properties: {
          url: { type: "string", description: "\u5B8C\u6574 URL" }
        },
        required: ["url"]
      }
    }
  },
  {
    type: "function",
    function: {
      name: "get_current_time",
      description: "\u83B7\u53D6\u5F53\u524D\u7CFB\u7EDF\u65F6\u95F4\u548C\u65E5\u671F\u3002\u7528\u6237\u8BE2\u95EE\u65F6\u95F4/\u65E5\u671F/\u4ECA\u5929\u662F\u51E0\u53F7\u65F6\u4F7F\u7528\u3002",
      parameters: {
        type: "object",
        properties: {
          format: { type: "string", description: "\u683C\u5F0F\uFF1Aiso / date / time / full\uFF0C\u9ED8\u8BA4 full" }
        }
      }
    }
  },
  {
    type: "function",
    function: {
      name: "calc",
      description: "\u5B89\u5168\u8BA1\u7B97\u6570\u5B66\u8868\u8FBE\u5F0F\uFF08\u65E0 eval\uFF0C\u4EC5\u652F\u6301 + - * / % \u548C\u62EC\u53F7\uFF09\u3002",
      parameters: {
        type: "object",
        properties: {
          expr: { type: "string", description: "\u6570\u5B66\u8868\u8FBE\u5F0F" }
        },
        required: ["expr"]
      }
    }
  }
];
function safeCalc(expr) {
  const s = String(expr).replace(/[^0-9+\-*/().%\s]/g, "");
  if (!/^[0-9+\-*/().%\s]+$/.test(s) || !s.trim()) return { error: "\u8868\u8FBE\u5F0F\u4E0D\u5408\u6CD5" };
  const fn = new Function(`return (${s})`);
  const v = fn();
  return { result: v };
}
async function executeTool(name, args2, cwd) {
  switch (name) {
    case "bash": {
      const timeout = Math.max(5, parseInt(args2.timeout || 30, 10) || 30) * 1e3;
      try {
        const out = execSync(args2.cmd, {
          cwd,
          encoding: "utf8",
          shell: "/bin/bash",
          timeout,
          stdio: ["pipe", "pipe", "pipe"]
        });
        return { stdout: out.slice(0, 3e4), exitCode: 0 };
      } catch (e) {
        return {
          stdout: String(e.stdout || "").slice(0, 3e4),
          stderr: String(e.stderr || e.message).slice(0, 1e4),
          exitCode: e.status ?? 1
        };
      }
    }
    case "read_file": {
      try {
        const p = path2.resolve(cwd, args2.path);
        const content = fs2.readFileSync(p, "utf8");
        const lines = content.split("\n");
        const offset = Math.max(1, parseInt(args2.offset || 1, 10) || 1);
        const limit = Math.min(5e3, parseInt(args2.limit || 500, 10) || 500);
        const slice = lines.slice(offset - 1, offset - 1 + limit);
        const out = slice.map((l, i) => `${offset + i}: ${l}`).join("\n");
        return {
          content: out,
          totalLines: lines.length,
          truncated: lines.length > offset - 1 + limit
        };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "write_file": {
      try {
        const p = path2.resolve(cwd, args2.path);
        fs2.mkdirSync(path2.dirname(p), { recursive: true });
        fs2.writeFileSync(p, String(args2.content ?? ""), "utf8");
        return { ok: true, path: p, bytes: String(args2.content ?? "").length };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "edit_file": {
      try {
        const p = path2.resolve(cwd, args2.path);
        const content = fs2.readFileSync(p, "utf8");
        const oldText = String(args2.old ?? "");
        if (!oldText) return { error: "old \u4E0D\u80FD\u4E3A\u7A7A" };
        const count = content.split(oldText).length - 1;
        if (count === 0) return { error: "\u672A\u627E\u5230\u8981\u66FF\u6362\u7684\u6587\u672C\uFF08old \u4E0D\u5339\u914D\uFF09" };
        if (count > 1) return { error: `old \u6587\u672C\u51FA\u73B0 ${count} \u6B21\uFF0C\u4E0D\u552F\u4E00\uFF0C\u8BF7\u5305\u542B\u66F4\u591A\u4E0A\u4E0B\u6587` };
        const next = content.replace(oldText, String(args2.new ?? ""));
        fs2.writeFileSync(p, next, "utf8");
        return { ok: true, path: p, replaced: 1 };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "list_dir": {
      try {
        const p = path2.resolve(cwd, args2.path || ".");
        const entries = fs2.readdirSync(p, { withFileTypes: true });
        return {
          path: p,
          entries: entries.slice(0, 500).map((e) => {
            let size = "";
            let isDir = e.isDirectory();
            if (e.isFile()) {
              try {
                size = fs2.statSync(path2.join(p, e.name)).size;
              } catch {
              }
            }
            return { name: e.name, type: isDir ? "dir" : "file", size };
          }),
          total: entries.length,
          truncated: entries.length > 500
        };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "grep": {
      try {
        const root = path2.resolve(cwd, args2.path || ".");
        const pattern = new RegExp(args2.pattern, "i");
        const include = args2.include ? new RegExp(args2.include.replace(/\*/g, ".*")) : null;
        const results = [];
        const walk = (dir, depth) => {
          if (depth > 6 || results.length >= 200) return;
          let entries;
          try {
            entries = fs2.readdirSync(dir, { withFileTypes: true });
          } catch {
            return;
          }
          for (const e of entries) {
            if (e.name.startsWith(".") || e.name === "node_modules" || e.name === ".git") continue;
            const full = path2.join(dir, e.name);
            if (e.isDirectory()) walk(full, depth + 1);
            else if (e.isFile() && (!include || include.test(e.name))) {
              try {
                const lines = fs2.readFileSync(full, "utf8").split("\n");
                for (let i = 0; i < lines.length; i++) {
                  if (pattern.test(lines[i])) {
                    results.push({ file: full, line: i + 1, text: lines[i].slice(0, 200) });
                    if (results.length >= 200) break;
                  }
                }
              } catch {
              }
            }
          }
        };
        walk(root, 0);
        return { matches: results, count: results.length };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "glob": {
      try {
        const root = path2.resolve(cwd, args2.path || ".");
        const pat = String(args2.pattern || "**/*");
        const re = new RegExp(
          "^" + pat.split("/").map((s) => s.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*\*/g, ".*").replace(/\*/g, "[^/]*")).join("/") + "$"
        );
        const results = [];
        const walk = (dir, depth) => {
          if (depth > 6 || results.length >= 500) return;
          let entries;
          try {
            entries = fs2.readdirSync(dir, { withFileTypes: true });
          } catch {
            return;
          }
          for (const e of entries) {
            if (e.name.startsWith(".") || e.name === "node_modules" || e.name === ".git") continue;
            const full = path2.join(dir, e.name);
            const rel = path2.relative(root, full);
            if (e.isDirectory()) walk(full, depth + 1);
            else if (re.test(rel)) results.push(rel);
          }
        };
        walk(root, 0);
        return { files: results, count: results.length };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "fetch_url": {
      try {
        const res = await fetch(args2.url, {
          headers: { "User-Agent": "ux-agent/1.0" },
          signal: AbortSignal.timeout(2e4)
        });
        const text = await res.text();
        return {
          status: res.status,
          contentType: res.headers.get("content-type") || "",
          body: text.replace(/<script[\s\S]*?<\/script>/gi, "").replace(/<style[\s\S]*?<\/style>/gi, "").replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").slice(0, 2e4)
        };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "get_current_time": {
      const d = /* @__PURE__ */ new Date();
      const p = (n) => String(n).padStart(2, "0");
      const full = `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
      const f = args2.format || "full";
      return {
        full,
        iso: d.toISOString(),
        date: `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`,
        time: `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`,
        weekday: ["\u65E5", "\u4E00", "\u4E8C", "\u4E09", "\u56DB", "\u4E94", "\u516D"][d.getDay()],
        format: f
      };
    }
    case "calc":
      return safeCalc(args2.expr);
    default:
      return { error: `\u672A\u77E5\u5DE5\u5177: ${name}` };
  }
}

// src/agents.js
var BASE_RULES = "\u4F60\u662F Uinxed AI Agent\uFF0C\u4E00\u4E2A\u8FD0\u884C\u5728\u7EC8\u7AEF\u91CC\u7684\u7F16\u7A0B\u52A9\u624B\u3002\u4F60\u62E5\u6709\u5DE5\u5177\u8C03\u7528\u80FD\u529B\uFF1A\u5F53\u4EFB\u52A1\u9700\u8981\u6267\u884C\u547D\u4EE4\u3001\u8BFB\u5199\u6587\u4EF6\u3001\u641C\u7D22\u4EE3\u7801\u3001\u8BBF\u95EE\u7F51\u7EDC\u65F6\uFF0C\u5FC5\u987B\u8C03\u7528\u5DE5\u5177\u5B8C\u6210\uFF0C\u800C\u4E0D\u662F\u51ED\u7A7A\u731C\u6D4B\u3002\u5F53\u524D\u73AF\u5883\u63D0\u4F9B\u4E86\u5DE5\u5177\uFF08\u5982 bash\u3001read_file\u3001write_file\u3001edit_file\u3001list_dir\u3001grep\u3001glob\u3001fetch_url\u3001calc\uFF09\uFF0C\u5177\u4F53\u5DE5\u5177\u5217\u8868\u548C\u8C03\u7528\u683C\u5F0F\u4F1A\u5728\u7CFB\u7EDF\u6D88\u606F\u7684[\u53EF\u7528\u5DE5\u5177\u4E0E\u8C03\u7528\u89C4\u5219]\u4E2D\u7ED9\u51FA\uFF0C\u8BF7\u4E25\u683C\u6309\u8BE5\u683C\u5F0F\u8F93\u51FA\u5DE5\u5177\u8C03\u7528\u3002\u8C03\u7528\u5DE5\u5177\u540E\u6839\u636E\u8FD4\u56DE\u7ED3\u679C\u7EE7\u7EED\uFF0C\u76F4\u5230\u4EFB\u52A1\u5B8C\u6210\u518D\u603B\u7ED3\u56DE\u590D\u3002\u56DE\u590D\u4FDD\u6301\u7B80\u6D01\uFF0C\u4E2D\u6587\u4E3A\u4E3B\uFF0C\u91CD\u8981\u4EE3\u7801\u7528 markdown \u4EE3\u7801\u5757\u5C55\u793A\u3002";
var AGENTS = {
  build: {
    id: "build",
    name: "build",
    role: "primary",
    desc: "\u9ED8\u8BA4 agent\uFF0C\u5B8C\u6574\u5DE5\u5177\u8BBF\u95EE\uFF0C\u9002\u5408\u5F00\u53D1\u5DE5\u4F5C",
    color: "green",
    prompt: BASE_RULES + "\u4F60\u62E5\u6709\u5168\u90E8\u5DE5\u5177\u6743\u9650\uFF08bash/\u8BFB\u5199\u6587\u4EF6/\u641C\u7D22/\u7F51\u7EDC/\u8BA1\u7B97\uFF09\uFF0C\u53EF\u4EE5\u81EA\u7531\u4FEE\u6539\u6587\u4EF6\u3001\u8FD0\u884C\u547D\u4EE4\u5B8C\u6210\u7F16\u7A0B\u4EFB\u52A1\u3002\u591A\u6B65\u4EFB\u52A1\uFF1A\u5148\u7528 list_dir/grep \u4E86\u89E3\u73B0\u72B6 \u2192 \u5FC5\u8981\u65F6\u8BFB\u6587\u4EF6 \u2192 \u4FEE\u6539 \u2192 \u8FD0\u884C\u9A8C\u8BC1\u3002",
    tools: "*"
  },
  plan: {
    id: "plan",
    name: "plan",
    role: "primary",
    desc: "\u53EA\u8BFB agent\uFF0C\u5206\u6790\u4EE3\u7801\u4E0E\u5236\u5B9A\u65B9\u6848\uFF0C\u4E0D\u505A\u4FEE\u6539",
    color: "cyan",
    prompt: BASE_RULES + "\u4F60\u662F\u89C4\u5212\u4E0E\u5206\u6790 agent\uFF0C\u53EA\u8BFB\u6A21\u5F0F\uFF1A\u7981\u6B62 write_file/edit_file/bash \u7B49\u5199\u64CD\u4F5C\uFF0C\u53EA\u7528 read_file/list_dir/grep/glob/fetch_url/calc \u8C03\u7814\uFF0C\u8F93\u51FA\u5206\u6790\u7ED3\u8BBA\u6216\u5B9E\u65BD\u8BA1\u5212\uFF0C\u4E0D\u4FEE\u6539\u4EFB\u4F55\u6587\u4EF6\u3002",
    tools: ["read_file", "list_dir", "grep", "glob", "fetch_url", "calc"]
  },
  explorer: {
    id: "explorer",
    name: "explorer",
    role: "subagent",
    desc: "\u5FEB\u901F\u53EA\u8BFB\u63A2\u7D22\u4EE3\u7801\u5E93\uFF0C\u9002\u5408\u88AB @ \u59D4\u6258\u67E5\u627E\u6587\u4EF6/\u7ED3\u6784",
    color: "yellow",
    prompt: "\u4F60\u662F\u63A2\u7D22\u5B50\u4EE3\u7406\uFF0C\u53EA\u8BFB\u3002\u5FEB\u901F\u5B9A\u4F4D\u6587\u4EF6\u3001\u51FD\u6570\u3001\u7ED3\u6784\uFF0C\u56DE\u7B54\u8981\u7B80\u77ED\uFF08\u6587\u4EF6\u540D+\u884C\u53F7\uFF09\u3002\u7981\u6B62\u4FEE\u6539\u4EFB\u4F55\u6587\u4EF6\u3002\u82E5\u9700\u8981\u641C\u7D22\u8BF7\u8C03\u7528 grep/glob \u5DE5\u5177\u3002",
    tools: ["read_file", "list_dir", "grep", "glob"]
  },
  general: {
    id: "general",
    name: "general",
    role: "subagent",
    desc: "\u901A\u7528\u5B50\u4EE3\u7406\uFF0C\u5904\u7406\u591A\u6B65\u72EC\u7ACB\u4EFB\u52A1",
    color: "magenta",
    prompt: BASE_RULES + "\u4F60\u662F\u901A\u7528\u5B50\u4EE3\u7406\uFF0C\u53EF\u6267\u884C\u591A\u6B65\u4EFB\u52A1\u5E76\u8FD4\u56DE\u7ED3\u679C\u6458\u8981\u3002\u72EC\u7ACB\u5B8C\u6210\u4EFB\u52A1\uFF0C\u6700\u540E\u7ED9\u51FA\u7ED3\u8BBA\u3002",
    tools: "*"
  }
};
function getAgent(id) {
  return AGENTS[id] || AGENTS.build;
}
function primaryAgents() {
  return Object.values(AGENTS).filter((a) => a.role === "primary");
}
function subAgents() {
  return Object.values(AGENTS).filter((a) => a.role === "subagent");
}
function filterTools(defs, agent) {
  if (!agent) return [];
  if (agent.tools === "*") return defs;
  const allow = new Set(agent.tools);
  return defs.filter((d) => allow.has(d.function.name));
}

// src/App.jsx
init_config();
import path3 from "node:path";
var fmtTime = (ts) => {
  const d = new Date(ts);
  const p = (n) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};
var COMMANDS = [
  { cmd: "/help", desc: "\u663E\u793A\u6240\u6709\u547D\u4EE4" },
  { cmd: "/connect", desc: "\u63A5\u5165\u63D0\u4F9B\u5546\uFF08\u81EA\u5B9A\u4E49 API \u670D\u52A1\uFF09" },
  { cmd: "/provider", desc: "\u5207\u6362\u63D0\u4F9B\u5546\uFF08/provider <name>\uFF09" },
  { cmd: "/key", desc: "\u8BBE\u7F6E\u5F53\u524D\u63D0\u4F9B\u5546 API Key\uFF08/key <sk-xxx>\uFF09" },
  { cmd: "/model", desc: "\u5207\u6362\u6A21\u578B\uFF08/model <id>\uFF09" },
  { cmd: "/thinking", desc: "\u5F00\u542F/\u5173\u95ED thinking \u5C55\u793A" },
  { cmd: "/agent", desc: "\u5217\u51FA/\u5207\u6362 agent\uFF08Tab \u4E5F\u5207\u6362\uFF09" },
  { cmd: "/quota", desc: "\u67E5\u8BE2\u672C\u5730\u7F51\u5173\u4F59\u989D\uFF08\u4EC5\u672C\u5730\u63D0\u4F9B\u5546\uFF09" },
  { cmd: "/cd", desc: "\u5207\u6362\u5DE5\u4F5C\u76EE\u5F55" },
  { cmd: "/pwd", desc: "\u663E\u793A\u5DE5\u4F5C\u76EE\u5F55" },
  { cmd: "/new", desc: "\u6E05\u7A7A\u5F53\u524D\u4F1A\u8BDD" },
  { cmd: "/clear", desc: "\u6E05\u7A7A\u672C\u5730\u5386\u53F2" },
  { cmd: "/exit", desc: "\u9000\u51FA" }
];
function CommandPalette({ input, onPick }) {
  const q = input.slice(1).toLowerCase();
  const matches = COMMANDS.filter((c) => c.cmd.includes(q));
  if (!matches.length) return null;
  return /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "column", marginBottom: 1 }, matches.map((c) => /* @__PURE__ */ React3.createElement(Text3, { key: c.cmd, color: "cyan" }, "  ", c.cmd, " ", /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "\u2014 ", c.desc))));
}
function sanitizeText(s) {
  return String(s == null ? "" : s).replace(/\u001b\[[0-9;]*m/g, "").replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, "").replace(/\u200b/g, "");
}
function estHeight(content, width) {
  const lines = sanitizeText(content).split("\n");
  let h = 0;
  let inCode = false;
  for (const l of lines) {
    if (l.trim().startsWith("```")) {
      h += 2;
      inCode = !inCode;
      continue;
    }
    const wrapped = Math.max(1, Math.ceil(l.length / Math.max(width - 8, 20)));
    h += wrapped;
  }
  return h + 2;
}
var MessageBoundary = class extends React3.Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
  }
  static getDerivedStateFromError(error) {
    return { error };
  }
  componentDidCatch(error) {
    this.props.onError?.(error);
  }
  render() {
    if (this.state.error) {
      return /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "column", marginBottom: 1 }, /* @__PURE__ */ React3.createElement(Text3, { bold: true, color: "red" }, "\u26A0 \u6D88\u606F\u6E32\u67D3\u5931\u8D25\uFF08\u5DF2\u5FFD\u7565\uFF09"), /* @__PURE__ */ React3.createElement(Text3, { dimColor: true, wrap: "wrap" }, String(this.state.error.message || this.state.error).slice(0, 200)));
    }
    return this.props.children;
  }
};
function MessageItem({ m, width, expandedThinking, onToggleThinking }) {
  const color = m.role === "user" ? "green" : m.tool ? "yellow" : "magenta";
  const prefix = m.role === "user" ? "\u276F" : m.tool ? "\u2699" : "\u25C6";
  const isToolResult = m.role === "tool_result";
  const content = sanitizeText(m.content);
  const reasoning = sanitizeText(m.reasoning);
  return /* @__PURE__ */ React3.createElement(MessageBoundary, null, /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "column", marginBottom: 1, width: Math.max(width - 2, 20) }, /* @__PURE__ */ React3.createElement(Text3, null, /* @__PURE__ */ React3.createElement(Text3, { bold: true, color }, prefix), " ", /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, fmtTime(m.time)), m.agentName && /* @__PURE__ */ React3.createElement(Text3, { color }, " [", m.agentName, "]"), m.toolName && /* @__PURE__ */ React3.createElement(Text3, { color: "yellow" }, " \u2699", m.toolName)), reasoning ? /* @__PURE__ */ React3.createElement(
    ThinkingBlock,
    {
      reasoning,
      expanded: expandedThinking,
      onToggle: onToggleThinking
    }
  ) : null, isToolResult ? /* @__PURE__ */ React3.createElement(Text3, { color: "gray", wrap: "wrap" }, content) : /* @__PURE__ */ React3.createElement(Markdown, { content, width: Math.max(width - 4, 16) })));
}
function ConnectModal({ provider, onSubmit, onCancel }) {
  const [step, setStep] = useState(0);
  const [name, setName] = useState(provider?.name || "");
  const [baseUrl, setBaseUrl2] = useState(provider?.baseUrl || "");
  const [models, setModels] = useState(provider?.models?.join(",") || "");
  const [apiKey, setApiKey2] = useState(provider?.apiKey || "");
  const [err, setErr] = useState("");
  const submit = (v) => {
    if (step === 0) {
      if (!name.trim() || !baseUrl.trim()) {
        setErr("\u540D\u79F0\u548C\u5730\u5740\u4E0D\u80FD\u4E3A\u7A7A");
        return;
      }
      setErr("");
      setStep(1);
    } else if (step === 1) {
      setErr("");
      setStep(2);
    } else {
      onSubmit({
        name: name.trim(),
        baseUrl: baseUrl.trim(),
        models: models.split(",").map((s) => s.trim()).filter(Boolean),
        apiKey: apiKey.trim() || null,
        id: provider?.id
      });
    }
  };
  return /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "column", borderStyle: "round", borderColor: "cyan", paddingX: 2, paddingY: 1, marginBottom: 1 }, /* @__PURE__ */ React3.createElement(Text3, { bold: true, color: "cyan" }, "\u8FDE\u63A5\u63D0\u4F9B\u5546 (", step + 1, "/3)"), step === 0 && /* @__PURE__ */ React3.createElement(React3.Fragment, null, /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "\u540D\u79F0:"), /* @__PURE__ */ React3.createElement(TextInput, { value: name, onChange: setName, onSubmit: () => submit(), placeholder: "\u5982: \u6211\u7684\u4E2D\u8F6C\u7AD9" }), /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "\u63A5\u53E3\u5730\u5740 (OpenAI \u517C\u5BB9 base, \u5982 https://xxx.com/v1):"), /* @__PURE__ */ React3.createElement(TextInput, { value: baseUrl, onChange: setBaseUrl2, onSubmit: () => submit(), placeholder: "https://api.example.com/v1" })), step === 1 && /* @__PURE__ */ React3.createElement(React3.Fragment, null, /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "\u6A21\u578B\u5217\u8868 (\u9017\u53F7\u5206\u9694):"), /* @__PURE__ */ React3.createElement(TextInput, { value: models, onChange: setModels, onSubmit: () => submit(), placeholder: "model-a, model-b" })), step === 2 && /* @__PURE__ */ React3.createElement(React3.Fragment, null, /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "API Key (\u53EF\u7559\u7A7A):"), /* @__PURE__ */ React3.createElement(TextInput, { value: apiKey, onChange: setApiKey2, onSubmit: () => submit(), placeholder: "sk-xxx" }), /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "Enter \u4FDD\u5B58 \xB7 Esc \u53D6\u6D88")), err && /* @__PURE__ */ React3.createElement(Text3, { color: "red" }, err));
}
function App() {
  const { exit } = useApp();
  const { stdout } = useStdout();
  const [cfg2, setCfg] = useState(() => loadConfig());
  const [messages, setMessages] = useState([]);
  const [conversation, setConversation] = useState(() => loadConfig().conversation || []);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [profile, setProfile] = useState(null);
  const [status, setStatus] = useState("\u5C31\u7EEA");
  const [agentId, setAgentId] = useState("build");
  const [cwd, setCwd] = useState(() => process.cwd());
  const [mode, setMode] = useState("chat");
  const [loginInput, setLoginInput] = useState("");
  const [loginErr, setLoginErr] = useState("");
  const [connectProvider, setConnectProvider] = useState(null);
  const [modelOptions, setModelOptions] = useState([]);
  const [modelPick, setModelPick] = useState("");
  const [showCommands, setShowCommands] = useState(false);
  const [scrollOffset, setScrollOffset] = useState(0);
  const [expandedThinking, setExpandedThinking] = useState(false);
  const [thinkingCache, setThinkingCache] = useState({});
  const [streaming, setStreaming] = useState(null);
  const aborter = useRef(null);
  const toastTimer = useRef(null);
  const WIDTH = stdout.columns || 100;
  const HEIGHT = stdout.rows || 30;
  const MSG_HEIGHT = Math.max(HEIGHT - 11, 8);
  const provider = getActiveProvider();
  const agent = getAgent(agentId);
  const clearScreen = useCallback(() => {
    try {
      process.stdout.write("\x1B[2J\x1B[H");
    } catch {
    }
  }, []);
  useEffect(() => {
    setScrollOffset(0);
  }, [messages.length, busy, streaming]);
  const toast = useCallback((msg) => {
    setStatus(msg);
    clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setStatus("\u5C31\u7EEA"), 3e3);
  }, []);
  const persist = useCallback((msgs, conv) => {
    saveConfig({
      history: msgs.filter((m) => m.role === "user" || m.role === "assistant").map((m) => ({ role: m.role, content: m.content, time: m.time, reasoning: m.reasoning })).slice(-200),
      conversation: (conv || conversation).slice(-200)
    });
  }, [conversation]);
  const refreshProfile = useCallback(async () => {
    try {
      return await getProfile();
    } catch {
      return null;
    }
  }, []);
  useEffect(() => {
    clearScreen();
    const hist = loadConfig().history;
    if (hist.length) {
      setMessages(hist.map((m) => ({ ...m, time: m.time || Date.now() })));
    } else {
      setMessages([{
        role: "assistant",
        content: "\u4F60\u597D\uFF0C\u6211\u662F **Uinxed AI Agent**\u3002\u652F\u6301\u591A\u63D0\u4F9B\u5546\uFF08`/provider`\uFF09\u3001\u5DE5\u5177\u8C03\u7528\u3001thinking \u5C55\u793A\u3002\u8F93\u5165 `/` \u67E5\u770B\u547D\u4EE4\u3002",
        time: Date.now()
      }]);
    }
    const p = getActiveProvider();
    if (p?.apiKey) {
      refreshProfile().then((u) => {
        setStatus(u ? `\u5DF2\u767B\u5F55 ${u.username}${u.unlimited ? "\uFF08\u65E0\u9650\u989D\u5EA6\uFF09" : `\uFF0C\u4F59\u989D \xA5${(u.quota || 0).toFixed(2)}`}` : `\u5DF2\u8FDE\u63A5 ${p.name}`);
      });
    } else {
      setStatus(`${p.name} \u672A\u8BBE\u7F6E Key \xB7 \u63D0\u4F9B\u5546 ${p.name}\uFF08${p.baseUrl}\uFF09`);
    }
  }, [refreshProfile, clearScreen]);
  const runAgent = useCallback(
    async (userText, targetAgent, isSub = false) => {
      const active = isSub ? getAgent(targetAgent) : agent;
      const toolDefs = filterTools(TOOL_DEFS, active);
      const currentProvider = getActiveProvider();
      if (!currentProvider.apiKey) {
        setMode("login");
        setStatus(`\u8BF7\u4E3A ${currentProvider.name} \u8F93\u5165 API Key`);
        setBusy(false);
        return;
      }
      const msgs = [
        { role: "system", content: active.prompt },
        ...conversation.slice(-20),
        { role: "user", content: userText }
      ];
      const streamId = Date.now() + "-" + Math.random().toString(36).slice(2, 6);
      const thinkingId = `think-${streamId}`;
      let streamAcc = { content: "", reasoning: "" };
      let lastFlush = 0;
      let conversationAdded = false;
      for (let step = 0; step < 12; step++) {
        setStatus(`${active.name} \u601D\u8003\u4E2D\u2026 (${step + 1}/12)`);
        let finalRes = null;
        aborter.current = new AbortController();
        try {
          for await (const chunk of chatStream(msgs, {
            tools: toolDefs,
            signal: aborter.current.signal
          })) {
            if (chunk.reasoning) {
              streamAcc.reasoning += chunk.reasoning;
              setThinkingCache((c) => ({ ...c, [thinkingId]: streamAcc.reasoning }));
            }
            if (chunk.content) {
              streamAcc.content += chunk.content;
              const now = Date.now();
              if (now - lastFlush > 80 || chunk.content.includes("\n")) {
                setStreaming(null);
                setStreaming({ id: streamId, role: "assistant", content: streamAcc.content, reasoning: streamAcc.reasoning, time: Date.now() });
                lastFlush = now;
              }
            }
            if (chunk.finishReason || chunk.done) finalRes = chunk;
          }
          if (streamAcc.content || streamAcc.reasoning) {
            setStreaming({ id: streamId, role: "assistant", content: streamAcc.content, reasoning: streamAcc.reasoning, time: Date.now() });
          }
        } catch (e) {
          if (e.name === "AbortError") {
            setStatus("\u5DF2\u53D6\u6D88");
            setStreaming(null);
            setBusy(false);
            return;
          }
          setMessages((m) => [...m, {
            role: "assistant",
            agentName: active.name,
            content: e instanceof ApiError && e.type === "content_filter" ? "\u8BDD\u9898\u88AB\u8FC7\u6EE4\u3002" : `\u9519\u8BEF: ${e.message}`,
            time: Date.now()
          }]);
          setStatus("\u51FA\u9519");
          setStreaming(null);
          setBusy(false);
          return;
        }
        const toolCalls = finalRes?.toolCalls || [];
        const reasoning = thinkingCache[thinkingId] || "";
        if (toolCalls.length) {
          const assistantMsg = { role: "assistant", content: "", tool_calls: [] };
          assistantMsg.reasoning_content = streamAcc.reasoning || "";
          const toolResults = [];
          for (let ti = 0; ti < toolCalls.length; ti++) {
            const tc = toolCalls[ti];
            if (!tc.id) tc.id = `call_${Date.now()}_${ti}`;
            assistantMsg.tool_calls.push(tc);
            let parsed = {};
            try {
              parsed = JSON.parse(tc.function.arguments || "{}");
            } catch {
            }
            setMessages((m) => [...m, {
              role: "tool",
              agentName: active.name,
              toolName: tc.function.name,
              content: `\u2699 ${tc.function.name}(${JSON.stringify(parsed).slice(0, 100)})`,
              time: Date.now()
            }]);
            setStatus(`\u6267\u884C ${tc.function.name}\u2026`);
            if (tc.function.name === "bash") clearScreen();
            const result = await executeTool(tc.function.name, parsed, cwd);
            const resultText = JSON.stringify(result, null, 2).slice(0, 12e3);
            setMessages((m) => [...m, {
              role: "tool_result",
              agentName: active.name,
              toolName: tc.function.name,
              content: resultText.slice(0, 2500),
              time: Date.now()
            }]);
            toolResults.push({ role: "tool", tool_call_id: tc.id, content: resultText });
          }
          msgs.push(assistantMsg);
          for (const tr of toolResults) msgs.push(tr);
          if (!conversationAdded) {
            conversationAdded = true;
            setConversation((conv) => {
              const next = [...conv, { role: "user", content: userText }].concat(
                assistantMsg,
                toolResults
              );
              setMessages((m) => {
                persist(m, next);
                return m;
              });
              return next;
            });
          } else {
            setConversation((conv) => {
              const next = [...conv, assistantMsg, ...toolResults];
              setMessages((m) => {
                persist(m, next);
                return m;
              });
              return next;
            });
          }
          setStreaming(null);
          setThinkingCache((c) => {
            const n = { ...c };
            delete n[thinkingId];
            return n;
          });
          streamAcc = { content: "", reasoning: "" };
          continue;
        }
        const content = streamAcc.content || finalRes?.content || "(\u7A7A\u56DE\u590D)";
        const finalMsg = {
          role: "assistant",
          agentName: active.name,
          content,
          reasoning: streamAcc.reasoning || void 0,
          time: Date.now(),
          usage: finalRes?.usage
        };
        const apiFinal = {
          role: "assistant",
          content,
          reasoning_content: streamAcc.reasoning || ""
        };
        setConversation((conv) => {
          const next = conversationAdded ? [...conv, apiFinal] : [...conv, { role: "user", content: userText }, apiFinal];
          persist([...messages, finalMsg], next);
          return next;
        });
        setMessages((m) => [...m, finalMsg]);
        setStreaming(null);
        setThinkingCache((c) => {
          const n = { ...c };
          delete n[thinkingId];
          return n;
        });
        const u = finalRes?.usage || {};
        setStatus(`${active.name} \u5B8C\u6210 \xB7 ${u.prompt_tokens || 0}/${u.completion_tokens || 0} tokens${isSub ? "\uFF08\u5B50\u4EFB\u52A1\uFF09" : ""}`);
        setBusy(false);
        refreshProfile();
        return;
      }
      setMessages((m) => [...m, { role: "assistant", agentName: active.name, content: "\u6B65\u9AA4\u6570\u5DF2\u8FBE\u4E0A\u9650\u3002", time: Date.now() }]);
      setBusy(false);
      setStreaming(null);
      setStatus("\u8FBE\u5230\u6B65\u9AA4\u4E0A\u9650");
    },
    [messages, conversation, cwd, persist, agent, refreshProfile, streaming, clearScreen]
  );
  const runCommand = useCallback(async (cmd) => {
    const [name, ...rest] = cmd.trim().split(/\s+/);
    const arg = rest.join(" ");
    switch (name) {
      case "/help":
        setMessages((m) => [...m, {
          role: "assistant",
          content: "**\u53EF\u7528\u547D\u4EE4**\n" + COMMANDS.map((c) => `- \`${c.cmd}\` \u2014 ${c.desc}`).join("\n"),
          time: Date.now()
        }]);
        break;
      case "/connect":
        setConnectProvider(null);
        setMode("connect");
        break;
      case "/provider": {
        const providers = loadConfig().providers;
        if (!arg) {
          setMessages((m) => [...m, {
            role: "assistant",
            content: "**\u63D0\u4F9B\u5546**\n" + providers.map(
              (p) => `- \`${p.id}\` ${p.name}${p.id === loadConfig().activeProvider ? "\uFF08\u5F53\u524D\uFF09" : ""} \xB7 ${p.baseUrl}`
            ).join("\n") + "\n\n\u5207\u6362: `/provider <id>` \xB7 \u63A5\u5165\u65B0\u7684: `/connect`",
            time: Date.now()
          }]);
        } else {
          const target = providers.find((p) => p.id === arg);
          if (target) {
            const next = setActiveProvider(arg);
            if (next) {
              setCfg(next);
              setStatus(`\u5DF2\u5207\u6362\u63D0\u4F9B\u5546: ${target.name}\uFF08${next.model}\uFF09`);
              if (!target.apiKey) setMode("login");
            }
          } else {
            setStatus(`\u6CA1\u6709\u63D0\u4F9B\u5546: ${arg}\uFF08/connect \u63A5\u5165\uFF09`);
          }
        }
        break;
      }
      case "/key":
        if (arg) {
          const p = getActiveProvider();
          setProviderApiKey(p.id, arg.trim());
          setCfg(loadConfig());
          setStatus("\u9A8C\u8BC1 Key\u2026");
          const ok = await checkApiKey(arg.trim(), p.id);
          if (ok) {
            refreshProfile().then(
              (u) => setStatus(u ? `\u5DF2\u767B\u5F55 ${u.username}${u.unlimited ? "\uFF08\u65E0\u9650\u989D\u5EA6\uFF09" : `\uFF0C\u4F59\u989D \xA5${(u.quota || 0).toFixed(2)}`}` : "Key \u6709\u6548")
            );
          } else setStatus("Key \u65E0\u6548");
        } else {
          const p = getActiveProvider();
          const key = p.apiKey;
          setStatus(key ? `${p.name} Key: ${key.slice(0, 10)}\u2026\uFF08/key <\u65B0Key>\uFF09` : `${p.name} \u672A\u8BBE\u7F6E Key`);
        }
        break;
      case "/thinking":
        if (arg === "on" || arg === "true" || arg === "1") {
          setThinking(true);
          setStatus("thinking \u663E\u793A\u5F00\u542F");
        } else if (arg === "off" || arg === "false" || arg === "0") {
          setThinking(false);
          setStatus("thinking \u663E\u793A\u5173\u95ED");
        } else {
          const cur = loadConfig().thinking;
          setThinking(!cur);
          setStatus(`thinking \u663E\u793A: ${!cur ? "\u5F00" : "\u5173"}`);
        }
        break;
      case "/model":
        if (!arg) {
          const p = getActiveProvider();
          setModelOptions(p.models || []);
          setModelPick("");
          setMode("model");
          setStatus(`\u5F53\u524D\u6A21\u578B: ${loadConfig().model}`);
        } else {
          saveConfig({ model: arg });
          setCfg(loadConfig());
          setStatus(`\u6A21\u578B \u2192 ${arg}`);
        }
        break;
      case "/quota":
        setStatus("\u67E5\u8BE2\u4E2D\u2026");
        try {
          const u = await getProfile();
          setProfile(u);
          setStatus(u ? `${u.username} \xB7 \u4F59\u989D \xA5${(u.quota || 0).toFixed(2)} \xB7 ${u.groupName} \xD7${u.groupRate}${u.unlimited ? "\uFF08\u65E0\u9650\uFF09" : ""}` : "\u8BE5\u63D0\u4F9B\u5546\u65E0\u8D26\u6237\u63A5\u53E3");
        } catch (e) {
          setStatus(`\u67E5\u8BE2\u5931\u8D25: ${e.message}`);
        }
        break;
      case "/cd":
        if (!arg) setStatus(`\u5F53\u524D\u76EE\u5F55: ${cwd}`);
        else {
          const next = path3.resolve(cwd, arg);
          try {
            const fs3 = await import("node:fs");
            if (fs3.statSync(next).isDirectory()) {
              setCwd(next);
              setStatus(`\u5DF2\u5207\u6362\u5230: ${next}`);
            } else setStatus("\u4E0D\u662F\u76EE\u5F55");
          } catch {
            setStatus(`\u76EE\u5F55\u4E0D\u5B58\u5728: ${next}`);
          }
        }
        break;
      case "/pwd":
        setStatus(`\u5DE5\u4F5C\u76EE\u5F55: ${cwd}`);
        break;
      case "/new":
        clearScreen();
        setMessages([{ role: "assistant", content: "\u65B0\u4F1A\u8BDD\u5DF2\u5F00\u59CB\u3002", time: Date.now() }]);
        setConversation([]);
        saveConfig({ history: [], conversation: [] });
        break;
      case "/clear":
        clearScreen();
        setMessages([{ role: "assistant", content: "\u5386\u53F2\u5DF2\u6E05\u7A7A\u3002", time: Date.now() }]);
        setConversation([]);
        saveConfig({ history: [], conversation: [] });
        break;
      case "/exit":
        exit();
        break;
      default:
        setStatus(`\u672A\u77E5\u547D\u4EE4: ${name}\uFF08/help\uFF09`);
    }
  }, [cwd, exit, refreshProfile, clearScreen]);
  const onSubmit = (value) => {
    if (busy) {
      toast("\u8BF7\u7B49\u5F85\u5F53\u524D\u4EFB\u52A1\u5B8C\u6210");
      return;
    }
    const text = value.trim();
    if (!text) return;
    setShowCommands(false);
    if (text.startsWith("/")) {
      runCommand(text);
      setInput("");
      return;
    }
    const p = getActiveProvider();
    if (!p.apiKey) {
      setMode("login");
      setStatus(`\u8BF7\u4E3A ${p.name} \u8F93\u5165 API Key`);
      return;
    }
    const sub = subAgents().find((a) => text.startsWith(`@${a.name}`));
    if (sub) {
      const rest = text.replace(new RegExp(`^@${sub.name}\\s*`), "");
      setMessages((m) => [...m, { role: "user", content: text, time: Date.now() }]);
      setInput("");
      setBusy(true);
      clearScreen();
      runAgent(rest || "\u5E2E\u6211\u5B8C\u6210\u8FD9\u4E2A\u4EFB\u52A1", sub.name, true);
      return;
    }
    setMessages((m) => [...m, { role: "user", content: text, time: Date.now() }]);
    setInput("");
    setBusy(true);
    runAgent(text, agentId, false);
  };
  const onLoginSubmit = async (value) => {
    const key = value.trim();
    if (!key) {
      setLoginErr("Key \u4E0D\u80FD\u4E3A\u7A7A");
      return;
    }
    setLoginErr("\u9A8C\u8BC1\u4E2D\u2026");
    const p = getActiveProvider();
    const ok = await checkApiKey(key, p.id);
    if (ok) {
      setProviderApiKey(p.id, key);
      setCfg(loadConfig());
      setMode("chat");
      setLoginErr("");
      setStatus(`\u5DF2\u4FDD\u5B58 ${p.name} Key`);
      refreshProfile();
    } else setLoginErr("Key \u65E0\u6548\uFF0C\u8BF7\u68C0\u67E5\uFF08\u53EF\u7559\u7A7A\u8DF3\u8FC7\u9A8C\u8BC1\uFF09");
  };
  const onConnectSubmit = (data) => {
    const p = upsertProvider(data);
    setCfg(loadConfig());
    setMode("chat");
    setStatus(`\u5DF2\u63A5\u5165\u63D0\u4F9B\u5546: ${p.name}`);
    if (data.apiKey) {
      setProviderApiKey(p.id, data.apiKey);
      setCfg(loadConfig());
    }
  };
  const onModelSubmit = (id) => {
    if (id) {
      saveConfig({ model: id });
      setCfg(loadConfig());
      setStatus(`\u6A21\u578B \u2192 ${id}`);
    }
    setMode("chat");
  };
  useInput((_input, key) => {
    if (key.escape) {
      if (mode !== "chat") {
        setMode("chat");
        setConnectProvider(null);
        return;
      }
    }
    if (mode !== "chat") return;
    if (key.tab) {
      const primaries = primaryAgents();
      const idx = primaries.findIndex((a) => a.id === agentId);
      const next = primaries[(idx + 1) % primaries.length];
      setAgentId(next.id);
      clearScreen();
      setStatus(`agent \u2192 ${next.name}`);
    }
    if (key.ctrl && (_input === "t" || _input === "T")) {
      setExpandedThinking((e) => !e);
    }
    if (key.upArrow) setScrollOffset((o) => Math.min(o + 3, 1e4));
    if (key.downArrow) setScrollOffset((o) => Math.max(o - 3, 0));
    if (key.pageUp) setScrollOffset((o) => o + MSG_HEIGHT);
    if (key.pageDown) setScrollOffset((o) => Math.max(o - MSG_HEIGHT, 0));
  });
  const itemHeights = messages.map((m) => estHeight(m.content, WIDTH) + 1);
  let startIdx = messages.length;
  let used = 0;
  if (scrollOffset === 0) {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (used + itemHeights[i] > MSG_HEIGHT) break;
      used += itemHeights[i];
      startIdx = i;
    }
  } else {
    let offset = scrollOffset;
    for (let i = messages.length - 1; i >= 0; i--) {
      if (offset > 0) {
        offset -= itemHeights[i];
        continue;
      }
      if (used + itemHeights[i] > MSG_HEIGHT) break;
      used += itemHeights[i];
      startIdx = i;
    }
  }
  if (startIdx < 0) startIdx = 0;
  const visible = messages.slice(startIdx);
  const agentColor = agent.color;
  return /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "column" }, /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "row", flexShrink: 0 }, /* @__PURE__ */ React3.createElement(Text3, { bold: true, color: "cyan" }, "\u25C6 Uinxed"), /* @__PURE__ */ React3.createElement(Text3, { color: agentColor, bold: true }, " ", agent.name), /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, " \xB7 ", provider.name), /* @__PURE__ */ React3.createElement(Text3, { bold: true }, " \xB7 ", loadConfig().model), /* @__PURE__ */ React3.createElement(Text3, { color: profile ? "green" : "yellow" }, " \xB7 ", profile ? profile.username : provider.apiKey ? "\u5DF2\u8FDE\u63A5" : "\u672A\u767B\u5F55"), profile && !profile.unlimited && /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, " \xB7 \xA5", (profile.quota || 0).toFixed(2)), /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, " \xB7 ", status)), /* @__PURE__ */ React3.createElement(Box3, { flexGrow: 1, flexShrink: 1, height: MSG_HEIGHT, flexDirection: "column", borderStyle: "round", borderColor: "gray", paddingX: 1, overflow: "hidden" }, visible.map((m, i) => /* @__PURE__ */ React3.createElement(
    MessageItem,
    {
      key: m.time + "-" + i,
      m,
      width: WIDTH,
      expandedThinking,
      onToggleThinking: () => setExpandedThinking((e) => !e)
    }
  )), streaming && /* @__PURE__ */ React3.createElement(
    MessageItem,
    {
      m: { role: "assistant", content: streaming.content, reasoning: streaming.reasoning, time: Date.now() },
      width: WIDTH,
      expandedThinking,
      onToggleThinking: () => setExpandedThinking((e) => !e)
    }
  ), !streaming && Object.keys(thinkingCache).length > 0 && /* @__PURE__ */ React3.createElement(
    MessageItem,
    {
      m: { role: "assistant", content: "\u25C8 \u63A8\u7406\u4E2D\u2026", time: Date.now() },
      width: WIDTH,
      expandedThinking,
      onToggleThinking: () => setExpandedThinking((e) => !e)
    }
  ), busy && !streaming && /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "\u2026"), scrollOffset > 0 && /* @__PURE__ */ React3.createElement(Text3, { dimColor: true, backgroundColor: "#222", bold: true }, " \u2191 \u4E0A\u7FFB\u4E2D\uFF08\u2193 \u56DE\u5E95\u90E8\uFF09")), mode === "connect" && /* @__PURE__ */ React3.createElement(ConnectModal, { provider: connectProvider, onSubmit: onConnectSubmit, onCancel: () => setMode("chat") }), mode === "login" && /* @__PURE__ */ React3.createElement(Box3, { borderStyle: "round", borderColor: "yellow", paddingX: 2, paddingY: 1, marginBottom: 1 }, /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "column" }, /* @__PURE__ */ React3.createElement(Text3, { bold: true, color: "yellow" }, "\u4E3A ", provider.name, " \u8F93\u5165 API Key:"), /* @__PURE__ */ React3.createElement(TextInput, { value: loginInput, onChange: setLoginInput, onSubmit: onLoginSubmit }), loginErr && /* @__PURE__ */ React3.createElement(Text3, { color: "red" }, loginErr), /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "Enter \u4FDD\u5B58 \xB7 Esc \u53D6\u6D88"))), showCommands && input.startsWith("/") && /* @__PURE__ */ React3.createElement(CommandPalette, { input }), /* @__PURE__ */ React3.createElement(Box3, { borderStyle: "round", borderColor: "gray", paddingX: 1, flexShrink: 0 }, mode === "chat" && /* @__PURE__ */ React3.createElement(
    TextInput,
    {
      value: input,
      onChange: (v) => {
        setInput(v);
        setShowCommands(v.startsWith("/"));
      },
      onSubmit,
      placeholder: busy ? "\u4EFB\u52A1\u6267\u884C\u4E2D\u2026" : "\u8F93\u5165\u6D88\u606F\u3001/\u547D\u4EE4 \u6216 @agent \u59D4\u6258",
      disabled: busy
    }
  ), mode === "model" && /* @__PURE__ */ React3.createElement(Box3, { flexDirection: "column" }, /* @__PURE__ */ React3.createElement(Text3, { bold: true, color: "cyan" }, "\u53EF\u7528\u6A21\u578B\uFF08\u8F93\u5165 id \u540E Enter\uFF09:"), modelOptions.map((m) => /* @__PURE__ */ React3.createElement(Text3, { key: m, color: "white" }, "  ", m)), /* @__PURE__ */ React3.createElement(TextInput, { value: modelPick, onChange: setModelPick, onSubmit: onModelSubmit }))), /* @__PURE__ */ React3.createElement(Box3, { flexShrink: 0 }, /* @__PURE__ */ React3.createElement(Text3, { dimColor: true }, "Tab \u5207agent \xB7 \u2191\u2193 \u6EDA\u52A8 \xB7 Ctrl+T \u5C55\u5F00\u601D\u8003 \xB7 / \u547D\u4EE4 \xB7 @agent \u59D4\u6258 \xB7 Esc \u53D6\u6D88")));
}

// src/index.js
init_config();
var args = process.argv.slice(2);
if (args.includes("--version") || args.includes("-v")) {
  console.log("ux-agent 1.0.0");
  process.exit(0);
}
if (args.includes("--help") || args.includes("-h")) {
  console.log(`Uinxed AI TUI Agent

\u7528\u6CD5:
  ux-agent                \u542F\u52A8\u4EA4\u4E92\u754C\u9762
  ux-agent --key <key>    \u8BBE\u7F6E API Key \u5E76\u542F\u52A8
  ux-agent --base <url>   \u8BBE\u7F6E\u63A5\u53E3\u5730\u5740(\u9ED8\u8BA4 ${DEFAULT_BASE_URL})
  ux-agent --model <id>   \u8BBE\u7F6E\u9ED8\u8BA4\u6A21\u578B
  ux-agent --reset        \u6E05\u9664\u5168\u90E8\u914D\u7F6E\u4E0E\u5386\u53F2
  ux-agent --version      \u7248\u672C\u4FE1\u606F
`);
  process.exit(0);
}
if (args.includes("--reset")) {
  const fs3 = await import("node:fs");
  const os2 = await import("node:os");
  const path4 = await import("node:path");
  fs3.rmSync(path4.join(os2.homedir(), ".config", "ux-agent"), { recursive: true, force: true });
  console.log("\u914D\u7F6E\u5DF2\u6E05\u9664");
  process.exit(0);
}
var cliConfig = {};
for (let i = 0; i < args.length; i++) {
  if (args[i] === "--key" && args[i + 1]) cliConfig.apiKey = args[i + 1];
  if (args[i] === "--base" && args[i + 1]) cliConfig.baseUrl = args[i + 1];
  if (args[i] === "--model" && args[i + 1]) cliConfig.model = args[i + 1];
  if (args[i] === "--provider" && args[i + 1]) cliConfig.provider = args[i + 1];
}
if (cliConfig.apiKey || cliConfig.baseUrl || cliConfig.model) {
  const { saveConfig: saveConfig2, setProviderApiKey: setProviderApiKey2, setActiveProvider: setActiveProvider2 } = await Promise.resolve().then(() => (init_config(), config_exports));
  if (cliConfig.provider) setActiveProvider2(cliConfig.provider);
  const active = (await Promise.resolve().then(() => (init_config(), config_exports))).getActiveProvider();
  if (cliConfig.apiKey) setProviderApiKey2(active.id, cliConfig.apiKey);
  if (cliConfig.baseUrl || cliConfig.model) saveConfig2(cliConfig);
  console.log(
    `\u5DF2\u914D\u7F6E: ${Object.entries(cliConfig).map(([k, v]) => `${k}=${k === "apiKey" ? v.slice(0, 8) + "\u2026" : v}`).join(", ")}`
  );
}
var cfg = loadConfig();
if (!cfg.apiKey) {
  console.log("\u63D0\u793A: \u672A\u68C0\u6D4B\u5230 API Key\u3002\u53EF\u76F4\u63A5\u5728\u754C\u9762\u5185\u8F93\u5165\uFF0C\u6216\u7528 --key \u53C2\u6570\u6307\u5B9A\u3002");
}
render(React4.createElement(App));
