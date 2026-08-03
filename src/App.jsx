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
import { LineRow } from "./Markdown.jsx";
import { markdownLines, wrapPlain } from "./mdlines.js";
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
  /* 子 agent 会话:delegate 时创建,可切换到子聊天区实时查看/继续对话 */
  const [subSession, setSubSession] = useState(null); // {id, agentId, task, msgs[], streaming, busy, status, result}
  const [subView, setSubView] = useState(false); // 是否切到子聊天区
  const [subInput, setSubInput] = useState("");
  const [subScrollOffset, setSubScrollOffset] = useState(0);
  const subMsgsRef = useRef([]); // 子会话权威 API 消息序列
  const aborter = useRef(null);
  const toastTimer = useRef(null);

  /* 终端动态尺寸:resize 时自动重算,布局吃满终端 */
  const [termSize, setTermSize] = useState(() => [stdout.columns || 100, stdout.rows || 30]);
  useEffect(() => {
    const onResize = () => setTermSize([stdout.columns || 100, stdout.rows || 30]);
    stdout.on("resize", onResize);
    return () => stdout.off("resize", onResize);
  }, [stdout]);

  const WIDTH = termSize[0];
  const HEIGHT = termSize[1];
  /* 垂直布局：状态栏(1) + 消息区border(2) + 输入区(3) + 快捷键(1) = 7 */
  const MSG_HEIGHT = Math.max(HEIGHT - 7, 10);
  const rowWidth = Math.max(WIDTH - 4, 16);

  const provider = getActiveProvider();
  const agent = getAgent(agentId);

  /* 终端级清屏：TUI 残留内容会影响体验 */
  const clearScreen = useCallback(() => {
    try {
      process.stdout.write("\x1b[2J\x1b[H");
    } catch {}
  }, []);

  /* 底部跟随：offset=0 时新行自动顶上来；用户上翻后保持位置（safeOffset clamp） */

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

  /* ============ 子 Agent 会话(供 delegate 工具使用) ============ */
  /* 初始化子会话状态(UI 入口 + 子聊天区数据源) */
  const initSubSession = useCallback((subAgentName, task) => {
    const id = `sub-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
    const msgs = [{ role: "system", content: getAgent(subAgentName).prompt }, { role: "user", content: task }];
    subMsgsRef.current = msgs;
    setSubSession({ id, agentId: subAgentName, task, msgs: [], streaming: null, busy: true, status: "运行中", result: null });
    return id;
  }, []);

  /* 更新子会话:msgs 用 ref 里的权威序列,streaming 单独存 */
  const updateSub = useCallback((patch) => {
    setSubSession((prev) => prev ? { ...prev, ...patch } : prev);
  }, []);

  /* 子 agent 多轮循环:返回 { agent, result } 供主 agent 工具结果回传 */
  const runSubAgentLoop = useCallback(async (subAgentName, task, maxSteps = 8, onProgress) => {
    const sub = getAgent(subAgentName);
    if (!sub || sub.role !== "subagent") return { error: `未知子 agent: ${subAgentName}（可用 explorer/general）` };
    const subTools = filterTools(TOOL_DEFS, sub).filter((d) => d.function.name !== "delegate");
    const subMsgs = subMsgsRef.current.length >= 2
      ? subMsgsRef.current
      : [{ role: "system", content: sub.prompt }, { role: "user", content: task }];
    subMsgsRef.current = subMsgs;
    let subAcc = { content: "", reasoning: "" };
    let subLastFlush = 0;
    for (let step = 0; step < maxSteps; step++) {
      let finalRes = null;
      subAcc = { content: "", reasoning: "" };
      updateSub({ status: `运行中 (${step + 1}/${maxSteps})` });
      try {
        for await (const chunk of chatStream(subMsgs, { tools: subTools })) {
          if (chunk.reasoning) subAcc.reasoning += chunk.reasoning;
          if (chunk.content) {
            subAcc.content += chunk.content;
            /* 节流刷新:每 120ms 或有换行才更新,避免逐字闪光 */
            const now = Date.now();
            if (now - subLastFlush > 120 || chunk.content.includes("\n")) {
              updateSub({ streaming: { role: "assistant", content: subAcc.content, reasoning: subAcc.reasoning, time: Date.now() } });
              subLastFlush = now;
            }
            onProgress?.(subAcc);
          }
          if (chunk.finishReason || chunk.done) finalRes = chunk;
        }
      } catch (e) {
        updateSub({ busy: false, status: "出错" });
        return { error: `子 agent ${sub.name} 出错: ${e.message}` };
      }
      const toolCalls = finalRes?.toolCalls || [];
      if (!toolCalls.length) {
        const out = subAcc.content || "(空回复)";
        updateSub({ busy: false, status: "完成", result: out, streaming: null });
        return { agent: sub.name, result: out };
      }
      const subAssistantMsg = { role: "assistant", content: subAcc.content, tool_calls: [] };
      subAssistantMsg.reasoning_content = subAcc.reasoning || "";
      const subResults = [];
      for (let ti = 0; ti < toolCalls.length; ti++) {
        const tc = toolCalls[ti];
        if (!tc.id) tc.id = `call_${Date.now()}_${ti}`;
        subAssistantMsg.tool_calls.push(tc);
        let parsed = {};
        try { parsed = JSON.parse(tc.function.arguments || "{}"); } catch {}
        updateSub({ status: `执行 ${tc.function.name}…` });
        const result = await executeTool(tc.function.name, parsed, cwd);
        subResults.push({ role: "tool", tool_call_id: tc.id, content: JSON.stringify(result).slice(0, 12000) });
      }
      subMsgs.push(subAssistantMsg);
      for (const tr of subResults) subMsgs.push(tr);
      subMsgsRef.current = subMsgs;
      updateSub({ streaming: null });
    }
    const out = subAcc.content || "(达到步骤上限)";
    updateSub({ busy: false, status: "完成", result: out, streaming: null });
    return { agent: sub.name, result: out };
  }, [cwd, updateSub]);

  /* 子聊天区继续对话:把用户新消息追加进子会话再跑循环 */
  const subSendMessage = useCallback((text) => {
    if (!subSession) return;
    subMsgsRef.current = [...subMsgsRef.current, { role: "user", content: text }];
    updateSub({ busy: true, status: "运行中" });
    setSubInput("");
    return runSubAgentLoop(subSession.agentId, text, 8);
  }, [subSession, runSubAgentLoop, updateSub]);

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
            /* 工具调用只显示一行简短提示 */
            setMessages((m) => [...m, {
              role: "tool", agentName: active.name,
              content: `调用工具 ${tc.function.name}`,
              time: Date.now(),
            }]);
            setStatus(`执行 ${tc.function.name}…`);
            if (tc.function.name === "bash") clearScreen();
            let result;
            if (tc.function.name === "delegate") {
              /* AI 主动委托子 agent:创建子会话(UI 折叠入口 + 可切换的子聊天区) */
              const subName = parsed.agent || "general";
              const subTask = parsed.task || "帮我完成这个任务";
              setStatus(`委托 ${subName}…`);
              initSubSession(subName, subTask);
              result = await runSubAgentLoop(subName, subTask, 8);
              result = { subagent: subName, output: result };
            } else {
              result = await executeTool(tc.function.name, parsed, cwd);
            }
            const resultText = JSON.stringify(result, null, 2).slice(0, 12000);
            /* 工具结果不展示原始 JSON，只显示一行简短状态 */
            setMessages((m) => [...m, {
              role: "tool_result", agentName: active.name,
              content: `✓ ${tc.function.name} 完成`,
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
    [messages, conversation, cwd, persist, agent, refreshProfile, streaming, clearScreen, runSubAgentLoop, initSubSession]
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
      if (subView) { setSubView(false); clearScreen(); return; }
      if (mode !== "chat") { setMode("chat"); setConnectProvider(null); return; }
    }
    /* 子聊天区:独立滚动上下文 */
    if (subView) {
      if (key.upArrow) setSubScrollOffset((o) => Math.min(o + 3, 10000));
      if (key.downArrow) setSubScrollOffset((o) => Math.max(o - 3, 0));
      if (key.pageUp) setSubScrollOffset((o) => o + MSG_HEIGHT);
      if (key.pageDown) setSubScrollOffset((o) => Math.max(o - MSG_HEIGHT, 0));
      return;
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
    if (key.rightArrow && subSession && subSession.id) {
      setSubView(true);
      setSubScrollOffset(0);
      clearScreen();
      setStatus(`子 agent ${subSession.agentId} 会话 · Esc 返回主界面`);
    }
    if (key.ctrl && (_input === "t" || _input === "T")) {
      setExpandedThinking((e) => !e);
    }
    if (key.upArrow) setScrollOffset((o) => Math.min(o + 3, 10000));
    if (key.downArrow) setScrollOffset((o) => Math.max(o - 3, 0));
    if (key.pageUp) setScrollOffset((o) => o + MSG_HEIGHT);
    if (key.pageDown) setScrollOffset((o) => Math.max(o - MSG_HEIGHT, 0));  });

  /* 滚动窗口：基于行的精确切片 */
  const msgColor = (m) => (m.role === "user" ? "green" : m.role === "tool" ? "yellow" : m.role === "tool_result" ? "gray" : "magenta");
  const msgPrefix = (m) => (m.role === "user" ? "❯" : m.role === "tool" ? "⚙" : m.role === "tool_result" ? "✓" : "◆");

  /* 单条消息 → 行数组（每行固定 1 高） */
  const messageToRows = (m, isStreaming) => {
    const rows = [];
    const color = msgColor(m);
    const prefix = msgPrefix(m);
    const header = `${prefix} ${fmtTime(m.time)}${m.agentName ? ` [${m.agentName}]` : ""}`;
    rows.push({ kind: "header", m, text: header, color, bold: true });

    /* 思考块（折叠时只显示一行，展开时多行，长行自动换行） */
    if (m.reasoning) {
      if (expandedThinking) {
        for (const l of wrapPlain(String(m.reasoning), rowWidth)) {
          rows.push({ kind: "thinking", m, text: `  ${l}`, color: "gray", dim: true });
        }
      } else {
        rows.push({ kind: "thinking", m, text: "  ↯ 已思考（Ctrl+T 展开）", color: "gray", dim: true });
      }
    }
    if (!m.content) return rows;

    /* 工具结果 → 灰色单行 */
    if (m.role === "tool_result") {
      rows.push({ kind: "md", m, text: `  ${String(m.content).replace(/\n/g, " ")}`, color: "gray", dim: true });
      return rows;
    }
    /* 其余 → markdown 行模型 */
    const lines = markdownLines(m.content, rowWidth, {});
    for (const l of lines) rows.push({ kind: "md", m, mdline: l });
    return rows;
  };

  /* 全部可见行（含流式中的消息） */
  let allRows = [];
  for (const m of messages) allRows = allRows.concat(messageToRows(m, false));
  if (streaming) allRows = allRows.concat(messageToRows(streaming, true));
  else if (Object.keys(thinkingCache).length && busy) {
    allRows.push({ kind: "thinking", m: null, text: "  ↯ 推理中…", color: "gray", dim: true });
  }
  /* 子 agent 折叠入口(类似 thinking,→ 切换查看) */
  if (subSession) {
    const subDone = !subSession.busy;
    allRows.push({
      kind: "subagent", m: null,
      text: subDone
        ? `  ↳ 子agent ${subSession.agentId} 完成${subView ? "" : "（→ 查看结果）"}`
        : `  ↯ 子agent ${subSession.agentId} ${subSession.status || "运行中"}${subView ? "" : "（→ 查看）"}`,
      color: "magenta", bold: true,
    });
  }
  if (busy && !streaming && !allRows.length) allRows.push({ kind: "md", m: null, text: "…", color: "gray" });

  const maxScroll = Math.max(0, allRows.length - MSG_HEIGHT);
  const safeOffset = Math.min(scrollOffset, maxScroll);
  const start = Math.max(0, allRows.length - MSG_HEIGHT - safeOffset);
  const visibleRows = allRows.slice(start, start + MSG_HEIGHT);

  const agentColor = agent.color;

  /* ============ 子聊天区渲染（subView=true） ============ */
  if (subView && subSession) {
    const subAgent = getAgent(subSession.agentId);
    const subColor = subAgent.color || "magenta";
    const subMsgs = subMsgsRef.current || [];
    /* 从 ref 构建子会话 UI 消息:user 原文 + assistant 正文 + tool 一行 */
    const subRows = [];
    let pendingText = null;
    let toolName = "";
    for (const m of subMsgs) {
      if (m.role === "user") {
        if (pendingText) { subRows.push({ role: "assistant", content: pendingText }); pendingText = null; }
        subRows.push({ role: "user", content: m.content });
      } else if (m.role === "assistant") {
        /* 先固化已收到的正文(即使带 tool_calls 也要保留),再插工具行 */
        if (m.content) pendingText = (pendingText || "") + m.content;
        if (m.tool_calls && m.tool_calls.length) {
          if (pendingText) { subRows.push({ role: "assistant", content: pendingText }); pendingText = null; }
          const names = m.tool_calls.map((tc) => tc.function?.name || "?").join(", ");
          toolName = names;
        }
      } else if (m.role === "tool") {
        if (pendingText) { subRows.push({ role: "assistant", content: pendingText }); pendingText = null; }
        subRows.push({ role: "tool", content: `⚙ 调用 ${toolName || "工具"}` });
      }
    }
    if (pendingText) subRows.push({ role: "assistant", content: pendingText });
    /* 流式内容 */
    if (subSession.streaming?.content) subRows.push({ role: "assistant-stream", content: subSession.streaming.content });

    /* 行模型窗口:每个 subRow 转成 markdown 行,再按 MSG_HEIGHT 切片(同主视图) */
    const subAllRows = [];
    const subRowToLines = (rm) => {
      if (rm.role === "user") {
        const prefix = "❯ ";
        const lines = markdownLines(rm.content, Math.max(rowWidth - 2, 10), {});
        if (!lines.length) return [{ text: prefix, color: "green" }];
        lines[0].spans = [{ text: prefix, ...(lines[0].spans?.[0]?.color ? {} : {}) }, ...(lines[0].spans || [])];
        lines[0].color = "green"; lines[0].bold = false;
        return lines;
      }
      if (rm.role === "tool") {
        return [{ text: rm.content, color: "gray", dim: true }];
      }
      const base = rm.role === "assistant-stream" ? { color: "gray", dim: true } : { color: "magenta" };
      const lines = markdownLines(rm.content, rowWidth, base);
      if (!lines.length) return [{ ...base, text: "" }];
      return lines;
    };
    for (const rm of subRows) subAllRows.push(...subRowToLines(rm));
    const subMax = Math.max(0, subAllRows.length - MSG_HEIGHT);
    const subSafe = Math.min(subScrollOffset, subMax);
    const subStart = Math.max(0, subAllRows.length - MSG_HEIGHT - subSafe);
    const subVisible = subAllRows.slice(subStart, subStart + MSG_HEIGHT);

    return (
      <Box flexDirection="column" height={HEIGHT} borderStyle="round" borderColor={subColor}>
        <Box flexShrink={0}>
          <Text bold color={subColor} wrap="truncate" width={Math.max(WIDTH - 2, 10)}>
            ◆ 子agent {subSession.agentId} · {subSession.task.slice(0, 40)}{subSession.busy ? " · 运行中" : " · 完成"} · Esc 返回
          </Text>
        </Box>
        <Box flexGrow={1} flexShrink={1} height={MSG_HEIGHT} flexDirection="column" paddingX={1} overflow="hidden">
          {subVisible.map((r, i) => (
            <MessageBoundary key={i}>
              {r.spans ? (
                <LineRow line={r} width={rowWidth} />
              ) : (
                <Text color={r.color || "magenta"} bold={r.bold} dim={r.dim} wrap="truncate" width={rowWidth}>
                  {r.text}
                </Text>
              )}
            </MessageBoundary>
          ))}
          {subSession.streaming && !subSession.streaming.content && <Text dimColor>  ↯ 推理中…</Text>}
        </Box>
        <Box borderStyle="round" borderColor="gray" paddingX={1} flexShrink={0}>
          <TextInput
            value={subInput}
            onChange={setSubInput}
            onSubmit={subSendMessage}
            placeholder={subSession.busy ? "子agent 运行中…" : "继续对话，Enter 发送"}
            disabled={false}
          />
        </Box>
        <Box flexShrink={0}>
          <Text dimColor>Esc 返回主界面 · ↑↓ 滚动 · Enter 发送</Text>
        </Box>
      </Box>
    );
  }

  return (
    <Box flexDirection="column" height={HEIGHT} borderStyle="round" borderColor="gray">
      {/* 状态栏 */}
      <Box flexDirection="row" flexShrink={0}>
        <Text bold color="cyan" wrap="truncate" width={Math.max(WIDTH - 2, 10)}>◆ Uinxed {agent.name} · {provider.name} · {loadConfig().model} · {profile ? profile.username : (provider.apiKey ? "已连接" : "未登录")}{profile && !profile.unlimited ? ` · ¥${(profile.quota || 0).toFixed(2)}` : ""} · {status}</Text>
      </Box>

      {/* 消息区：行模型切片，每行固定 1 高，不会炸 */}
      <Box flexGrow={1} flexShrink={1} height={MSG_HEIGHT} flexDirection="column" paddingX={1} overflow="hidden">
        {visibleRows.map((r, i) => (
          <MessageBoundary key={r.m?.time + "-" + r.text + "-" + i}>
            {r.kind === "md" && r.mdline ? (
              <LineRow line={r.mdline} width={rowWidth} />
            ) : (
              <Text color={r.color} bold={r.bold} dim={r.dim} wrap="truncate" width={Math.max(rowWidth, 10)}>
                {r.text}
              </Text>
            )}
          </MessageBoundary>
        ))}
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
