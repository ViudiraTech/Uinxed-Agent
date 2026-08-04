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

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export const CONFIG_DIR = path.join(os.homedir(), ".config", "ux-agent");
export const CONFIG_FILE = path.join(CONFIG_DIR, "config.json");

export const DEFAULT_BASE_URL = "http://localhost:8080/v1";
export const DEFAULT_MODEL = "glm-4-flash";

/* 内置提供商模板(首次运行时自动写入) */
export const BUILTIN_PROVIDERS = [
  {
    id: "ux-gateway",
    name: "本地网关",
    baseUrl: DEFAULT_BASE_URL,
    apiKey: null,
    models: ["glm-4-flash", "glm-4-flash-proxy"],
    defaultModel: "glm-4-flash",
    builtin: true,
  },
  {
    id: "deepseek",
    name: "DeepSeek",
    baseUrl: "https://api.deepseek.com/v1",
    apiKey: null,
    models: ["deepseek-v4-pro", "deepseek-v4-flash"],
    defaultModel: "deepseek-v4-flash",
    builtin: true,
    supportsThinking: true,
  },
];

export function loadConfig() {
  try {
    const raw = fs.readFileSync(CONFIG_FILE, "utf8");
    const cfg = JSON.parse(raw);
    const providers = ensureProviders(cfg.providers);
    /* 兼容读取:旧版数据仍在 config.json 时,构造内存 session 供展示/迁移。
     * 不再自动写回 config.json(会话数据应迁往 SQLite,见 db.js / App 迁移提示)。 */
    const sessions = Array.isArray(cfg.sessions) ? cfg.sessions : [];
    if (!sessions.length && (Array.isArray(cfg.history) && cfg.history.length || Array.isArray(cfg.conversation) && cfg.conversation.length)) {
      sessions.push({
        id: "s-default",
        name: "会话 1",
        history: Array.isArray(cfg.history) ? cfg.history : [],
        conversation: Array.isArray(cfg.conversation) ? cfg.conversation : [],
        agentId: cfg.agentId || "build",
        cwd: cfg.cwd || null,
        updatedAt: Date.now(),
      });
    }
    return {
      apiKey: null, // 废弃:全部走 provider.apiKey
      baseUrl: cfg.baseUrl || DEFAULT_BASE_URL,
      model: cfg.model || DEFAULT_MODEL,
      history: Array.isArray(cfg.history) ? cfg.history : [],
      conversation: Array.isArray(cfg.conversation) ? cfg.conversation : [],
      sessions,
      activeSessionId: cfg.activeSessionId || sessions[0]?.id || null,
      /* 存储模式:db = SQLite;config = 兼容旧 config.json */
      storage: cfg.storage === "db" ? "db" : "config",
      providers,
      activeProvider: cfg.activeProvider || providers[0]?.id || "ux-gateway",
      thinking: cfg.thinking !== false,
      cwd: cfg.cwd || null,
    };
  } catch {
    const providers = ensureProviders();
    return {
      apiKey: null, baseUrl: DEFAULT_BASE_URL, model: DEFAULT_MODEL,
      history: [], conversation: [], sessions: [], activeSessionId: null,
      storage: "config",
      providers, activeProvider: providers[0].id, thinking: true, cwd: null,
    };
  }
}

function ensureProviders(existing) {
  if (Array.isArray(existing) && existing.length) {
    /* 合并内置模板(补充新字段)与自定义提供商 */
    const merged = existing.map((p) => {
      const tpl = BUILTIN_PROVIDERS.find((b) => b.id === p.id);
      return { ...(tpl || {}), ...p };
    });
    /* 补齐缺失的内置模板 */
    for (const b of BUILTIN_PROVIDERS) {
      if (!merged.some((m) => m.id === b.id)) merged.push({ ...b });
    }
    return merged;
  }
  return BUILTIN_PROVIDERS.map((p) => ({ ...p }));
}

export function saveConfig(partial) {
  const cur = loadConfig();
  const next = { ...cur, ...partial };
  fs.mkdirSync(CONFIG_DIR, { recursive: true });
  fs.writeFileSync(CONFIG_FILE, JSON.stringify(next, null, 2), "utf8");
  return next;
}

export function getActiveProvider() {
  const cfg = loadConfig();
  return cfg.providers.find((p) => p.id === cfg.activeProvider) || cfg.providers[0];
}

export function setApiKey(apiKey) {
  saveConfig({ apiKey });
}

export function setBaseUrl(baseUrl) {
  saveConfig({ baseUrl: baseUrl.replace(/\/+$/, "") });
}

export function setModel(model) {
  saveConfig({ model });
}

/* 提供商 API Key */
export function setProviderApiKey(providerId, apiKey) {
  const cfg = loadConfig();
  const providers = cfg.providers.map((p) =>
    p.id === providerId ? { ...p, apiKey: apiKey || null } : p
  );
  saveConfig({ providers, apiKey: providerId === cfg.activeProvider ? apiKey || cfg.apiKey : cfg.apiKey });
}

export function getProviderApiKey(providerId) {
  const cfg = loadConfig();
  const p = cfg.providers.find((x) => x.id === providerId);
  return (p && p.apiKey) || null;
}

/* 切换活动提供商 */
export function setActiveProvider(providerId) {
  const cfg = loadConfig();
  const p = cfg.providers.find((x) => x.id === providerId);
  if (!p) return null;
  const next = {
    ...cfg,
    activeProvider: providerId,
    baseUrl: p.baseUrl,
    model: p.defaultModel || cfg.model,
  };
  fs.mkdirSync(CONFIG_DIR, { recursive: true });
  fs.writeFileSync(CONFIG_FILE, JSON.stringify(next, null, 2), "utf8");
  return next;
}

/* 新增/更新提供商(connect) */
export function upsertProvider(provider) {
  const cfg = loadConfig();
  const id = provider.id || provider.name.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  const exists = cfg.providers.some((p) => p.id === id);
  const newProvider = {
    id,
    name: provider.name,
    baseUrl: (provider.baseUrl || "").replace(/\/+$/, ""),
    apiKey: provider.apiKey || null,
    models: Array.isArray(provider.models) && provider.models.length ? provider.models : ["default"],
    defaultModel: provider.defaultModel || (Array.isArray(provider.models) ? provider.models[0] : "default"),
    builtin: false,
  };
  const providers = exists
    ? cfg.providers.map((p) => (p.id === id ? { ...p, ...newProvider } : p))
    : [...cfg.providers, newProvider];
  saveConfig({ providers });
  return newProvider;
}

export function removeProvider(providerId) {
  const cfg = loadConfig();
  if (providerId === cfg.activeProvider) return { error: "不能删除当前活动的提供商" };
  const providers = cfg.providers.filter((p) => p.id !== providerId);
  saveConfig({ providers });
  return { ok: true };
}

export function setHistory(history) {
  saveConfig({ history: history.slice(-200) });
}

export function clearHistory() {
  saveConfig({ history: [] });
}

export function setThinking(enabled) {
  saveConfig({ thinking: !!enabled });
}
