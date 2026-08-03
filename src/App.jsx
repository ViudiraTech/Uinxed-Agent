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

import React, { useEffect, useRef, useState, useCallback } from "react";
import { Box, Text, useApp, useInput, useStdout } from "ink";
import TextInput from "ink-text-input";
import Markdown from "./Markdown.jsx";
import ThinkingBlock from "./Thinking.jsx";
import { chatStream, listModels, getProfile, checkApiKey, ApiError } from "./provider.js";
import { TOOL_DEFS, executeTool } from "./tools.js";
import { AGENTS, getAgent, primaryAgents, subAgents, filterTools } from "./agents.js";
import {
  loadConfig,
  saveConfig,
  getActiveProvider,
  setProviderApiKey,
  setActiveProvider,
  upsertProvider,
  removeProvider,
  setThinking,
} from "./config.js";
import path from "node:path";

const fmtTime = (ts) => {
  const d = new Date(ts);
  const p = (n) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};

const COMMANDS = [
  { cmd: "/help", desc: "显示所有命令" },
  { cmd: "/connect", desc: "接入提供商（自定义 API 服务）" },
  { cmd: "/provider", desc: "切换提供商（/provider <name>）" },
  { cmd: "/key", desc: "设置当前提供商 API Key（/key <sk-xxx>）" },
  { cmd: "/model", desc: "切换模型（/model <id>）" },
  { cmd: "/thinking", desc: "开启/关闭 thinking 展示" },
  { cmd: "/agent", desc: "列出/切换 agent（Tab 也切换）" },
  { cmd: "/quota", desc: "查询本地网关余额（仅本地提供商）" },
  { cmd: "/cd", desc: "切换工作目录" },
  { cmd: "/pwd", desc: "显示工作目录" },
  { cmd: "/new", desc: "清空当前会话" },
  { cmd: "/clear", desc: "清空本地历史" },
  { cmd: "/exit", desc: "退出" },
];

function CommandPalette({ input, onPick }) {
  const q = input.slice(1).toLowerCase();
  const matches = COMMANDS.filter((c) => c.cmd.includes(q));
  if (!matches.length) return null;
  return (
    <Box flexDirection="column" marginBottom={1}>
      {matches.map((c) => (
        <Text key={c.cmd} color="cyan">
          {"  "}{c.cmd} <Text dimColor>— {c.desc}</Text>
        </Text>
      ))}
    </Box>
  );
}

/* 内容清洗:剥离 ANSI 转义与控制字符,避免破坏终端布局 */
function sanitizeText(s) {
  return String(s == null ? "" : s)
    .replace(/\u001b\[[0-9;]*m/g, "")
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, "")
    .replace(/\u200b/g, "");
}

function estHeight(content, width) {
  const lines = sanitizeText(content).split("\n");
  let h = 0;
  let inCode = false;
  for (const l of lines) {
    if (l.trim().startsWith("```")) { h += 2; inCode = !inCode; continue; }
    const wrapped = Math.max(1, Math.ceil(l.length / Math.max(width - 8, 20)));
    h += wrapped;
  }
  return h + 2;
}

/* 错误边界:单个消息渲染失败不炸整个 TUI */
class MessageBoundary extends React.Component {
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
      return (
        <Box flexDirection="column" marginBottom={1}>
          <Text bold color="red">⚠ 消息渲染失败（已忽略）</Text>
          <Text dimColor wrap="wrap">{String(this.state.error.message || this.state.error).slice(0, 200)}</Text>
        </Box>
      );
    }
    return this.props.children;
  }
}

function MessageItem({ m, width, expandedThinking, onToggleThinking }) {
  const color = m.role === "user" ? "green" : m.tool ? "yellow" : "magenta";
  const prefix = m.role === "user" ? "❯" : m.tool ? "⚙" : "◆";
  const isToolResult = m.role === "tool_result";
  const content = sanitizeText(m.content);
  const reasoning = sanitizeText(m.reasoning);
  return (
    <MessageBoundary>
      <Box flexDirection="column" marginBottom={1} width={Math.max(width - 2, 20)}>
        <Text>
          <Text bold color={color}>{prefix}</Text>{" "}
          <Text dimColor>{fmtTime(m.time)}</Text>
          {m.agentName && <Text color={color}> [{m.agentName}]</Text>}
          {m.toolName && <Text color="yellow"> ⚙{m.toolName}</Text>}
        </Text>
        {reasoning ? (
          <ThinkingBlock
            reasoning={reasoning}
            expanded={expandedThinking}
            onToggle={onToggleThinking}
          />
        ) : null}
        {isToolResult ? (
          <Text color="gray" wrap="wrap">{content}</Text>
        ) : (
          <Markdown content={content} width={Math.max(width - 4, 16)} />
        )}
      </Box>
    </MessageBoundary>
  );
}

/* 连接向导弹窗 */
function ConnectModal({ provider, onSubmit, onCancel }) {
  const [step, setStep] = useState(0); // 0=名称/地址 1=模型 2=APIKey
  const [name, setName] = useState(provider?.name || "");
  const [baseUrl, setBaseUrl] = useState(provider?.baseUrl || "");
  const [models, setModels] = useState(provider?.models?.join(",") || "");
  const [apiKey, setApiKey] = useState(provider?.apiKey || "");
  const [err, setErr] = useState("");

  const submit = (v) => {
    if (step === 0) {
      if (!name.trim() || !baseUrl.trim()) { setErr("名称和地址不能为空"); return; }
      setErr(""); setStep(1);
    } else if (step === 1) {
      setErr(""); setStep(2);
    } else {
      onSubmit({
        name: name.trim(),
        baseUrl: baseUrl.trim(),
        models: models.split(",").map((s) => s.trim()).filter(Boolean),
        apiKey: apiKey.trim() || null,
        id: provider?.id,
      });
    }
  };

  return (
    <Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={2} paddingY={1} marginBottom={1}>
      <Text bold color="cyan">连接提供商 ({step + 1}/3)</Text>
      {step === 0 && (
        <>
          <Text dimColor>名称:</Text>
          <TextInput value={name} onChange={setName} onSubmit={() => submit()} placeholder="如: 我的中转站" />
          <Text dimColor>接口地址 (OpenAI 兼容 base, 如 https://xxx.com/v1):</Text>
          <TextInput value={baseUrl} onChange={setBaseUrl} onSubmit={() => submit()} placeholder="https://api.example.com/v1" />
        </>
      )}
      {step === 1 && (
        <>
          <Text dimColor>模型列表 (逗号分隔):</Text>
          <TextInput value={models} onChange={setModels} onSubmit={() => submit()} placeholder="model-a, model-b" />
        </>
      )}
      {step === 2 && (
        <>
          <Text dimColor>API Key (可留空):</Text>
          <TextInput value={apiKey} onChange={setApiKey} onSubmit={() => submit()} placeholder="sk-xxx" />
          <Text dimColor>Enter 保存 · Esc 取消</Text>
        </>
      )}
      {err && <Text color="red">{err}</Text>}
    </Box>
  );
}

export default function App() {
  const { exit } = useApp();
  const { stdout } = useStdout();
  const [cfg, setCfg] = useState(() => loadConfig());
  const [messages, setMessages] = useState([]);
  const [conversation, setConversation] = useState(() => loadConfig().conversation || []);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [profile, setProfile] = useState(null);
  const [status, setStatus] = useState("就绪");
  const [agentId, setAgentId] = useState("build");
  const [cwd, setCwd] = useState(() => process.cwd());
  const [mode, setMode] = useState("chat"); // chat | login | connect | model
  const [loginInput, setLoginInput] = useState("");
  const [loginErr, setLoginErr] = useState("");
  const [connectProvider, setConnectProvider] = useState(null);
  const [modelOptions, setModelOptions] = useState([]);
  const [modelPick, setModelPick] = useState("");
  const [showCommands, setShowCommands] = useState(false);
  const [scrollOffset, setScrollOffset] = useState(0);
  const [expandedThinking, setExpandedThinking] = useState(false);
  const [thinkingCache, setThinkingCache] = useState({}); // msgId -> reasoning
  const [streaming, setStreaming] = useState(null); // 当前流式消息
  const aborter = useRef(null);
  const toastTimer = useRef(null);

  const WIDTH = stdout.columns || 100;
  const HEIGHT = stdout.rows || 30;
  const MSG_HEIGHT = Math.max(HEIGHT - 11, 8);

  const provider = getActiveProvider();
  const agent = getAgent(agentId);

  /* 终端级清屏：TUI 残留内容会影响体验 */
  const clearScreen = useCallback(() => {
    try {
      process.stdout.write("\x1b[2J\x1b[H");
    } catch {}
  }, []);

  useEffect(() => { setScrollOffset(0); }, [messages.length, busy, streaming]);

  const toast = useCallback((msg) => {
    setStatus(msg);
    clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setStatus("就绪"), 3000);
  }, []);

  const persist = useCallback((msgs, conv) => {
    saveConfig({
      history: msgs
        .filter((m) => m.role === "user" || m.role === "assistant")
        .map((m) => ({ role: m.role, content: m.content, time: m.time, reasoning: m.reasoning }))
        .slice(-200),
      conversation: (conv || conversation).slice(-200),
    });
  }, [conversation]);

  const refreshProfile = useCallback(async () => {
    try { return await getProfile(); } catch { return null; }
  }, []);

  useEffect(() => {
    clearScreen();
    const hist = loadConfig().history;
    if (hist.length) {
      setMessages(hist.map((m) => ({ ...m, time: m.time || Date.now() })));
    } else {
      setMessages([{
        role: "assistant",
        content: "你好，我是 **Uinxed AI Agent**。支持多提供商（`/provider`）、工具调用、thinking 展示。输入 `/` 查看命令。",
        time: Date.now(),
      }]);
    }
    const p = getActiveProvider();
    if (p?.apiKey) {
      refreshProfile().then((u) => {
        setStatus(u ? `已登录 ${u.username}${u.unlimited ? "（无限额度）" : `，余额 ¥${(u.quota || 0).toFixed(2)}`}` : `已连接 ${p.name}`);
      });
    } else {
      setStatus(`${p.name} 未设置 Key · 提供商 ${p.name}（${p.baseUrl}）`);
    }
  }, [refreshProfile, clearScreen]);

  /* ============ Agent 循环(流式 + 工具) ============ */
  const runAgent = useCallback(
    async (userText, targetAgent, isSub = false) => {
      const active = isSub ? getAgent(targetAgent) : agent;
      const toolDefs = filterTools(TOOL_DEFS, active);
      const currentProvider = getActiveProvider();
      if (!currentProvider.apiKey) {
        setMode("login");
        setStatus(`请为 ${currentProvider.name} 输入 API Key`);
        setBusy(false);
        return;
      }

      const msgs = [
        { role: "system", content: active.prompt },
        ...conversation.slice(-20),
        { role: "user", content: userText },
      ];

      const streamId = Date.now() + "-" + Math.random().toString(36).slice(2, 6);
      const thinkingId = `think-${streamId}`;
      let streamAcc = { content: "", reasoning: "" };
      let lastFlush = 0;
      let conversationAdded = false;

      for (let step = 0; step < 12; step++) {
        setStatus(`${active.name} 思考中… (${step + 1}/12)`);
        let finalRes = null;
        aborter.current = new AbortController();
        try {
          for await (const chunk of chatStream(msgs, {
            tools: toolDefs,
            signal: aborter.current.signal,
          })) {
            if (chunk.reasoning) {
              streamAcc.reasoning += chunk.reasoning;
              setThinkingCache((c) => ({ ...c, [thinkingId]: streamAcc.reasoning }));
            }
            if (chunk.content) {
              streamAcc.content += chunk.content;
              /* 节流刷新:每 80ms 或有换行时才更新,避免逐字闪光 */
              const now = Date.now();
              if (now - lastFlush > 80 || chunk.content.includes("\n")) {
                setStreaming(null);
                setStreaming({ id: streamId, role: "assistant", content: streamAcc.content, reasoning: streamAcc.reasoning, time: Date.now() });
                lastFlush = now;
              }
            }
            if (chunk.finishReason || chunk.done) finalRes = chunk;
          }
          /* 最终 flush */
          if (streamAcc.content || streamAcc.reasoning) {
            setStreaming({ id: streamId, role: "assistant", content: streamAcc.content, reasoning: streamAcc.reasoning, time: Date.now() });
          }
        } catch (e) {
          if (e.name === "AbortError") { setStatus("已取消"); setStreaming(null); setBusy(false); return; }
          setMessages((m) => [...m, {
            role: "assistant", agentName: active.name,
            content: e instanceof ApiError && e.type === "content_filter" ? "话题被过滤。" : `错误: ${e.message}`,
            time: Date.now(),
          }]);
          setStatus("出错");
          setStreaming(null); setBusy(false);
          return;
        }

        const toolCalls = finalRes?.toolCalls || [];
        const reasoning = thinkingCache[thinkingId] || "";

        if (toolCalls.length) {
          /* DeepSeek: assistant 带 tool_calls 时 reasoning_content 必须回传 */
          const assistantMsg = { role: "assistant", content: "", tool_calls: [] };
          assistantMsg.reasoning_content = streamAcc.reasoning || "";
          const toolResults = [];
          for (let ti = 0; ti < toolCalls.length; ti++) {
            const tc = toolCalls[ti];
            if (!tc.id) tc.id = `call_${Date.now()}_${ti}`;
            assistantMsg.tool_calls.push(tc);
            let parsed = {};
            try { parsed = JSON.parse(tc.function.arguments || "{}"); } catch {}
            setMessages((m) => [...m, {
              role: "tool", agentName: active.name, toolName: tc.function.name,
              content: `⚙ ${tc.function.name}(${JSON.stringify(parsed).slice(0, 100)})`,
              time: Date.now(),
            }]);
            setStatus(`执行 ${tc.function.name}…`);
            if (tc.function.name === "bash") clearScreen();
            const result = await executeTool(tc.function.name, parsed, cwd);
            const resultText = JSON.stringify(result, null, 2).slice(0, 12000);
            setMessages((m) => [...m, {
              role: "tool_result", agentName: active.name, toolName: tc.function.name,
              content: resultText.slice(0, 2500),
              time: Date.now(),
            }]);
            toolResults.push({ role: "tool", tool_call_id: tc.id, content: resultText });
          }
          msgs.push(assistantMsg);            // assistant 在前
          for (const tr of toolResults) msgs.push(tr);  // tool 结果在后
          if (!conversationAdded) {
            conversationAdded = true;
            setConversation((conv) => {
              const next = [...conv, { role: "user", content: userText }].concat(
                assistantMsg, toolResults
              );
              setMessages((m) => { persist(m, next); return m; });
              return next;
            });
          } else {
            setConversation((conv) => {
              const next = [...conv, assistantMsg, ...toolResults];
              setMessages((m) => { persist(m, next); return m; });
              return next;
            });
          }
          setStreaming(null);
          /* 清理本轮 thinking 缓存 */
          setThinkingCache((c) => { const n = { ...c }; delete n[thinkingId]; return n; });
          streamAcc = { content: "", reasoning: "" };
          continue;
        }

        /* 完成 */
        const content = streamAcc.content || finalRes?.content || "(空回复)";
        const finalMsg = {
          role: "assistant",
          agentName: active.name,
          content,
          reasoning: streamAcc.reasoning || undefined,
          time: Date.now(),
          usage: finalRes?.usage,
        };
        const apiFinal = {
          role: "assistant",
          content,
          reasoning_content: streamAcc.reasoning || "",
        };
        setConversation((conv) => {
          const next = conversationAdded
            ? [...conv, apiFinal]
            : [...conv, { role: "user", content: userText }, apiFinal];
          persist([...messages, finalMsg], next);
          return next;
        });
        setMessages((m) => [...m, finalMsg]);
        setStreaming(null);
        setThinkingCache((c) => { const n = { ...c }; delete n[thinkingId]; return n; });
        const u = finalRes?.usage || {};
        setStatus(`${active.name} 完成 · ${u.prompt_tokens || 0}/${u.completion_tokens || 0} tokens${isSub ? "（子任务）" : ""}`);
        setBusy(false);
        refreshProfile();
        return;
      }
      setMessages((m) => [...m, { role: "assistant", agentName: active.name, content: "步骤数已达上限。", time: Date.now() }]);
      setBusy(false); setStreaming(null);
      setStatus("达到步骤上限");
    },
    [messages, conversation, cwd, persist, agent, refreshProfile, streaming, clearScreen]
  );

  /* ============ 命令 ============ */
  const runCommand = useCallback(async (cmd) => {
    const [name, ...rest] = cmd.trim().split(/\s+/);
    const arg = rest.join(" ");
    switch (name) {
      case "/help":
        setMessages((m) => [...m, {
          role: "assistant",
          content: "**可用命令**\n" + COMMANDS.map((c) => `- \`${c.cmd}\` — ${c.desc}`).join("\n"),
          time: Date.now(),
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
            content: "**提供商**\n" + providers.map((p) =>
              `- \`${p.id}\` ${p.name}${p.id === loadConfig().activeProvider ? "（当前）" : ""} · ${p.baseUrl}`
            ).join("\n") + "\n\n切换: `/provider <id>` · 接入新的: `/connect`",
            time: Date.now(),
          }]);
        } else {
          const target = providers.find((p) => p.id === arg);
          if (target) {
            const next = setActiveProvider(arg);
            if (next) {
              setCfg(next);
              setStatus(`已切换提供商: ${target.name}（${next.model}）`);
              if (!target.apiKey) setMode("login");
            }
          } else {
            setStatus(`没有提供商: ${arg}（/connect 接入）`);
          }
        }
        break;
      }
      case "/key":
        if (arg) {
          const p = getActiveProvider();
          setProviderApiKey(p.id, arg.trim());
          setCfg(loadConfig());
          setStatus("验证 Key…");
          const ok = await checkApiKey(arg.trim(), p.id);
          if (ok) {
            refreshProfile().then((u) =>
              setStatus(u ? `已登录 ${u.username}${u.unlimited ? "（无限额度）" : `，余额 ¥${(u.quota || 0).toFixed(2)}`}` : "Key 有效")
            );
          } else setStatus("Key 无效");
        } else {
          const p = getActiveProvider();
          const key = p.apiKey;
          setStatus(key ? `${p.name} Key: ${key.slice(0, 10)}…（/key <新Key>）` : `${p.name} 未设置 Key`);
        }
        break;
      case "/thinking":
        if (arg === "on" || arg === "true" || arg === "1") { setThinking(true); setStatus("thinking 显示开启"); }
        else if (arg === "off" || arg === "false" || arg === "0") { setThinking(false); setStatus("thinking 显示关闭"); }
        else {
          const cur = loadConfig().thinking;
          setThinking(!cur);
          setStatus(`thinking 显示: ${!cur ? "开" : "关"}`);
        }
        break;
      case "/model":
        if (!arg) {
          const p = getActiveProvider();
          setModelOptions(p.models || []);
          setModelPick("");
          setMode("model");
          setStatus(`当前模型: ${loadConfig().model}`);
        } else {
          saveConfig({ model: arg });
          setCfg(loadConfig());
          setStatus(`模型 → ${arg}`);
        }
        break;
      case "/quota":
        setStatus("查询中…");
        try {
          const u = await getProfile();
          setProfile(u);
          setStatus(u ? `${u.username} · 余额 ¥${(u.quota || 0).toFixed(2)} · ${u.groupName} ×${u.groupRate}${u.unlimited ? "（无限）" : ""}` : "该提供商无账户接口");
        } catch (e) { setStatus(`查询失败: ${e.message}`); }
        break;
      case "/cd":
        if (!arg) setStatus(`当前目录: ${cwd}`);
        else {
          const next = path.resolve(cwd, arg);
          try {
            const fs = await import("node:fs");
            if (fs.statSync(next).isDirectory()) { setCwd(next); setStatus(`已切换到: ${next}`); }
            else setStatus("不是目录");
          } catch { setStatus(`目录不存在: ${next}`); }
        }
        break;
      case "/pwd":
        setStatus(`工作目录: ${cwd}`);
        break;
      case "/new":
        clearScreen();
        setMessages([{ role: "assistant", content: "新会话已开始。", time: Date.now() }]);
        setConversation([]);
        saveConfig({ history: [], conversation: [] });
        break;
      case "/clear":
        clearScreen();
        setMessages([{ role: "assistant", content: "历史已清空。", time: Date.now() }]);
        setConversation([]);
        saveConfig({ history: [], conversation: [] });
        break;
      case "/exit":
        exit();
        break;
      default:
        setStatus(`未知命令: ${name}（/help）`);
    }
  }, [cwd, exit, refreshProfile, clearScreen]);

  /* ============ 输入 ============ */
  const onSubmit = (value) => {
    if (busy) { toast("请等待当前任务完成"); return; }
    const text = value.trim();
    if (!text) return;
    setShowCommands(false);
    if (text.startsWith("/")) { runCommand(text); setInput(""); return; }
    const p = getActiveProvider();
    if (!p.apiKey) { setMode("login"); setStatus(`请为 ${p.name} 输入 API Key`); return; }
    const sub = subAgents().find((a) => text.startsWith(`@${a.name}`));
    if (sub) {
      const rest = text.replace(new RegExp(`^@${sub.name}\\s*`), "");
      setMessages((m) => [...m, { role: "user", content: text, time: Date.now() }]);
      setInput(""); setBusy(true);
      clearScreen();
      runAgent(rest || "帮我完成这个任务", sub.name, true);
      return;
    }
    setMessages((m) => [...m, { role: "user", content: text, time: Date.now() }]);
    setInput(""); setBusy(true);
    runAgent(text, agentId, false);
  };

  const onLoginSubmit = async (value) => {
    const key = value.trim();
    if (!key) { setLoginErr("Key 不能为空"); return; }
    setLoginErr("验证中…");
    const p = getActiveProvider();
    const ok = await checkApiKey(key, p.id);
    if (ok) {
      setProviderApiKey(p.id, key);
      setCfg(loadConfig());
      setMode("chat"); setLoginErr("");
      setStatus(`已保存 ${p.name} Key`);
      refreshProfile();
    } else setLoginErr("Key 无效，请检查（可留空跳过验证）");
  };

  const onConnectSubmit = (data) => {
    const p = upsertProvider(data);
    setCfg(loadConfig());
    setMode("chat");
    setStatus(`已接入提供商: ${p.name}`);
    if (data.apiKey) {
      setProviderApiKey(p.id, data.apiKey);
      setCfg(loadConfig());
    }
  };

  const onModelSubmit = (id) => {
    if (id) { saveConfig({ model: id }); setCfg(loadConfig()); setStatus(`模型 → ${id}`); }
    setMode("chat");
  };

  /* 按键 */
  useInput((_input, key) => {
    if (key.escape) {
      if (mode !== "chat") { setMode("chat"); setConnectProvider(null); return; }
    }
    if (mode !== "chat") return;
    if (key.tab) {
      const primaries = primaryAgents();
      const idx = primaries.findIndex((a) => a.id === agentId);
      const next = primaries[(idx + 1) % primaries.length];
      setAgentId(next.id);
      clearScreen();
      setStatus(`agent → ${next.name}`);
    }
    if (key.ctrl && (_input === "t" || _input === "T")) {
      setExpandedThinking((e) => !e);
    }
    if (key.upArrow) setScrollOffset((o) => Math.min(o + 3, 10000));
    if (key.downArrow) setScrollOffset((o) => Math.max(o - 3, 0));
    if (key.pageUp) setScrollOffset((o) => o + MSG_HEIGHT);
    if (key.pageDown) setScrollOffset((o) => Math.max(o - MSG_HEIGHT, 0));
  });

  /* 滚动窗口 */
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
      if (offset > 0) { offset -= itemHeights[i]; continue; }
      if (used + itemHeights[i] > MSG_HEIGHT) break;
      used += itemHeights[i];
      startIdx = i;
    }
  }
  if (startIdx < 0) startIdx = 0;
  const visible = messages.slice(startIdx);

  const agentColor = agent.color;

  return (
    <Box flexDirection="column">
      {/* 状态栏 */}
      <Box flexDirection="row" flexShrink={0}>
        <Text bold color="cyan">◆ Uinxed</Text>
        <Text color={agentColor} bold> {agent.name}</Text>
        <Text dimColor> · {provider.name}</Text>
        <Text bold> · {loadConfig().model}</Text>
        <Text color={profile ? "green" : "yellow"}> · {profile ? profile.username : (provider.apiKey ? "已连接" : "未登录")}</Text>
        {profile && !profile.unlimited && <Text dimColor> · ¥{(profile.quota || 0).toFixed(2)}</Text>}
        <Text dimColor> · {status}</Text>
      </Box>

      {/* 消息区 */}
      <Box flexGrow={1} flexShrink={1} height={MSG_HEIGHT} flexDirection="column" borderStyle="round" borderColor="gray" paddingX={1} overflow="hidden">
        {visible.map((m, i) => (
          <MessageItem
            key={m.time + "-" + i}
            m={m}
            width={WIDTH}
            expandedThinking={expandedThinking}
            onToggleThinking={() => setExpandedThinking((e) => !e)}
          />
        ))}
        {streaming && (
          <MessageItem
            m={{ role: "assistant", content: streaming.content, reasoning: streaming.reasoning, time: Date.now() }}
            width={WIDTH}
            expandedThinking={expandedThinking}
            onToggleThinking={() => setExpandedThinking((e) => !e)}
          />
        )}
        {!streaming && Object.keys(thinkingCache).length > 0 && (
          <MessageItem
            m={{ role: "assistant", content: "◈ 推理中…", time: Date.now() }}
            width={WIDTH}
            expandedThinking={expandedThinking}
            onToggleThinking={() => setExpandedThinking((e) => !e)}
          />
        )}
        {busy && !streaming && <Text dimColor>…</Text>}
        {scrollOffset > 0 && <Text dimColor backgroundColor="#222" bold> ↑ 上翻中（↓ 回底部）</Text>}
      </Box>

      {/* 连接弹窗 */}
      {mode === "connect" && (
        <ConnectModal provider={connectProvider} onSubmit={onConnectSubmit} onCancel={() => setMode("chat")} />
      )}
      {mode === "login" && (
        <Box borderStyle="round" borderColor="yellow" paddingX={2} paddingY={1} marginBottom={1}>
          <Box flexDirection="column">
            <Text bold color="yellow">为 {provider.name} 输入 API Key:</Text>
            <TextInput value={loginInput} onChange={setLoginInput} onSubmit={onLoginSubmit} />
            {loginErr && <Text color="red">{loginErr}</Text>}
            <Text dimColor>Enter 保存 · Esc 取消</Text>
          </Box>
        </Box>
      )}

      {/* 命令面板 */}
      {showCommands && input.startsWith("/") && <CommandPalette input={input} />}

      {/* 输入区 */}
      <Box borderStyle="round" borderColor="gray" paddingX={1} flexShrink={0}>
        {mode === "chat" && (
          <TextInput
            value={input}
            onChange={(v) => { setInput(v); setShowCommands(v.startsWith("/")); }}
            onSubmit={onSubmit}
            placeholder={busy ? "任务执行中…" : "输入消息、/命令 或 @agent 委托"}
            disabled={busy}
          />
        )}
        {mode === "model" && (
          <Box flexDirection="column">
            <Text bold color="cyan">可用模型（输入 id 后 Enter）:</Text>
            {modelOptions.map((m) => <Text key={m} color="white">  {m}</Text>)}
            <TextInput value={modelPick} onChange={setModelPick} onSubmit={onModelSubmit} />
          </Box>
        )}
      </Box>

      <Box flexShrink={0}>
        <Text dimColor>Tab 切agent · ↑↓ 滚动 · Ctrl+T 展开思考 · / 命令 · @agent 委托 · Esc 取消</Text>
      </Box>
    </Box>
  );
}
