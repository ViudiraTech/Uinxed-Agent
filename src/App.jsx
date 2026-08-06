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

import React, { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { Box, Text, useApp, useInput, useStdout } from "ink";
import TextInput from "./SafeTextInput.jsx";
import Markdown from "./Markdown.jsx";
import { LineRow } from "./Markdown.jsx";
import { markdownLines, wrapPlain, diffLines, stringWidth } from "./mdlines.js";
import { chatStream, chat, listModels, getProfile, checkApiKey, ApiError } from "./provider.js";
import { TOOL_DEFS, executeTool, executeToolAsync } from "./tools.js";
import { AGENTS, getAgent, primaryAgents, subAgents, filterTools, agentSystem } from "./agents.js";
import { listSkills, skillPromptBlock } from "./skills.js";
import ActivityPanel, { activityRowCount } from "./ActivityPanel.jsx";
import { fmtDuration, useAnimationTime } from "./anim.js";
import { execSync } from "node:child_process";
import {
  getContextWindow, compactThreshold, requestHistoryBudget, COMPACT_RATIO,
  estimateTokenCount, estimateMessagesTokens, fitConversation, buildCompactionConversation,
} from "./context.js";
import {
  loadConfig,
  saveConfig,
  getActiveProvider,
  getProviderApiKey,
  setProviderApiKey,
  setActiveProvider,
  upsertProvider,
  removeProvider,
  setThinking,
  CONFIG_DIR,
  CONFIG_FILE,
} from "./config.js";
import { initDb, closeDb, dbReady, dbLoadSessions, dbSaveSession, dbDeleteSession, dbDeleteAllSessions, dbSessionCount, DB_FILE } from "./db.js";
import fs from "node:fs";
import path from "node:path";

const fmtTime = (ts) => {
  const d = new Date(ts);
  const p = (n) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};

/* reasoning effort 滑块级别(索引即强度,high 及以上标签带动画,max 蓝色波纹) */
const EFFORT_LEVELS = ["low", "medium", "high", "xhigh", "max"];
/* Claude Code ultracode 式独立 Mode:不在 5 档刻度上,分隔线右侧另占一档。
 * supercode = max 推理 + 多子 agent 并发编排(模型自主 delegate 拆解) */
const SUPERCODE = "supercode";
const SUPERCODE_PROMPT = `\n\n## Supercode 模式\n你正处于 supercode 模式:推理 effort=max,并开启多子 agent 并发编排。\n对每个实质任务,先自主规划:把任务拆分为可并行的独立工作流(理解→实施→验证),在同一轮回复中多次并发调用 delegate 子 agent(explorer/general/coding)执行;全部返回后汇总评估、修正补做,直到任务彻底完成才结束回合。`;

/* HSL → hex(滑块/横幅渐变动画共用) */
function hslToHex(h, s, l) {
  h = ((h % 360) + 360) % 360;
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = l - c / 2;
  let r = 0, g = 0, b = 0;
  if (h < 60) [r, g, b] = [c, x, 0];
  else if (h < 120) [r, g, b] = [x, c, 0];
  else if (h < 180) [r, g, b] = [0, c, x];
  else if (h < 240) [r, g, b] = [0, x, c];
  else if (h < 300) [r, g, b] = [x, 0, c];
  else [r, g, b] = [c, 0, x];
  const to = (v) => Math.round((v + m) * 255).toString(16).padStart(2, "0");
  return `#${to(r)}${to(g)}${to(b)}`;
}

/* 两个 hex 颜色按 f∈[0,1] 插值(波纹衰减/呼吸共用) */
function hexInterp(a, b, f) {
  const pa = parseInt(a.slice(1), 16);
  const pb = parseInt(b.slice(1), 16);
  const mix = (x, y) => Math.round(x + (y - x) * f);
  const r = mix((pa >> 16) & 255, (pb >> 16) & 255);
  const g = mix((pa >> 8) & 255, (pb >> 8) & 255);
  const bl = mix(pa & 255, pb & 255);
  return `#${((r << 16) | (g << 8) | bl).toString(16).padStart(6, "0")}`;
}

/* 波纹预设:max 蓝色(稀疏平缓),supercode 紫色(更多环/更强/波峰更宽,填满整个面板) */
const RIPPLE_PRESETS = {
  max:       { N: 5, PERIOD: 3600, amp: 0.8, w0: 3.0, wg: 4.5, decay: 2.4, from: "#0A1E33", to: "#4FC3F7" },
  supercode: { N: 8, PERIOD: 3200, amp: 1.0, w0: 4.5, wg: 6.0, decay: 1.9, from: "#170A33", to: "#C084FC" },
};

/* 波纹(panel 全域背景):从标签文字中心发出,波峰缓慢扩散,先快后慢、阻尼衰减渐隐,
 * 填满整个面板。返回颜色 hex 或 null;fade∈[0,1] 为切入淡入系数。 */
function rippleBgAt(x, y, t, w, cx, cy, pr, fade = 1) {
  const d = Math.hypot(x - cx, (y - cy) * 1.8);
  const maxR = Math.hypot(Math.max(cx, w - cx) + 4, 8 * 1.8);
  const { N, PERIOD, amp, w0, wg, decay, from, to } = pr;
  const STEP = PERIOD / N;
  let best = 0;
  for (let k = 0; k < N; k++) {
    const age = ((t - k * STEP) % PERIOD + PERIOD) % PERIOD;
    const p = age / PERIOD;
    const r = maxR * Math.pow(p, 0.8);
    const ringW = w0 + p * wg;
    const diff = Math.abs(d - r);
    if (diff < ringW) {
      const a = (1 - diff / ringW) * amp * Math.exp(-decay * p) * fade;
      if (a > best) best = a;
    }
  }
  return best > 0.04 ? hexInterp(from, to, best) : null;
}

const COMMANDS = [
  { cmd: "/help", desc: "显示所有命令" },
  { cmd: "/connect", desc: "接入提供商（自定义 API 服务）" },
  { cmd: "/provider", desc: "切换提供商（/provider <name>）" },
  { cmd: "/key", desc: "设置当前提供商 API Key（/key <sk-xxx>）" },
  { cmd: "/model", desc: "切换模型（/model <id>）" },
  { cmd: "/thinking", desc: "开启/关闭 thinking 展示" },
  { cmd: "/effort", desc: "调整推理 effort（low/medium/high/xhigh/max）" },
  { cmd: "/agent", desc: "列出/切换 agent（Tab 也切换）" },
  { cmd: "/quota", desc: "查询本地网关余额（仅本地提供商）" },
  { cmd: "/context", desc: "查看上下文占用与压缩阈值" },
  { cmd: "/compact", desc: "立即压缩上下文（摘要历史）" },
  { cmd: "/todos", desc: "显示当前任务清单" },
  { cmd: "/cd", desc: "切换工作目录" },
  { cmd: "/pwd", desc: "显示工作目录" },
  { cmd: "/new", desc: "新建会话（/new <名字>）" },
  { cmd: "/sessions", desc: "会话列表与切换（↑↓ 选择 · Enter 切换）" },
  { cmd: "/delete", desc: "删除会话（/delete <编号或名称>）" },
  { cmd: "/storage", desc: "存储方案互转（/storage db 迁入数据库 · /storage config 写回 config.json）" },
  { cmd: "/diff", desc: "代码改动审阅（git diff · +绿/-红）" },
  { cmd: "/skills", desc: "技能系统（/skills <名称> 加载指令）" },
  { cmd: "/clear", desc: "清空当前会话历史" },
  { cmd: "/restore", desc: "恢复出厂设置（删除所有会话数据）" },
  { cmd: "/exit", desc: "退出" },
];

/* 会话拾取器(/sessions):↑↓ 选择高亮,Enter 切换 */
function SessionPicker({ sessions, activeSessionId, index, width }) {
  if (!sessions.length) return null;
  const fmtAgo = (ts) => {
    const s = Math.max(0, (Date.now() - ts) / 1000);
    if (s < 60) return "刚刚";
    if (s < 3600) return `${Math.round(s / 60)} 分钟前`;
    if (s < 86400) return `${Math.round(s / 3600)} 小时前`;
    return `${Math.round(s / 86400)} 天前`;
  };
  const cur = sessions[Math.min(index, sessions.length - 1)];
  return (
    <Box flexDirection="column" marginBottom={1} borderStyle="round" borderColor="cyan" paddingX={1}>
      <Text bold color="cyan" wrap="truncate" width={Math.max(width, 10)}>
        ▸ 会话列表（{sessions.length}）· ↑↓ 选择 · Enter 切换 · Esc 取消
      </Text>
      {sessions.slice(0, 8).map((s, i) => {
        const sel = cur?.id === s.id;
        const count = (s.history || []).length;
        return (
          <Text key={s.id} color={s.id === activeSessionId ? "green" : "white"}
            bold={sel} dim={!sel} wrap="truncate" width={Math.max(width, 10)}>
            {`  ${i + 1}. ${s.name}${s.id === activeSessionId ? "（当前）" : ""}${sel ? " ◂" : ""} · ${count} 条 · ${s.agentId || "build"}${fmtAgo(s.updatedAt) ? " · " + fmtAgo(s.updatedAt) : ""}`}
          </Text>
        );
      })}
    </Box>
  );
}

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
    <Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={2} paddingY={1} marginBottom={1} height={8}>
      <Text bold color="cyan">连接提供商 ({step + 1}/3)</Text>
      {step === 0 && (
        <>
          <Text dimColor>名称:</Text>
          <TextInput value={name} onChange={setName} onSubmit={() => submit()} placeholder="如: 我的中转站" />
          <Text dimColor>接口地址 (OpenAI 兼容 base, 如 https://xxx.com/v1):</Text>
          <Box flexShrink={0}>
            <TextInput value={baseUrl} onChange={setBaseUrl} onSubmit={() => submit()} placeholder="https://api.example.com/v1" />
          </Box>
        </>
      )}
      {step === 1 && (
        <>
          <Text dimColor>模型列表 (逗号分隔):</Text>
          <TextInput value={models} onChange={setModels} onSubmit={() => submit()} placeholder="model-a, model-b" />
          <Text dimColor>Enter 继续 · Esc 取消</Text>
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
  const [showBanner, setShowBanner] = useState(false); // 启动/新会话横幅(opencode 风格)
  const [effort, setEffort] = useState(() => loadConfig().effort || "high"); // reasoning effort
  const [effortIdx, setEffortIdx] = useState(2); // effort 滑块当前位置
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
  const [streaming, setStreaming] = useState(null); // 当前流式消息
  /* 输入历史:↑/↓ 回溯,Enter 提交时记录 */
  const inputHistoryRef = useRef([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  /* 命令面板:匹配时 ↑/↓ 切换高亮,Enter 选中 */
  const [paletteIndex, setPaletteIndex] = useState(0);

  /* ===== 多会话管理:每个会话独立 history/conversation/agent/cwd =====
   * 存储:storageRef.current === "db" → SQLite(ux-agent.db);否则兼容 config.json */
  const [sessions, setSessions] = useState(cfg.sessions || []);
  const [activeSessionId, setActiveSessionId] = useState(cfg.activeSessionId || null);
  const [showSessionList, setShowSessionList] = useState(false); // /sessions 拾取器
  const [sessionIndex, setSessionIndex] = useState(0);
  const storageRef = useRef(cfg.storage === "db" && dbReady());
  const pendingLegacyRef = useRef(null); // 待迁移的旧 config.json 会话数据
  const [migrateInput, setMigrateInput] = useState("");
  const [restoreInput, setRestoreInput] = useState("");
  const sessionsRef = useRef(cfg.sessions || []);
  const activeSessionIdRef = useRef(cfg.activeSessionId || null);
  const messagesRef = useRef([]);
  const conversationRef = useRef(cfg.conversation || []);
  useEffect(() => { sessionsRef.current = sessions; }, [sessions]);
  useEffect(() => { messagesRef.current = messages; }, [messages]);
  useEffect(() => { conversationRef.current = conversation; }, [conversation]);

  /* ===== 多子 agent 会话:delegate/@name 可并发创建,⇄ 键切换查看 ===== */
  const [subSessions, setSubSessions] = useState({}); // id -> {id, agentId, task, status, streaming, busy, done, error, result, tokens, startedAt}
  const [activeSubId, setActiveSubId] = useState(null); // 正在查看的子会话
  const [subView, setSubView] = useState(false); // 是否切到子聊天区
  const [subInput, setSubInput] = useState("");
  const [subScrollOffset, setSubScrollOffset] = useState(0);
  const subMsgsRef = useRef({}); // id -> 权威 API 消息序列
  const subAgentRef = useRef({}); // id -> agentId
  const subStartedRef = useRef({}); // id -> Date.now
  const subTokensRef = useRef({}); // id -> token count
  const subScrollRef = useRef({}); // id -> scroll offset
  const subSessionsRef = useRef({}); // 供 useInput 读取最新快照
  useEffect(() => { subSessionsRef.current = subSessions; }, [subSessions]);

  /* ===== 任务清单 (todo_write / todo_update 可视化) ===== */
  const [todos, setTodos] = useState([]);
  const [showTodos, setShowTodos] = useState(true);
  const todosRef = useRef([]);

  /* ===== Claude Code 风格活动动画状态 ===== */
  const [activity, setActivity] = useState(null); // {kind: thinking|tool|delegate|compacting|subagents, target}
  const busySinceRef = useRef(0);
  const turnTokensRef = useRef(0);
  const lastCompactAtRef = useRef(0);
  const [historyUsed, setHistoryUsed] = useState(0); // 当前上下文估算 token

  /* opencode 风格:工具输出是对话流里的内联可折叠块,不进气泡文本。
   * 折叠时每块只占 1 行(Ctrl+E 切换展开/折叠),天然不挤占聊天区。 */
  const [showToolDetails, setShowToolDetails] = useState(false);

  const aborter = useRef(null);
  const cancelRequestedRef = useRef(false);
  const toastTimer = useRef(null);
  const maxSinceRef = useRef(0); // 最近一次切入 max 档的时刻(波纹淡入用)
  const superSinceRef = useRef(0); // 最近一次切入 supercode 的时刻(紫色波纹淡入用)

  /* 终端动态尺寸:resize 时自动重算,布局吃满终端 */
  const [termSize, setTermSize] = useState(() => [stdout.columns || 100, stdout.rows || 30]);
  useEffect(() => {
    const onResize = () => setTermSize([stdout.columns || 100, stdout.rows || 30]);
    stdout.on("resize", onResize);
    return () => stdout.off("resize", onResize);
  }, [stdout]);

  const WIDTH = termSize[0];
  const HEIGHT = termSize[1];
  /* 动态布局:状态栏2 + 外框border2 + 输入区3 + 快捷键1 = 8 固定,
   * 再叠加命令面板/弹窗/活动动画面板的精确行数,消息区吃掉剩余空间。 */
  const subList = Object.values(subSessions);
  const modalLines = mode === "connect" ? 8 : mode === "login" ? 6 : mode === "model" ? 7 : mode === "migrate" ? 6 : mode === "effort" ? 6 : 0;
  const showPalette = showCommands && input.startsWith("/") && mode === "chat";
  const paletteLines = showPalette
    ? Math.min(COMMANDS.filter((c) => c.cmd.includes(input.slice(1).toLowerCase())).length, 6)
    : 0;
  const sessionListLines = showSessionList ? Math.min(sessions.length, 8) + 2 : 0;
  const activityLines = activityRowCount({ busy, subs: subList, todos, showTodos });
  const baseFixed = 8 + modalLines + paletteLines + sessionListLines + activityLines;
  const MSG_HEIGHT = Math.max(HEIGHT - baseFixed, 10);
  /* 子聊天区:固定 6 行开销(框边2 + 题头1 + 输入2 + 快捷键1) */
  const SUB_MSG_HEIGHT = Math.max(HEIGHT - 7, 10);
  const rowWidth = Math.max(WIDTH - 4, 16);

  const provider = getActiveProvider();
  const agent = getAgent(agentId);

  /* 子会话被清空时自动退出子聊天区 */
  useEffect(() => {
    if (subView && (!activeSubId || !subSessions[activeSubId])) {
      setSubView(false);
      setActiveSubId(null);
    }
  }, [subView, activeSubId, subSessions]);

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

  /* 按存储模式落盘会话列表：
   * db = 会话写 SQLite,config.json 只留 activeSessionId(瘦身);
   * config = 兼容模式,继续写 history/conversation/sessions 到 config.json。
   * changedId 指定仅重写该会话(热路径),否则写全量列表。 */
  const persistSessionList = useCallback((list, activeSid, changedId) => {
    if (storageRef.current === "db") {
      if (changedId) {
        const s = list.find((x) => x.id === changedId);
        if (s) dbSaveSession(s);
      } else {
        for (const s of list) dbSaveSession(s);
      }
      saveConfig({ activeSessionId: activeSid || null });
    } else {
      const active = list.find((s) => s.id === activeSid) || list[0] || null;
      saveConfig({
        history: active?.history || [],
        conversation: active?.conversation || [],
        sessions: list,
        activeSessionId: activeSid || null,
        storage: "config",
      });
    }
  }, []);

  /* 持久化:写入当前活动会话(两侧存储一致) */
  const persist = useCallback((msgs, conv) => {
    const history = msgs
      .filter((m) => m.role === "user" || m.role === "assistant")
      .map((m) => ({ role: m.role, content: m.content, time: m.time, reasoning: m.reasoning }))
      .slice(-200);
    const conversation = (conv || conversationRef.current).slice(-200);
    const sid = activeSessionIdRef.current;
    const updated = sid
      ? sessionsRef.current.map((s) => (s.id === sid ? { ...s, history, conversation, agentId, cwd, updatedAt: Date.now() } : s))
      : sessionsRef.current;
    setSessions(updated);
    persistSessionList(updated, sid, sid);
  }, [agentId, cwd, persistSessionList]);

  /* 切换到指定会话(先落盘当前会话,再载入目标会话) */
  const switchSession = useCallback((sid, saveCur = true) => {
    if (busy) { toast("请等待当前任务完成"); return; }
    const cur = sessionsRef.current;
    let next = cur;
    if (saveCur && activeSessionIdRef.current) {
      const curMsgs = messagesRef.current;
      const curConv = conversationRef.current;
      next = cur.map((s) => s.id === activeSessionIdRef.current
        ? {
          ...s,
          history: curMsgs.filter((m) => m.role === "user" || m.role === "assistant")
            .map((m) => ({ role: m.role, content: m.content, time: m.time, reasoning: m.reasoning }))
            .slice(-200),
          conversation: curConv.slice(-200),
          agentId, cwd, updatedAt: Date.now(),
        }
        : s);
    }
    const target = next.find((s) => s.id === sid);
    if (!target) { toast("会话不存在"); return; }
    clearScreen();
    setSubView(false); setActiveSubId(null); setSubSessions({});
    setMessages(target.history.length
      ? target.history.map((m) => ({ ...m, time: m.time || Date.now() }))
      : [{ role: "assistant", content: "新会话已开始。", time: Date.now() }]);
    setConversation(target.conversation || []);
    conversationRef.current = target.conversation || [];
    setHistoryUsed(estimateMessagesTokens(target.conversation || []));
    if (target.agentId) setAgentId(target.agentId);
    if (target.cwd) setCwd(target.cwd);
    setScrollOffset(0);
    setSessions(next);
    setActiveSessionId(sid);
    activeSessionIdRef.current = sid;
    setShowSessionList(false);
    setSessionIndex(0);
    persistSessionList(next, sid);
    setStatus(`已切换到会话: ${target.name}`);
  }, [busy, toast, clearScreen, agentId, cwd, persistSessionList]);

  const refreshProfile = useCallback(async () => {
    try { return await getProfile(); } catch { return null; }
  }, []);

  /* 从 cfg/数据库恢复会话与上下文(dbMode=true 走 SQLite,否则兼容 config.json) */
  const restoreApp = useCallback((cfg, dbMode) => {
    const sessList = dbMode ? dbLoadSessions() : cfg.sessions || [];
    setSessions(sessList);
    sessionsRef.current = sessList;
    const active = sessList.find((s) => s.id === cfg.activeSessionId) || sessList[0] || null;
    setActiveSessionId(active?.id || null);
    activeSessionIdRef.current = active?.id || null;
    const hist = active ? active.history : cfg.history || [];
    setShowBanner(!hist.length);
    if (hist.length) {
      setMessages(hist.map((m) => ({ ...m, time: m.time || Date.now() })));
    } else {
      setMessages([{
        role: "assistant",
        content: "你好，我是 **Uinxed AI Agent**。支持多提供商（`/provider`）、工具调用、thinking 展示。输入 `/` 查看命令。",
        time: Date.now(),
      }]);
    }
    /* 恢复 API 深度上下文(不含 tool 块,仅 user/assistant):否则重启后模型失去前文。
     * historyUsed 同步恢复,让状态栏 ctx% 从启动起就准确。 */
    const conv = active ? active.conversation : cfg.conversation || [];
    setConversation(conv);
    conversationRef.current = conv;
    if (conv.length) setHistoryUsed(estimateMessagesTokens(conv));
    if (active?.cwd) setCwd(active.cwd);
    const p = getActiveProvider();
    if (getProviderApiKey(p.id)) {
      refreshProfile().then((u) => {
        setStatus(u ? `已登录 ${u.username}${u.unlimited ? "（无限额度）" : `，余额 ¥${(u.quota || 0).toFixed(2)}`}` : `已连接 ${p.name}`);
      });
    } else {
      setStatus(`${p.name} 未设置 Key · 提供商 ${p.name}（${p.baseUrl}）`);
    }
  }, [refreshProfile]);

  /* 存储方案互转:config ←→ 数据库。返回切换布尔结果 */
  const changeStorage = useCallback((target) => {
    const cur = storageRef.current ? "db" : "config";
    if (target === cur) { setStatus(target === "db" ? "已是 SQLite 数据库模式" : "已是 config.json 模式"); return; }
    try {
      const cfg = loadConfig();
      if (target === "db") {
        /* config → 数据库:会话写入 SQLite,config.json 瘦身 */
        const list = sessionsRef.current;
        for (const s of list) {
          dbSaveSession({
            id: s.id, name: s.name || s.id, agentId: s.agentId || "build",
            cwd: s.cwd || null, updatedAt: s.updatedAt || Date.now(),
            history: s.history || [], conversation: s.conversation || [],
          });
        }
        const clean = { ...cfg };
        delete clean.history; delete clean.conversation; delete clean.sessions;
        clean.storage = "db";
        clean.activeSessionId = cfg.activeSessionId || list[0]?.id || null;
        fs.mkdirSync(CONFIG_DIR, { recursive: true });
        fs.writeFileSync(CONFIG_FILE, JSON.stringify(clean, null, 2), "utf8");
        storageRef.current = true;
        setStatus(`已迁移 ${list.length} 个会话到 SQLite 数据库（config.json 已瘦身）`);
      } else {
        /* 数据库 → config:会话写回 config.json,删除旧数据库 */
        const list = dbLoadSessions();
        if (!list.length) { setStatus("数据库没有会话，无需切回 config.json"); return; }
        const activeId = cfg.activeSessionId || list[0]?.id || null;
        const active = list.find((s) => s.id === activeId) || list[0] || null;
        const clean = {
          ...cfg, storage: "config", sessions: list, activeSessionId: activeId,
          history: active?.history || [], conversation: active?.conversation || [],
        };
        fs.mkdirSync(CONFIG_DIR, { recursive: true });
        fs.writeFileSync(CONFIG_FILE, JSON.stringify(clean, null, 2), "utf8");
        closeDb();
        for (const f of [DB_FILE, DB_FILE + "-wal", DB_FILE + "-shm"]) {
          try { fs.rmSync(f, { force: true }); } catch {}
        }
        storageRef.current = false;
        setSessions(list); sessionsRef.current = list;
        setStatus(`已切换为 config.json 存储（${list.length} 个会话；旧数据库已删除）`);
      }
    } catch (e) {
      setStatus(`存储切换失败: ${e.message}`);
    }
  }, []);

  /* 迁移选择:y/Enter = 迁移到 SQLite;n = 继续用 config.json */
  const onMigrateSubmit = useCallback((v) => {
    const ans = String(v || "").trim().toLowerCase();
    if (ans === "n") {
      saveConfig({ storage: "config", migratePrompted: true });
      setMode("chat");
      restoreApp(loadConfig(), false);
      setStatus("已选择继续使用 config.json（可随时用 /storage 迁入数据库）");
      return;
    }
    changeStorage("db");
    const ok = storageRef.current === true;
    setMode("chat");
    restoreApp(loadConfig(), ok);
    setStatus(ok ? "已迁移到数据库（可随时用 /storage config 切回）" : "迁移失败，继续使用 config.json");
  }, [restoreApp, changeStorage]);

  const onRestoreSubmit = useCallback((v) => {
    const ans = String(v || "").trim().toLowerCase();
    if (ans !== "y" && ans !== "yes") {
      setMode("chat");
      setStatus("已取消恢复");
      return;
    }
    dbDeleteAllSessions();
    closeDb();
    try { fs.rmSync(DB_FILE, { force: true }); } catch {}
    try { fs.rmSync(CONFIG_FILE, { force: true }); } catch {}
    setSessions([]);
    sessionsRef.current = [];
    setActiveSessionId(null);
    activeSessionIdRef.current = null;
    setMessages([{ role: "assistant", content: "已恢复出厂设置，所有数据已清除。", time: Date.now() }]);
    setShowBanner(true);
    setConversation([]);
    conversationRef.current = [];
    setHistoryUsed(0);
    setMode("chat");
    setStatus("已恢复出厂设置");
  }, []);

  useEffect(() => {
    clearScreen();
    initDb();
    const cfg = loadConfig();
    const legacyCount = (cfg.sessions || []).length;
    if (legacyCount && !cfg.migratePrompted) {
      /* 检测到旧 config.json 会话数据 → 启动时提示迁移(仅一次) */
      pendingLegacyRef.current = cfg;
      setStatus(`检测到 config.json 中的 ${legacyCount} 个旧会话，是否迁移到数据库？`);
      setMode("migrate");
      return;
    }
    if (cfg.storage === "db") {
      restoreApp(cfg, true);
      return;
    }
    restoreApp(cfg, false);
  }, [restoreApp, clearScreen]);

  /* ============ 多子 Agent 会话(供 delegate/@name 使用) ============ */
  const updateSub = useCallback((sid, patch) => {
    setSubSessions((prev) => (prev[sid] ? { ...prev, [sid]: { ...prev[sid], ...patch } } : prev));
  }, []);

  /* 注册一个子会话,立即返回 id(不阻塞) */
  const startSubAgent = useCallback((subAgentName, task) => {
    const id = `sub-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
    subMsgsRef.current[id] = [
      { role: "system", content: agentSystem(getAgent(subAgentName), loadConfig().model) },
      { role: "user", content: task },
    ];
    subAgentRef.current[id] = subAgentName;
    subStartedRef.current[id] = Date.now();
    subTokensRef.current[id] = 0;
    subScrollRef.current[id] = 0;
    setSubSessions((prev) => ({
      ...prev,
      [id]: {
        id, agentId: subAgentName, task,
        status: "运行中", streaming: null,
        busy: true, done: false, error: null, result: null,
        tokens: 0, startedAt: Date.now(),
      },
    }));
    return id;
  }, []);

  /* 任务清单操作(供工具回调) */
  const todoWrite = useCallback((list) => {
    const next = list.map((t, i) => ({ ...t, id: t.id ?? i + 1 }));
    todosRef.current = next;
    setTodos(next);
    return { ok: true, count: next.length, todos: next.map((t) => ({ id: t.id, subject: t.subject, status: t.status })) };
  }, []);

  const todoUpdate = useCallback(({ index, subject, status }) => {
    const list = todosRef.current;
    let idx = -1;
    if (index && index >= 1) idx = list.findIndex((t) => t.id === index);
    if (idx < 0 && subject) idx = list.findIndex((t) => t.subject === subject);
    if (idx < 0) return { error: "未找到该任务,请先用 todo_write 建立清单(/todos 查看)" };
    const next = list.map((t, i) => (i === idx ? { ...t, status } : t));
    todosRef.current = next;
    setTodos(next);
    return { ok: true, updated: { subject: next[idx].subject, status: next[idx].status } };
  }, []);

  /* 工具块写入消息流(opencode 风格内联块):运行中占 1 行,完成后可展开输出 */
  const pushToolBlock = useCallback((entry) => {
    const id = entry.id || `t${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`;
    setMessages((m) => [...m, {
      role: "tool", id, agentName: entry.agent || null,
      tool: entry.tool, args: entry.args ?? null,
      status: entry.status || "running",
      output: entry.output ?? null, error: entry.error ?? null,
      time: entry.time || Date.now(), dur: entry.dur || 0,
    }]);
    return id;
  }, []);

  const updateToolBlock = useCallback((id, patch) => {
    setMessages((m) => m.map((x) => (x.id === id && x.role === "tool" ? { ...x, ...patch } : x)));
  }, []);

  /* 工具结果 → 可读文本(终端输出优先,对象兜底 JSON) */
  const toolOutputText = (m) => {
    if (m.error) return String(m.error);
    const o = m.output;
    if (o == null) return "";
    if (typeof o === "string") return o;
    if (o.stdout != null || o.stderr != null) {
      const parts = [];
      if (o.stderr) parts.push(`[stderr]\n${o.stderr}`);
      if (o.stdout) parts.push(o.stdout);
      if (o.exitCode != null && o.exitCode !== 0) parts.push(`[exit ${o.exitCode}]`);
      return parts.length ? parts.join("\n") : "(无输出)";
    }
    try { return JSON.stringify(o, null, 2); } catch { return String(o); }
  };

  const toolArgsBrief = (args) => {
    if (args == null) return "";
    if (typeof args === "string") return args;
    if (args.command) return args.command;
    if (args.path) return args.path;
    if (args.pattern) return args.pattern;
    if (args.subject) return args.subject;
    if (args.agent) return args.agent + (args.task ? ` — ${args.task}` : "");
    try { return JSON.stringify(args).slice(0, 80); } catch { return ""; }
  };

  /* 子 agent 多轮循环:返回 { agent, result } 供主 agent 工具结果回传 */
  const runSubAgentLoop = useCallback(async (sid, subAgentName, task, signal) => {
    const sub = getAgent(subAgentName);
    if (!sub || (sub.role !== "subagent" && sub.role !== "both")) return { error: `未知子 agent: ${subAgentName}` };
    const subTools = filterTools(TOOL_DEFS, sub).filter((d) => d.function.name !== "delegate");
    let msgs = subMsgsRef.current[sid];
    if (!msgs) {
      msgs = [{ role: "system", content: agentSystem(sub, loadConfig().model) }, { role: "user", content: task }];
      subMsgsRef.current[sid] = msgs;
      subTokensRef.current[sid] = 0;
    }
    let subAcc = { content: "", reasoning: "" };
    let subLastFlush = 0;
    for (let step = 0; ; step++) {
      let finalRes = null;
      subAcc = { content: "", reasoning: "" };
      updateSub(sid, { status: `运行中 (${step + 1} 步)` });
      try {
        for await (const chunk of chatStream(msgs, { tools: subTools, signal })) {
          if (chunk.reasoning) subAcc.reasoning += chunk.reasoning;
          if (chunk.content) {
            subAcc.content += chunk.content;
            const now = Date.now();
            if (now - subLastFlush > 120 || chunk.content.includes("\n")) {
              updateSub(sid, { streaming: { content: subAcc.content, reasoning: subAcc.reasoning } });
              subLastFlush = now;
            }
          }
          if (chunk.finishReason || chunk.done) finalRes = chunk;
        }
        subTokensRef.current[sid] = (subTokensRef.current[sid] || 0) + estimateMessagesTokens(
          [{ content: subAcc.content + subAcc.reasoning }]
        );
        updateSub(sid, { tokens: subTokensRef.current[sid] });
      } catch (e) {
        if (e.name === "AbortError" || signal?.aborted) {
          updateSub(sid, { busy: false, done: true, status: "已停止", error: null, streaming: null });
          return { agent: sub.name, cancelled: true, result: null };
        }
        updateSub(sid, { busy: false, done: true, status: "出错", error: e.message, streaming: null });
        return { agent: sub.name, error: e.message, result: null };
      }
      const toolCalls = finalRes?.toolCalls || [];
      if (!toolCalls.length) {
        const out = subAcc.content || "(空回复)";
        const finalMsg = { role: "assistant", content: out };
        if (subAcc.reasoning) finalMsg.reasoning_content = subAcc.reasoning;
        subMsgsRef.current[sid] = [...msgs, finalMsg];
        updateSub(sid, { busy: false, done: true, status: "完成", result: out, streaming: null });
        return { agent: sub.name, result: out };
      }
      const subAssistantMsg = { role: "assistant", content: subAcc.content, tool_calls: [] };
      if (subAcc.reasoning) subAssistantMsg.reasoning_content = subAcc.reasoning;
      const subResults = await Promise.all(toolCalls.map(async (tc, ti) => {
        if (!tc.id) tc.id = `call_${Date.now()}_${ti}`;
        subAssistantMsg.tool_calls.push(tc);
        let parsed = {};
        try { parsed = JSON.parse(tc.function.arguments || "{}"); } catch {}
        updateSub(sid, { status: `执行 ${tc.function.name}…` });
        const tStart = Date.now();
        let r;
        try {
          r = await executeToolAsync(tc.function.name, parsed, cwd, { todoWrite, todoUpdate });
        } catch (e) {
          r = { error: e.message };
        }
        subTokensRef.current[sid] = (subTokensRef.current[sid] || 0) + estimateTokenCount(JSON.stringify(r));
        return { role: "tool", tool_call_id: tc.id, content: JSON.stringify(r).slice(0, 12000) };
      }));
      if (signal?.aborted) {
        updateSub(sid, { busy: false, done: true, status: "已停止", streaming: null });
        return { agent: sub.name, cancelled: true, result: null };
      }
      subMsgsRef.current[sid] = msgs = [...msgs, subAssistantMsg, ...subResults];
      updateSub(sid, { streaming: null });
    }
  }, [cwd, updateSub, todoWrite, todoUpdate]);

  /* 子聊天区继续对话:把用户新消息追加进指定子会话再跑循环 */
  const continueSubConversation = useCallback((sid, text) => {
    if (!String(text || "").trim() || subSessionsRef.current[sid]?.busy) return;
    const agentId = subAgentRef.current[sid] || "general";
    subMsgsRef.current[sid] = [...(subMsgsRef.current[sid] || []), { role: "user", content: text.trim() }];
    updateSub(sid, { busy: true, done: false, status: "运行中" });
    setSubInput("");
    cancelRequestedRef.current = false;
    const controller = new AbortController();
    aborter.current = controller;
    return runSubAgentLoop(sid, agentId, text.trim(), controller.signal).finally(() => {
      if (aborter.current === controller) aborter.current = null;
    });
  }, [runSubAgentLoop, updateSub]);

  /* ============ Agent 循环(流式 + 工具) ============ */

  /* 上下文压缩:超过阈值时让模型把历史摘要成结构化总结,替换 conversation。
   * 返回 {compacted, est, window, newConv, summary}
   * manual=true 时跳过阈值;摘要通过 chatStream 流式输出到聊天区。 */
  const compressConversation = useCallback(async (conv, manual = false) => {
    const model = loadConfig().model;
    const window = getContextWindow(model);
    const est = estimateMessagesTokens(conv);
    if (!manual && est < compactThreshold(model)) return { compacted: false, est, window };
    const prevActivity = activity;
    setActivity({ kind: "compacting", target: "" });
    if (manual) {
      setStatus(`压缩中…（≈${Math.round(est / 1000)}k tok）`);
    } else {
      setStatus(`上下文已满（≈${Math.round(est / 1000)}k tok）,压缩中…`);
    }
    /* 先插入占位消息,后续流式填充 */
    const compactId = `compact-${Date.now()}`;
    setMessages((m) => [...m, { id: compactId, role: "assistant", agentName: "compact", content: "", time: Date.now() }]);
    let acc = "";
    let lastFlush = 0;
    try {
      for await (const chunk of chatStream(buildCompactionConversation(conv), { model })) {
        if (chunk.content) {
          acc += chunk.content;
          const now = Date.now();
          if (now - lastFlush > 80 || chunk.content.includes("\n")) {
            setMessages((m) => m.map((x) => x.id === compactId ? { ...x, content: acc } : x));
            lastFlush = now;
          }
        }
      }
      /* 最终 flush */
      setMessages((m) => m.map((x) => x.id === compactId ? { ...x, content: acc } : x));
      const summary = acc.trim() || "(压缩摘要为空)";
      const newConv = [
        { role: "system", content: `[上下文已压缩]\n\n以下是此前对话的结构化摘要:\n\n${summary}` },
        ...conv.slice(-6),
      ];
      setConversation(newConv);
      setHistoryUsed(estimateMessagesTokens(newConv));
      setActivity(prevActivity);
      return { compacted: true, est, window, newConv, summary };
    } catch (e) {
      /* 失败时删除占位消息 */
      setMessages((m) => m.filter((x) => x.id !== compactId));
      setStatus(`压缩失败: ${e.message}`);
      setActivity(prevActivity);
      return { compacted: false, est, window, error: e.message };
    }
  }, [activity]);

  const runAgent = useCallback(
    async (userText, targetAgent, isSub = false) => {
      const active = isSub ? getAgent(targetAgent) : agent;
      const toolDefs = filterTools(TOOL_DEFS, active);
      const currentProvider = getActiveProvider();
      if (!getProviderApiKey(currentProvider.id) && currentProvider.requiresAuth !== false) {
        setMode("login");
        setStatus(`请为 ${currentProvider.name} 输入 API Key`);
        setBusy(false);
        return;
      }

      /* 按模型上下文窗口预算裁剪历史(deepseek 1M / glm 128k) */
      const hist = fitConversation(conversation, requestHistoryBudget(loadConfig().model));
      const msgs = [
        { role: "system", content: agentSystem(active, loadConfig().model)
          + (!isSub && loadConfig().effort === SUPERCODE ? SUPERCODE_PROMPT : "")
          + skillPromptBlock(cwd) },
        ...hist,
        { role: "user", content: userText },
      ];

      const streamId = Date.now() + "-" + Math.random().toString(36).slice(2, 6);
      let streamAcc = { content: "", reasoning: "" };
      let lastFlush = 0;
      let conversationAdded = false;
      busySinceRef.current = Date.now();
      turnTokensRef.current = 0;
      setHistoryUsed(estimateMessagesTokens(msgs));
      cancelRequestedRef.current = false;
      const controller = new AbortController();
      aborter.current = controller;

      for (let step = 0; ; step++) {
        setStatus(`${active.name} 思考中… (第 ${step + 1} 次)`);
        setActivity({ kind: "thinking", target: "" });
        let finalRes = null;
        try {
          for await (const chunk of chatStream(msgs, {
            tools: toolDefs,
            signal: controller.signal,
          })) {
            if (chunk.reasoning) {
              streamAcc.reasoning += chunk.reasoning;
            }
            if (chunk.content) {
              streamAcc.content += chunk.content;
              turnTokensRef.current += estimateTokenCount(chunk.content);
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
          if (e.name === "AbortError" || controller.signal.aborted) {
            setStatus("已取消"); setStreaming(null); setBusy(false);
            setActivity(null); turnTokensRef.current = 0;
            if (aborter.current === controller) aborter.current = null;
            return;
          }
          setMessages((m) => [...m, {
            role: "assistant", agentName: active.name,
            content: e instanceof ApiError && e.type === "content_filter" ? "话题被过滤。" : `错误: ${e.message}`,
            time: Date.now(),
          }]);
          setStatus("出错");
          setStreaming(null); setBusy(false);
          setActivity(null); turnTokensRef.current = 0;
          if (aborter.current === controller) aborter.current = null;
          return;
        }

        /* 上下文自动压缩(子 agent 不压缩,避免循环嵌套) */
        const estTokens = estimateMessagesTokens(msgs);
        setHistoryUsed(estTokens);
        if (!isSub && estTokens >= compactThreshold(loadConfig().model) && Date.now() - lastCompactAtRef.current > 20000) {
          lastCompactAtRef.current = Date.now();
          setMessages((m) => [...m, { role: "assistant", agentName: "compact", content: "------ 自动压缩 --------", time: Date.now() }]);
          const res = await compressConversation(msgs.slice(1));
          if (res.compacted) {
            msgs.splice(0, msgs.length, { role: "system", content: agentSystem(active, loadConfig().model) }, ...res.newConv);
            conversationAdded = true; /* 摘要已包含当前用户消息,避免重复前置 */
            setStatus(`✓ 上下文已自动压缩: ${Math.round(estTokens / 1000)}k → ${Math.round(estimateMessagesTokens(res.newConv) / 1000)}k tok`);
            setHistoryUsed(estimateMessagesTokens(msgs));
          }
        }

        const toolCalls = finalRes?.toolCalls || [];
        if (toolCalls.length) {
          /* DeepSeek: assistant 带 tool_calls 时 reasoning_content 必须回传。
           * 同时保留 AI 调用工具前输出的文字，避免被下一轮工具输出覆盖。 */
          const assistantMsg = { role: "assistant", content: streamAcc.content || "", tool_calls: [] };
          assistantMsg.reasoning_content = streamAcc.reasoning || "";
          if (streamAcc.content || streamAcc.reasoning) {
            setMessages((m) => [...m, {
              role: "assistant",
              agentName: active.name,
              content: streamAcc.content || "",
              reasoning: streamAcc.reasoning || undefined,
              time: Date.now(),
            }]);
          }
          /* 同一份内容已经进入历史；立即移除流式副本，避免工具执行期间
           * reasoning/content 同时从 messages 与 streaming 渲染两次。 */
          setStreaming(null);
          /* 同一批工具调用并发执行(delegate 多次调用 = 多个子 agent 并行)。
           * 工具块(opencode 风格)内联进消息流:运行中 1 行,完成后可 Ctrl+E 展开输出。 */
          const toolResults = await Promise.all(toolCalls.map(async (tc, ti) => {
            if (!tc.id) tc.id = `call_${Date.now()}_${ti}`;
            assistantMsg.tool_calls.push(tc);
            let parsed = {};
            try { parsed = JSON.parse(tc.function.arguments || "{}"); } catch {}
            const tStart = Date.now();
            const tid = pushToolBlock({
              agent: active.name, tool: tc.function.name, args: parsed,
              status: "running", time: tStart,
            });
            let result;
            if (tc.function.name === "delegate") {
              /* AI 主动委托子 agent:并发创建多个子会话(⇄ 键可实时查看) */
              const subName = parsed.agent || "general";
              const subTask = parsed.task || "帮我完成这个任务";
              setActivity({ kind: "delegate", target: subName });
              const sid = startSubAgent(subName, subTask);
              const subRes = await runSubAgentLoop(sid, subName, subTask, controller.signal);
              updateSub(sid, {
                busy: false, done: true, status: subRes.cancelled ? "已停止" : subRes.error ? "出错" : "完成",
                result: subRes.result, error: subRes.error || null,
              });
              result = { subagent: subName, output: subRes };
            } else {
              setActivity({ kind: "tool", target: tc.function.name });
              try {
                result = await executeToolAsync(tc.function.name, parsed, cwd, { todoWrite, todoUpdate });
              } catch (e) {
                result = { error: e.message };
              }
            }
            updateToolBlock(tid, {
              status: result?.error ? "error" : "ok",
              output: result,
              error: result?.error || null,
              dur: Date.now() - tStart,
            });
            let resultText = JSON.stringify(result, null, 2).slice(0, 10000);
            if (tc.function.name === "delegate") {
              /* 委托返回后明确要求主 agent 继续推进,避免其就此结束回合 */
              resultText += "\n\n[子 agent 已完成并返回结果。请评估该结果并继续推进整体任务：必要时修正、验证或委托下一个子任务，直到用户任务全部完成；完成后给出最终总结。不要在此结束回合。]";
            }
            return { role: "tool", tool_call_id: tc.id, content: resultText };
          }));
          if (cancelRequestedRef.current || controller.signal.aborted) {
            setStatus("已取消"); setStreaming(null); setBusy(false);
            setActivity(null); turnTokensRef.current = 0;
            if (aborter.current === controller) aborter.current = null;
            return;
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
          setMessages((m) => {
            const updated = [...m, finalMsg];
            persist(updated, next);
            return updated;
          });
          return next;
        });
        setStreaming(null);
        const u = finalRes?.usage || {};
        setStatus(`${active.name} 完成 · ${u.prompt_tokens || 0}/${u.completion_tokens || 0} tokens${isSub ? "（子任务）" : ""}`);
        setBusy(false);
        setActivity(null);
        turnTokensRef.current = 0;
        if (aborter.current === controller) aborter.current = null;
        refreshProfile();
        return;
      }
    },
    [messages, conversation, cwd, persist, agent, refreshProfile, streaming, clearScreen,
     runSubAgentLoop, startSubAgent, updateSub, todoWrite, todoUpdate, compressConversation,
     pushToolBlock, updateToolBlock]
  );

  /* ============ 命令 ============ */
  const runCommand = useCallback(async (cmd) => {
    const [name, ...rest] = cmd.trim().split(/\s+/);
    const arg = rest.join(" ");
    switch (name) {
      case "/help":
        pushToolBlock({ tool: "/help", output: COMMANDS.map((c) => `${c.cmd}  ${c.desc}`).join("\n"), status: "ok" });
        break;
      case "/connect":
        setConnectProvider(null);
        setMode("connect");
        break;
      case "/provider": {
        const providers = loadConfig().providers;
        if (!arg) {
          pushToolBlock({ tool: "/provider", output:
            providers.map((p) =>
              `- ${p.id}  ${p.name}${p.id === loadConfig().activeProvider ? "（当前）" : ""} · ${p.baseUrl}`
            ).join("\n") + "\n\n切换: /provider <id> · 接入新的: /connect",
            status: "ok" });
        } else {
          const target = providers.find((p) => p.id === arg);
          if (target) {
            const next = setActiveProvider(arg);
            if (next) {
              setCfg(next);
              setStatus(`已切换提供商: ${target.name}（${next.model}）`);
              if (!getProviderApiKey(target.id) && target.requiresAuth !== false) setMode("login");
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
          const key = getProviderApiKey(p.id);
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
      case "/effort": {
        const cur = loadConfig().effort || "high";
        const idx = EFFORT_LEVELS.indexOf(cur);
        setEffortIdx(cur === SUPERCODE ? EFFORT_LEVELS.length : idx >= 0 ? idx : 2);
        setMode("effort");
        setStatus("Reasoning effort · ←→ 调节 · Enter 确认 · Esc 取消");
        break;
      }
      case "/context": {
        const model = loadConfig().model;
        const window = getContextWindow(model);
        const used = estimateMessagesTokens(conversation);
        const thr = compactThreshold(model);
        const pct = Math.round((used / window) * 100);
        pushToolBlock({ tool: "/context", status: "ok", output: [
          `模型: ${model}`,
          `窗口: ${Math.round(window / 1000)}k tok`,
          `已用: ≈${Math.round(used / 1000)}k tok（${pct}%）`,
          `自动压缩阈值: ≈${Math.round(thr / 1000)}k tok（${Math.round(COMPACT_RATIO * 100)}%）`,
          "",
          "模型窗口表: deepseek-v4-pro / flash = 1M；glm-4-flash 未知按 128k 兜底。",
          "历史超限时自动压缩，也可 /compact 手动触发。",
        ].join("\n") });
        break;
      }
      case "/compact": {
        if (!conversation.length) { toast("没有对话内容可压缩"); break; }
        setStatus("压缩中…");
        setMessages((m) => [...m, { role: "assistant", agentName: "compact", content: "------ 压缩 --------", time: Date.now() }]);
        const res = await compressConversation(conversation, true);
        pushToolBlock({ tool: "/compact", status: res.error ? "error" : "ok", error: res.error || null, output:
          res.compacted
            ? `已压缩: ${Math.round(res.est / 1000)}k → ${Math.round(estimateMessagesTokens(res.newConv) / 1000)}k tok（窗口 ${Math.round(res.window / 1000)}k）`
            : `当前上下文 ≈${Math.round(res.est / 1000)}k tok，未达阈值（${Math.round(res.window / 1000)}k 窗口的 ${Math.round(COMPACT_RATIO * 100)}%）。` });
        break;
      }
      case "/todos": {
        if (!todos.length) {
          pushToolBlock({ tool: "/todos", status: "ok", output: "任务清单为空。让 agent 用 todo_write 工具建立清单后，面板会实时显示进度（也可 Ctrl+O 开关）。" });
        } else {
          const icon = (s) => (s === "completed" ? "✓" : s === "in_progress" ? "◈" : "○");
          pushToolBlock({
            tool: "/todos",
            status: "ok",
            output: todos.map((t) => `${icon(t.status)} ${t.subject}（${t.status}）`).join("\n"),
          });
        }
        break;
      }
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
      case "/new": {
        /* 保存当前会话并新建(会话可 /sessions 恢复) */
        const name = arg.trim() || `会话 ${sessionsRef.current.length + 1}`;
        const sid = `s${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
        const curMsgs = messagesRef.current;
        const curConv = conversationRef.current;
        const cur = sessionsRef.current;
        const updated = activeSessionIdRef.current
          ? cur.map((s) => s.id === activeSessionIdRef.current
            ? {
              ...s,
              history: curMsgs.filter((m) => m.role === "user" || m.role === "assistant")
                .map((m) => ({ role: m.role, content: m.content, time: m.time, reasoning: m.reasoning }))
                .slice(-200),
              conversation: curConv.slice(-200), agentId, cwd, updatedAt: Date.now(),
            }
            : s)
          : cur;
        const all = [...updated, { id: sid, name, history: [], conversation: [], agentId, cwd, updatedAt: Date.now() }];
        setSessions(all);
        sessionsRef.current = all;
        setActiveSessionId(sid);
        activeSessionIdRef.current = sid;
        setShowSessionList(false);
        clearScreen();
        setMessages([{ role: "assistant", content: `新会话「${name}」已开始。`, time: Date.now() }]);
        setShowBanner(true);
        setConversation([]);
        conversationRef.current = [];
        setHistoryUsed(0);
        setSubSessions({}); setSubView(false);
        setScrollOffset(0);
        persistSessionList(all, sid);
        setStatus(`新会话: ${name}，共 ${all.length} 个会话（/sessions 切换）`);
        break;
      }
      case "/clear":
        clearScreen();
        setMessages([{ role: "assistant", content: "当前会话历史已清空。", time: Date.now() }]);
        setShowBanner(true);
        setConversation([]);
        conversationRef.current = [];
        setHistoryUsed(0);
        persist([{ role: "assistant", content: "当前会话历史已清空。", time: Date.now() }], []);
        break;
      case "/sessions": {
        const list = sessionsRef.current;
        if (!list.length) {
          pushToolBlock({ tool: "/sessions", status: "ok", output: "暂无会话。/new 创建;每次对话自动落盘到当前会话。" });
          break;
        }
        setSessionIndex(Math.max(0, list.findIndex((s) => s.id === activeSessionIdRef.current)));
        setShowSessionList(true);
        setShowCommands(false);
        setStatus("↑↓ 选择会话 · Enter 切换 · Esc 取消");
        break;
      }
      case "/delete": {
        const list = sessionsRef.current;
        if (!list.length) { toast("没有可删除的会话"); break; }
        let target = null;
        if (/^\d+$/.test(arg)) target = list[Math.min(Number(arg) - 1, list.length - 1)];
        else target = list.find((s) => s.id === arg || s.name === arg);
        if (!target) { toast(`没有会话: ${arg}（编号或名称）`); break; }
        const remaining = list.filter((s) => s.id !== target.id) || [];
        setSessions(remaining);
        sessionsRef.current = remaining;
        if (storageRef.current === "db") dbDeleteSession(target.id);
        if (!remaining.length) {
          if (target.id === activeSessionIdRef.current) {
            setMessages([{ role: "assistant", content: "会话已清空。输入 /new 开始新会话。", time: Date.now() }]);
            setConversation([]); conversationRef.current = []; setHistoryUsed(0);
            activeSessionIdRef.current = null; setActiveSessionId(null);
            persistSessionList([], null);
          }
        } else if (target.id === activeSessionIdRef.current) {
          switchSession(remaining[0].id, false);
        } else {
          persistSessionList(remaining, activeSessionIdRef.current);
        }
        setStatus(`已删除会话: ${target.name}`);
        break;
      }
      case "/storage":
      case "/migrate": {
        /* 存储方案互转:推荐数据库;config.json 仅兼容场景使用 */
        if (arg) {
          const target = arg === "db" ? "db" : arg === "config" ? "config" : null;
          if (!target) { toast("/storage db|config（推荐 db，config.json 会过大）"); break; }
          changeStorage(target);
          break;
        }
        const dbCount = dbSessionCount();
        const cfgCount = sessionsRef.current.length;
        pushToolBlock({
          tool: "/storage",
          status: "ok",
          output: [
            `当前存储: ${storageRef.current ? "SQLite 数据库（推荐）" : "config.json（兼容模式）"}`,
            `SQLite 会话: ${dbCount} · config.json 会话: ${cfgCount}`,
            "",
            `互转: /storage db      将 config.json 会话迁入数据库（config.json 自动瘦身）`,
            `      /storage config  将数据库会话写回 config.json（旧数据库自动删除）`,
            "优先推荐数据库:单文件、读写快、config.json 只留配置；config.json 仅在无 SQLite 环境等特殊场景使用。",
          ].join("\n"),
        });
        break;
      }
      case "/diff": {
        setStatus("生成代码改动…");
        try {
          const q = JSON.stringify(cwd);
          const stat = execSync(`git -C ${q} diff HEAD --stat --color=never 2>/dev/null`, { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).toString().trim();
          const body = execSync(`git -C ${q} diff HEAD --color=never 2>/dev/null`, { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).toString();
          const staged = execSync(`git -C ${q} diff --cached --color=never 2>/dev/null`, { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).toString();
          let untracked = [];
          try { untracked = execSync(`git -C ${q} ls-files --others --exclude-standard`, { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).toString().trim().split("\n").filter(Boolean); } catch {}
          if (!stat && !staged.trim() && !untracked.length) {
            pushToolBlock({ tool: "/diff", status: "ok", output: "工作区无改动（git HEAD 一致）。" });
            break;
          }
          const parts = [];
          if (stat) parts.push(stat);
          if (staged.trim()) parts.push("── 已暂存 ──\n" + staged.trim());
          if (body.trim()) parts.push(body.trim());
          if (untracked.length) parts.push(`── 未跟踪 ${untracked.length} 个文件 ──\n${untracked.map((f) => `- ${f}`).join("\n")}`);
          setMessages((m) => [...m, {
            role: "diff", tool: "/diff", status: "ok",
            content: parts.join("\n\n"), time: Date.now(),
          }]);
          setStatus(`代码改动审阅: ${stat ? stat.split("\n").pop().trim() : "无暂存改动"}（+绿 / -红）`);
        } catch (e) {
          pushToolBlock({ tool: "/diff", status: "error", error: `${cwd} 不是 git 仓库`, output: "/diff 需要 git 仓库。可用 git init 初始化后重试。" });
        }
        break;
      }
      case "/skills": {
        if (arg) {
          const res = await executeTool("use_skill", { skill: arg }, cwd, {});
          if (res && res.error) {
            pushToolBlock({ tool: `/skills ${arg}`, status: "error", error: res.error, output: "可用: /skills 或 /skills <名称>" });
          } else {
            pushToolBlock({ tool: `/skills ${arg}`, status: "ok", output: `【${res.skill}】${res.description || ""}\n来源: ${res.source}\n\n${res.instructions}` });
          }
        } else {
          const skills = listSkills(cwd);
          pushToolBlock({
            tool: "/skills", status: "ok",
            output: skills.length
              ? skills.map((s) => `- ${s.name}${s.description ? `: ${s.description}` : ""}（${s.dir}）`).join("\n") + "\n\n加载指令: /skills <名称>"
              : "未在 .ux-agent/skills/ 下找到技能。创建目录后自动识别:\n  项目: <项目>/.ux-agent/skills/<name>/SKILL.md\n  全局: ~/.config/ux-agent/skills/<name>/SKILL.md",
          });
        }
        break;
      }
      case "/restore":
        setMode("restore");
        break;
      case "/exit":
        exit();
        break;
      default:
        setStatus(`未知命令: ${name}（/help）`);
    }
  }, [cwd, exit, refreshProfile, clearScreen, toast, compressConversation, conversation, todos, pushToolBlock, persist, switchSession, persistSessionList, changeStorage]);

  /* ============ 输入 ============ */
   const onSubmit = (value) => {
     if (busy) { toast("请等待当前任务完成"); return; }
     const text = value.trim();
     if (!text) return;
     setShowCommands(false);
     if (inputHistoryRef.current[0] !== text) {
       inputHistoryRef.current = [text, ...inputHistoryRef.current].slice(0, 50);
     }
     setHistoryIndex(-1);
     if (text.startsWith("/")) { runCommand(text); setInput(""); return; }
    setShowBanner(false);
    /* 无活动会话时自动创建（默认 SQLite 落盘，会话可 /sessions 恢复） */
    if (!activeSessionIdRef.current) {
      const name = `会话 ${sessionsRef.current.length + 1}`;
      const sid = `s${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
      const all = [...sessionsRef.current, { id: sid, name, history: [], conversation: [], agentId, cwd, updatedAt: Date.now() }];
      sessionsRef.current = all;
      setSessions(all);
      activeSessionIdRef.current = sid;
      setActiveSessionId(sid);
      persistSessionList(all, sid);
      setStatus(`已自动创建会话: ${name}`);
    }
    const p = getActiveProvider();
    if (!getProviderApiKey(p.id) && p.requiresAuth !== false) { setMode("login"); setStatus(`请为 ${p.name} 输入 API Key`); return; }
    const sub = subAgents().find((a) => text.startsWith(`@${a.name}`));
    if (sub) {
      const rest = text.replace(new RegExp(`^@${sub.name}\\s*`), "");
      setMessages((m) => [...m, { role: "user", content: text, time: Date.now() }]);
      setInput(""); setBusy(true);
      clearScreen();
      if (!activeSubId && Object.keys(subSessions).length) {
        /* 有子会话时自动进入最近会话查看 */
        setSubView(true);
        setStatus(`子agent ${sub.name} 已启动 · ⇄ 切换 · Esc 返回`);
      }
      /* 走真正的子会话通道:可实时查看、可继续对话、可 ↔ 并行 */
      setActivity({ kind: "delegate", target: sub.name });
      busySinceRef.current = Date.now();
      turnTokensRef.current = 0;
      const sid = startSubAgent(sub.name, rest || "帮我完成这个任务");
      cancelRequestedRef.current = false;
      const controller = new AbortController();
      aborter.current = controller;
      runSubAgentLoop(sid, sub.name, rest || "帮我完成这个任务", controller.signal).then((res) => {
        setBusy(false);
        if (res?.result) {
          setMessages((m) => [...m, {
            role: "assistant", agentName: sub.name,
            content: `**${sub.name} 子代理完成**\n\n${String(res.result).slice(0, 1200)}`,
            time: Date.now(),
          }]);
        } else if (res?.error) {
          setMessages((m) => [...m, {
            role: "assistant", agentName: sub.name,
            content: `**${sub.name} 子代理出错**\n\n${res.error}`,
            time: Date.now(),
          }]);
        }
        setStatus(`子agent ${sub.name} ${res?.cancelled ? "已停止" : res?.error ? "出错" : "完成"}（→ 查看结果）`);
        setActivity(null); turnTokensRef.current = 0;
      }).catch(() => {
        setBusy(false); setStatus(`子agent ${sub.name} 异常`); setActivity(null);
      }).finally(() => {
        if (aborter.current === controller) aborter.current = null;
      });
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
  const subIdList = () => Object.keys(subSessionsRef.current);
  const switchSub = (dir) => {
    const ids = subIdList();
    if (!ids.length) return;
    const cur = subSessionsRef.current[activeSubId] ? activeSubId : ids[0];
    const idx = ids.indexOf(cur);
    const next = ids[(idx + dir + ids.length) % ids.length];
    subScrollRef.current[activeSubId] = subScrollOffset;
    setActiveSubId(next);
    setSubScrollOffset(subScrollRef.current[next] || 0);
    setSubView(true);
    clearScreen();
    const s = subSessionsRef.current[next];
    setStatus(`子agent ${s?.agentId || next} 会话 ${idx + 1}/${ids.length} · ⇄ 切换 · Esc 返回`);
  };
  useInput((_input, key) => {
    if (key.escape) {
      if (aborter.current && !aborter.current.signal.aborted) {
        cancelRequestedRef.current = true;
        aborter.current.abort();
        setStatus("正在停止…");
        return;
      }
      if (subView) {
        subScrollRef.current[activeSubId] = subScrollOffset;
        setSubView(false);
        clearScreen();
        return;
      }
      if (mode !== "chat") {
        /* 迁移弹窗 Esc = 暂不迁移(等价 n) */
        if (mode === "migrate") { onMigrateSubmit("n"); return; }
        /* 恢复弹窗 Esc = 取消 */
        if (mode === "restore") { onRestoreSubmit("n"); return; }
        setMode("chat"); setConnectProvider(null); return;
      }
    }
    /* effort 滑块:←→ 调节(5 档 + 分隔线右侧 supercode Mode),Enter 确认,Esc 取消 */
    if (mode === "effort") {
      if (key.leftArrow) { setEffortIdx((i) => Math.max(i - 1, 0)); return; }
      if (key.rightArrow) {
        setEffortIdx((i) => {
          const n = Math.min(i + 1, EFFORT_LEVELS.length);
          if (n === 4) maxSinceRef.current = Date.now();
          else if (n === EFFORT_LEVELS.length) superSinceRef.current = Date.now();
          return n;
        });
        return;
      }
      if (key.return) {
        const lv = effortIdx >= EFFORT_LEVELS.length ? SUPERCODE : EFFORT_LEVELS[effortIdx];
        saveConfig({ effort: lv });
        setEffort(lv);
        setMode("chat");
        setStatus(lv === SUPERCODE
          ? "supercode 模式: max 推理 + 多子 agent 并发编排"
          : `reasoning effort → ${lv}`);
        return;
      }
      if (key.escape) { setMode("chat"); setStatus("就绪"); return; }
      return;
    }
    /* 子聊天区:独立滚动上下文 + ⇄ 切换子会话 */
    if (subView) {
      if (key.upArrow) setSubScrollOffset((o) => Math.min(o + 3, 10000));
      if (key.downArrow) setSubScrollOffset((o) => Math.max(o - 3, 0));
      if (key.pageUp) setSubScrollOffset((o) => o + MSG_HEIGHT);
      if (key.pageDown) setSubScrollOffset((o) => Math.max(o - MSG_HEIGHT, 0));
      if (key.leftArrow) switchSub(-1);
      if (key.rightArrow) switchSub(1);
      return;
    }
    if (mode !== "chat") return;
    /* 会话拾取器(/sessions):↑↓ 选择,Enter 切换,Esc 取消 */
    if (showSessionList) {
      const list = sessionsRef.current;
      if (key.escape) { setShowSessionList(false); setStatus("就绪"); return; }
      if (key.upArrow) { setSessionIndex((i) => Math.max(i - 1, 0)); return; }
      if (key.downArrow) { setSessionIndex((i) => Math.min(i + 1, list.length - 1)); return; }
      if (key.return && list.length) {
        switchSession(list[Math.min(sessionIndex, list.length - 1)].id);
        return;
      }
    }
    if (key.tab) {
      const primaries = primaryAgents();
      const idx = primaries.findIndex((a) => a.id === agentId);
      const next = primaries[(idx + 1) % primaries.length];
      setAgentId(next.id);
      clearScreen();
      setStatus(`agent → ${next.name}`);
    }
    if (key.rightArrow && subIdList().length) {
      subScrollRef.current[activeSubId] = subScrollOffset;
      setActiveSubId((prev) => {
        const ids = subIdList();
        const next = ids.includes(prev) ? prev : ids[0];
        const s = subSessionsRef.current[next];
        setSubScrollOffset(subScrollRef.current[next] || 0);
        setSubView(true);
        clearScreen();
        setStatus(`子agent ${s?.agentId || next} 会话 · ⇄ 切换 · Esc 返回`);
        return next;
      });
    }
    if (key.ctrl && (_input === "t" || _input === "T")) {
      setExpandedThinking((e) => !e);
    }
    if (key.ctrl && (_input === "o" || _input === "O")) {
      setShowTodos((v) => {
        setStatus(`任务清单面板: ${!v ? "开" : "关"}`);
        return !v;
      });
    }
     if (key.ctrl && (_input === "e" || _input === "E")) {
      setShowToolDetails((v) => {
        setStatus(v ? "工具详情已折叠" : "工具详情已展开（Ctrl+E 切换）");
        return !v;
      });
    }
    /* 命令面板:↑/↓ 切换高亮,Enter 选中当前高亮命令 */
    if (showPalette) {
      const matches = COMMANDS.filter((c) => c.cmd.includes(input.slice(1).toLowerCase()));
      if (key.upArrow) { setPaletteIndex((i) => Math.max(i - 1, 0)); return; }
      if (key.downArrow) { setPaletteIndex((i) => Math.min(i + 1, matches.length - 1)); return; }
      if (key.return && matches.length) {
        const pick = matches[Math.min(paletteIndex, matches.length - 1)];
        setInput(pick.cmd);
        setShowCommands(false);
        setPaletteIndex(0);
        onSubmit(pick.cmd);
        return;
      }
    }
    /* 输入历史:输入框有内容时↑/↓回溯;为空时↑/↓滚聊天区 */
    if (key.upArrow && input) {
      if (inputHistoryRef.current.length) {
        const idx = Math.min(historyIndex + 1, inputHistoryRef.current.length - 1);
        setHistoryIndex(idx);
        setInput(inputHistoryRef.current[idx]);
      }
      return;
    }
    if (key.downArrow && input) {
      if (historyIndex > 0) {
        const idx = historyIndex - 1;
        setHistoryIndex(idx);
        setInput(inputHistoryRef.current[idx]);
      } else {
        setHistoryIndex(-1);
        setInput("");
      }
      return;
    }
    if (key.upArrow) setScrollOffset((o) => Math.min(o + 3, 10000));
    if (key.downArrow) setScrollOffset((o) => Math.max(o - 3, 0));
    if (key.pageUp) setScrollOffset((o) => o + MSG_HEIGHT);
    if (key.pageDown) setScrollOffset((o) => Math.max(o - MSG_HEIGHT, 0));
  });

  /* 滚动窗口：基于行的精确切片 */
  const msgColor = (m) => (m.role === "user" ? "green" : m.role === "tool" ? "yellow" : m.role === "tool_result" ? "gray" : "magenta");
  const msgPrefix = (m) => (m.role === "user" ? "❯" : m.role === "tool" ? "⚙" : m.role === "tool_result" ? "✓" : "◆");

  /* 单条消息 → 行数组（每行固定 1 高） */
  const messageToRows = (m, isStreaming) => {
    const rows = [];
    /* 代码改动审阅(/diff):绿+/红- 高亮,始终展开 */
    if (m.role === "diff") {
      rows.push({ kind: "tool", m, text: `◇ ${m.tool || "代码改动"} · ${m.status || "ok"}`, color: "blue", bold: true });
      for (const l of diffLines(m.content, rowWidth)) {
        rows.push({ kind: "diff", m, text: `  ${l.text}`, color: l.color, bold: l.bold, dim: l.dim });
      }
      return rows;
    }
    /* 工具块(opencode 风格):折叠 = 1 行状态头,展开 = 状态头 + 参数 + 输出。
     * 命令输出(/help 等)始终展开。随消息流滚动,不悬浮、不挤占。 */
    if (m.role === "tool") {
      const isCmd = m.tool && m.tool.startsWith("/");
      const open = isCmd || showToolDetails;
      const icon = m.status === "running" ? "◌" : m.status === "error" ? "✗" : "✓";
      const color = m.status === "running" ? "cyan" : m.status === "error" ? "red" : "yellow";
      const statusTxt = m.status === "running" ? "运行中" : m.status === "error" ? "出错" : "完成";
      const agentTag = m.agentName ? ` [${m.agentName}]` : "";
      const durTxt = m.dur ? ` · ${fmtDuration(m.dur)}` : "";
      let head = `${icon} ${m.tool}${agentTag} · ${statusTxt}${durTxt}`;
      const outText = toolOutputText(m);
      const outCount = outText ? outText.split("\n").length : 0;
      if (!open && outCount) head += ` · 输出 ${outCount} 行`;
      if (!isCmd) head += open ? " · 折叠详情" : " · 展开详情";
      rows.push({ kind: "tool", m, text: head, color, bold: true });
      if (open) {
        const ck = `_t${rowWidth}`;
        if (m[ck]) { rows.push(...m[ck]); return rows; }
        const toolRows = [];
        const brief = toolArgsBrief(m.args);
        if (brief) toolRows.push({ kind: "tool", m, text: `  $ ${brief}`, color: "gray", dim: true });
        if (outText) {
          for (const l of wrapPlain(outText, Math.max(rowWidth - 2, 8))) {
            toolRows.push({ kind: "tool", m, text: `  ${l}`, color: "gray", dim: true });
          }
        }
        m[ck] = toolRows;
        rows.push(...toolRows);
      }
      return rows;
    }
    const color = msgColor(m);
    const prefix = msgPrefix(m);
    const label = m.role === "user" ? "You" : m.role === "assistant" ? (m.agentName || "Agent") : "";
    const header = label ? `${prefix} ${label} · ${fmtTime(m.time)}` : `${prefix} ${fmtTime(m.time)}`;
    rows.push({ kind: "header", m, text: header, color, bold: true });

    /* 思考块：行模型渲染（opencode 风格折叠），保持固定行高不破坏布局 */
    if (m.reasoning) {
      const sec = Math.max(1, Math.round(String(m.reasoning).trim().split("\n").length / 20));
      if (expandedThinking) {
        rows.push({ kind: "thinking", m, text: `┌ ◈ Thought for ${sec}s ▾`, color: "cyan", bold: true });
        for (const l of wrapPlain(String(m.reasoning), rowWidth - 2)) {
          rows.push({ kind: "thinking", m, text: `│ ${l}`, color: "gray", dim: true });
        }
        rows.push({ kind: "thinking", m, text: `└`, color: "gray", dim: true });
      } else {
        rows.push({ kind: "thinking", m, text: `┌ ◈ Thought for ${sec}s ▸`, color: "gray", dim: true });
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

  /* opencode 风格启动横幅:ASCII art logo + 弱化标签信息,固定行数不破坏行模型。
   * 颜色随时间做 HSL 色相旋转(8s 一圈),启动页不单调;发消息后 showBanner=false,动画自动停止。 */
  const bannerAnim = useAnimationTime(80, showBanner);
  /* effort 滑块动画:滑块打开期间驱动雷霆指针(⚡ 呼吸闪烁) */
  const effortAnim = useAnimationTime(80, mode === "effort");
  const bannerRows = useMemo(() => {
    const logo = [
      "   __  _______   ___  __ __________ ",
      "  / / / /  _/ | / / |/ // ____/ __ \\",
      " / / / // //  |/ /|   // __/ / / / /",
      "/ /_/ // // /|  //   |/ /___/ /_/ / ",
      "\\____/___/_/ |_//_/|_/_____/_____/",
    ];
    /* 色相旋转:每 80ms 移 3.6°,约 8s 一圈;5 行均分 90° 范围,形成流动渐变 */
    const baseHue = (bannerAnim * 3.6) % 360;
    const sessName = sessions.find((s) => s.id === activeSessionIdRef.current)?.name || (sessions.length ? sessions[0].name : "新会话");
    const st = `${storageRef.current ? "SQLite" : "config.json"}`;
    const rows = logo.map((l, i) => ({
      kind: "banner", m: null, text: l,
      color: hslToHex(baseHue + i * 18, 0.85, 0.62), bold: true,
    }));
    rows.push({ kind: "banner", m: null, text: " ", color: "gray", dim: true });
    rows.push({
      kind: "banner", m: null, text: `  Session   ${sessName}`,
      color: "gray", dim: true, bold: false,
    });
    rows.push({
      kind: "banner", m: null, text: `  Storage   ${st} · /help 命令 · /new 会话 · /sessions 切换`,
      color: "gray", dim: true,
    });
    return rows;
  }, [rowWidth, sessions, bannerAnim]);

  /* 全部可见行（含流式中的消息；思考/子agent活动动画改由 ActivityPanel 呈现）。
   * useMemo:避免每次渲染重复 wrap 所有工具输出(展开时 O(n)→O(n²))。 */
  const allRows = useMemo(() => {
    let rows = [];
    if (showBanner) rows = rows.concat(bannerRows);
    for (const m of messages) rows = rows.concat(messageToRows(m, false));
    if (streaming) rows = rows.concat(messageToRows(streaming, true));
    if (busy && !streaming && !rows.length) rows.push({ kind: "md", m: null, text: "…", color: "gray" });
    return rows;
  }, [messages, streaming, showToolDetails, rowWidth, busy, expandedThinking, showBanner, bannerRows]);

  const maxScroll = Math.max(0, allRows.length - MSG_HEIGHT);
  const safeOffset = Math.min(scrollOffset, maxScroll);
  const start = Math.max(0, allRows.length - MSG_HEIGHT - safeOffset);
  const visibleRows = allRows.slice(start, start + MSG_HEIGHT);

  const agentColor = agent.color;
  /* 上下文占用百分比（状态栏展示） */
  const ctxWindow = getContextWindow(loadConfig().model);
  const ctxPct = Math.min(999, Math.round((historyUsed / ctxWindow) * 100));
  const ctxColor = ctxPct >= 60 ? "red" : ctxPct >= 40 ? "yellow" : "cyan";

  /* ============ 子聊天区渲染（subView=true,可 ⇄ 切换多个子会话） ============ */
  if (subView) {
    const cur = subSessions[activeSubId];
    if (cur) {
      const subAgent = getAgent(cur.agentId);
      const subColor = subAgent.color || "magenta";
      const subMsgs = subMsgsRef.current[activeSubId] || [];
      /* 从 ref 构建子会话 UI 消息:user 原文 + assistant 正文 + tool 一行 */
      const subRows = [];
      let pendingText = null;
      let pendingReasoning = null;
      let toolName = "";
      const pushAssistant = () => {
        if (pendingText || pendingReasoning) {
          subRows.push({ role: "assistant", content: pendingText || "", reasoning: pendingReasoning || null });
          pendingText = null;
          pendingReasoning = null;
        }
      };
      for (const m of subMsgs) {
        if (m.role === "user") {
          pushAssistant();
          subRows.push({ role: "user", content: m.content });
        } else if (m.role === "assistant") {
          if (m.content) pendingText = (pendingText || "") + m.content;
          if (m.reasoning_content) pendingReasoning = (pendingReasoning || "") + m.reasoning_content;
          if (m.tool_calls && m.tool_calls.length) {
            pushAssistant();
            const names = m.tool_calls.map((tc) => tc.function?.name || "?").join(", ");
            toolName = names;
          }
        } else if (m.role === "tool") {
          pushAssistant();
          subRows.push({ role: "tool", content: `⚙ 调用 ${toolName || "工具"}` });
        }
      }
      pushAssistant();
      if (cur.streaming) {
        subRows.push({ role: "assistant-stream", content: cur.streaming.content || "", reasoning: cur.streaming.reasoning || null });
      }

      /* 行模型窗口:每个 subRow 转成 markdown 行,再按 SUB_MSG_HEIGHT 切片(同主视图) */
      const subAllRows = [];
      const subRowToLines = (rm) => {
        if (rm.role === "user") {
          return [{ text: `❯ You`, color: "green", bold: true }];
        }
        if (rm.role === "tool") {
          return [{ text: rm.content, color: "gray", dim: true }];
        }
        const isStream = rm.role === "assistant-stream";
        const headerColor = isStream ? "gray" : subColor;
        const headerText = `◆ ${cur.agentId}${isStream ? "…" : ""} · ${fmtTime(Date.now())}`;
        const rows = [{ text: headerText, color: headerColor, bold: true }];
        if (rm.reasoning) {
          rows.push({ text: `  ↯ ${String(rm.reasoning).split("\n").length} 行推理`, color: "gray", dim: true });
        }
        if (!rm.content) return rows;
        const lines = markdownLines(rm.content, rowWidth, { color: isStream ? "gray" : subColor, dim: isStream });
        for (const l of lines) rows.push(l);
        return rows;
      };
      for (const rm of subRows) subAllRows.push(...subRowToLines(rm));
      const subMax = Math.max(0, subAllRows.length - SUB_MSG_HEIGHT);
      const subSafe = Math.min(subScrollOffset, subMax);
      const subStart = Math.max(0, subAllRows.length - SUB_MSG_HEIGHT - subSafe);
      const subVisible = subAllRows.slice(subStart, subStart + SUB_MSG_HEIGHT);
      const subIds = Object.keys(subSessions);
      const subIdx = subIds.indexOf(activeSubId);

      return (
        <Box flexDirection="column" height={HEIGHT} borderStyle="round" borderColor={subColor}>
          <Box flexShrink={0} paddingX={1}>
            <Text bold color={subColor} wrap="truncate" width={Math.max(WIDTH - 2, 10)}>
              ◆ 子 {cur.agentId}{subIds.length > 1 ? ` (${subIdx + 1}/${subIds.length})` : ""} · {String(cur.task).slice(0, 40)}{cur.busy ? " ▸ 运行中" : cur.error ? " ✗ 出错" : " ✓ 完成"}
            </Text>
          </Box>
          <Box flexGrow={1} flexShrink={1} height={SUB_MSG_HEIGHT} flexDirection="column" paddingX={1} overflow="hidden">
            {subVisible.map((r, i) => (
              <MessageBoundary key={i}>
                {r.spans ? (
                  <LineRow line={r} width={rowWidth} />
                ) : (
                  <Text color={r.color || subColor} bold={r.bold} dim={r.dim} wrap="truncate" width={rowWidth}>
                    {r.text}
                  </Text>
                )}
              </MessageBoundary>
            ))}
            {cur.streaming && !cur.streaming.content && <Text dimColor>  ↯ 推理中…</Text>}
          </Box>
          <Box borderStyle="round" borderColor={cur.busy ? "gray" : "green"} paddingX={1} flexShrink={0}>
            <TextInput
              value={subInput}
              onChange={setSubInput}
              onSubmit={(v) => continueSubConversation(activeSubId, v)}
              placeholder={cur.busy ? "运行中…" : "继续对话…"}
              disabled={cur.busy}
            />
          </Box>
          <Box flexShrink={0}>
            <Text dimColor> ⇄ 子agent · ↑↓ 滚动 · Esc 返回</Text>
          </Box>
        </Box>
      );
    }
    /* 子会话已不存在:由 useEffect 重置 subView,这里直接渲染主视图 */
  }

  return (
    <Box flexDirection="column" height={HEIGHT} borderStyle="round" borderColor="gray">
      {/* 状态栏 */}
      <Box flexDirection="row" flexShrink={0} paddingX={1}>
        <Text bold color="cyan" wrap="truncate" width={Math.max(WIDTH - 2, 10)}>
          ◆ {agent.name} · {provider.name} · {loadConfig().model}
          {profile ? ` · ${profile.username}` : getProviderApiKey(provider.id) ? " · 已连接" : ""}
          {historyUsed > 0 ? <Text color={ctxColor}> · ctx {ctxPct}%</Text> : null}
          <Text dimColor> · {status}</Text>
        </Text>
      </Box>

      {/* 消息区：行模型切片，每行固定 1 高，不会炸 */}
      <Box flexGrow={1} flexShrink={1} height={MSG_HEIGHT} flexDirection="column" paddingX={1} overflow="hidden">
        {visibleRows.map((r, i) => (
          <MessageBoundary key={r.m?.time + "-" + i}>
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

      {/* 连接弹窗（固定行数，保证布局预算精确） */}
      {mode === "connect" && (
        <ConnectModal provider={connectProvider} onSubmit={onConnectSubmit} onCancel={() => setMode("chat")} />
      )}
      {mode === "login" && (
        <Box borderStyle="round" borderColor="yellow" paddingX={2} height={6}>
          <Box flexDirection="column">
            <Text bold color="yellow">为 {provider.name} 输入 API Key:</Text>
            <TextInput value={loginInput} onChange={setLoginInput} onSubmit={onLoginSubmit} />
            {loginErr && <Text color="red">{loginErr}</Text>}
            <Text dimColor>Enter 保存 · Esc 取消</Text>
          </Box>
        </Box>
      )}

      {/* 迁移确认弹窗(启动时检测到旧 config.json 会话数据) */}
      {mode === "migrate" && (
        <Box borderStyle="round" borderColor="cyan" paddingX={1} height={6}>
          <Box flexDirection="column">
            <Text bold color="cyan" wrap="wrap">检测到旧版 config.json 会话数据（{pendingLegacyRef.current?.sessions?.length || 1} 个），是否迁移到 SQLite？</Text>
            <TextInput value={migrateInput} onChange={setMigrateInput} onSubmit={onMigrateSubmit} placeholder="y 迁入数据库（推荐） · n 继续用 config.json" />
            <Text dimColor wrap="wrap">迁移后 config.json 只保留配置（不再膨胀）；Esc = 暂不迁移</Text>
          </Box>
        </Box>
      )}

      {/* 恢复出厂确认弹窗 */}
      {mode === "restore" && (
        <Box borderStyle="round" borderColor="red" paddingX={1} height={6}>
          <Box flexDirection="column">
            <Text bold color="red" wrap="wrap">⚠ 确认恢复出厂设置？这将删除所有会话数据（数据库 + config.json 会话记录），此操作不可撤销。</Text>
            <TextInput value={restoreInput} onChange={setRestoreInput} onSubmit={onRestoreSubmit} placeholder="y 确认删除所有数据 · Esc 取消" />
            <Text dimColor wrap="wrap">删除后需要重新创建会话；Esc = 取消</Text>
          </Box>
        </Box>
      )}

      {/* 命令面板 */}
      {showPalette && <CommandPalette input={input} />}

      {/* 会话拾取器 */}
      {showSessionList && (
        <SessionPicker sessions={sessions} activeSessionId={activeSessionId} index={sessionIndex} width={rowWidth} />
      )}

      {/* Claude Code 风格活动动画面板（思考/工具/子agent/待办） */}
      <Box flexShrink={0} height={activityLines} overflow="hidden">
        <ActivityPanel
          busy={busy}
          activity={activity}
          sinceRef={busySinceRef}
          tokensRef={turnTokensRef}
          subs={subList}
          todos={todos}
          showTodos={showTodos}
          width={rowWidth}
          color={agentColor}
          effort={effort}
        />
      </Box>

      {/* 输入区 */}
      <Box borderStyle="round" borderColor="gray" paddingX={1} flexShrink={0}>
        {mode === "chat" && (
          <TextInput
            value={input}
            onChange={(v) => { setInput(v); setShowCommands(v.startsWith("/")); setPaletteIndex(0); setHistoryIndex(-1); }}
            onSubmit={(v) => {
              /* 命令面板有匹配时,Enter 交给面板处理(面板已提交选中的命令),
               * 避免 TextInput 与面板各执行一次 → 命令跑两遍 */
              const paletteActive = mode === "chat" && showCommands && v.startsWith("/");
              const matches = paletteActive ? COMMANDS.filter((c) => c.cmd.includes(v.slice(1).toLowerCase())) : [];
              if (paletteActive && matches.length) return;
              onSubmit(v);
            }}
            placeholder={busy ? "…" : "/ 命令 · @agent · ↑↓ 历史"}
            disabled={busy}
          />
        )}
        {mode === "model" && (
          <Box flexDirection="column" height={5}>
            <Text bold color="cyan">可用模型（输入 id 后 Enter）:</Text>
            {modelOptions.slice(0, 4).map((m) => <Text key={m} color="white">  {m}</Text>)}
            {modelOptions.length > 4 && <Text dimColor>  …共 {modelOptions.length} 个模型</Text>}
            <TextInput value={modelPick} onChange={setModelPick} onSubmit={onModelSubmit} />
          </Box>
        )}
        {mode === "effort" && (() => {
          /* Claude Code /effort 滑块:两端 Faster/Smarter,刻度线上指针,级别名按刻度中心对齐。
           * 指针随档位递进:low ○ 灰 · medium ◐ 青 · high ◑ 金 · xhigh ◉ 橙 · max ◆ 蓝;
           * supercode 为分隔线 ┊ 右侧的独立 Mode(参照 ultracode):
           * 5 档刻度仍为 low…max,右侧轨道上紫色 ◆,标签紫色(未选中静态紫、选中紫呼吸),
           * 副注释行标注 "max + 多子 agent 并发"。
           * 波纹:max 蓝色 / supercode 紫色,从对应标签中心发出,平滑扩散渐隐,填满整个面板。 */
          const pad = 4;
          const barLen = Math.max(20, Math.min(40, rowWidth - 38));
          const posOf = (i) => Math.round((i * (barLen - 1)) / (EFFORT_LEVELS.length - 1));
          const marks = ["○", "◐", "◑", "◉", "◆"];
          const markCols = ["gray", "cyan", "#FFE066", "#FFB347", "#38BDF8"];
          const seg = barLen / (EFFORT_LEVELS.length - 1);
          /* supercode 区域坐标(分隔线右侧独立 Mode) */
          const divX = pad + barLen + 2;
          const superLabelX = divX + 2 + 3;
          const superTrack = 14;
          const superMarkIdx = 5;
          const superCx = superLabelX + Math.floor(SUPERCODE.length / 2);
          const superColor = effortIdx === 5
            ? hslToHex(268 + 8 * Math.sin(effortAnim * 0.12), 0.9, 0.62 + 0.2 * Math.sin(effortAnim * 0.35))
            : "#A78BFA";
          const superMark = "◆";
          const effName = effortIdx >= EFFORT_LEVELS.length ? SUPERCODE : EFFORT_LEVELS[effortIdx];
          /* 当前档位呼吸色:high 金 / xhigh 橙 / max 蓝 / supercode 紫 */
          const liveColor = (() => {
            if (effortIdx === 5) return superColor;
            if (effortIdx === 4) return hslToHex(205 + 10 * Math.sin(effortAnim * 0.12), 0.92, 0.52 + 0.2 * Math.sin(effortAnim * 0.16));
            if (effortIdx >= 2) return hslToHex(38, 0.95, 0.5 + 0.22 * Math.sin(effortAnim * 0.3));
            return markCols[effortIdx];
          })();
          const ruler = (() => {
            let s = "";
            for (let i = 0; i < barLen; i++) s += effortIdx <= 4 && i === posOf(effortIdx) ? marks[effortIdx] : "─";
            s += "  ┊  ";
            for (let i = 0; i < superTrack; i++) s += effortIdx === 5 && i === superMarkIdx ? superMark : "─";
            return s;
          })();
          /* 标签行:仅 active 动画,xhigh/max 未选中时静态灰,supercode 未选中静态紫 */
          const labelColor = (i, active) => {
            if (!active) return "gray";
            if (i === 4) return hslToHex(205 + 10 * Math.sin(effortAnim * 0.12), 0.92, 0.55 + 0.25 * Math.sin(effortAnim * 0.35));
            if (i === 3) return hslToHex(30, 0.95, 0.55 + 0.22 * Math.sin(effortAnim * 0.3));
            return markCols[i];
          };
          /* 波纹:max 蓝 / supercode 紫,从对应标签文字中心发出,多环连续扩散渐隐,填满整个面板 */
          let cx = Math.min(pad + posOf(4), rowWidth - 1);
          const cy = 3;
          const bgFor = (x, y) => {
            let pr = null, since = 0, ox = cx;
            if (effortIdx === 4) { pr = RIPPLE_PRESETS.max; since = maxSinceRef.current; }
            else if (effortIdx === 5) { pr = RIPPLE_PRESETS.supercode; since = superSinceRef.current; ox = superCx; }
            if (!pr) return null;
            const fadeIn = Math.min(1, (Date.now() - since) / 700);
            if (fadeIn <= 0.02) return null;
            return rippleBgAt(x, y, effortAnim, rowWidth, ox, cy, pr, fadeIn);
          };
          const rrow = (y, cells) => (
            <Text>
              {Array.from({ length: rowWidth }, (_, x) => {
                const c = cells[x] || { ch: " " };
                const col = typeof c.color === "function" ? c.color(x, y) : c.color;
                return (
                  <Text key={x} color={col} bold={c.bold} backgroundColor={bgFor(x, y)}>{c.ch}</Text>
                );
              })}
            </Text>
          );
          const cellsOf = (y, s, color, bold) => {
            const cells = {};
            s.split("").forEach((ch, x) => { cells[x] = { ch, color, bold }; });
            return cells;
          };
          /* 构造各行的逐字符内容 */
          const rowCells = [];
          /* 标题行:Effort — 当前档位名(supercode 紫色) */
          rowCells[0] = (() => {
            const cells = {};
            "Effort".split("").forEach((ch, x) => { cells[x] = { ch, color: "cyan", bold: true }; });
            ` — ${effName}`.split("").forEach((ch, x) => {
              cells[6 + x] = { ch, color: effName === SUPERCODE ? superColor : liveColor, bold: true };
            });
            return cells;
          })();
          const fastLine = " ".repeat(pad) + "Faster" + " ".repeat(Math.max(6, barLen - 13)) + "Smarter";
          rowCells[1] = cellsOf(1, fastLine, "gray", false);
          const rulerLine = " ".repeat(pad) + ruler;
          rowCells[2] = (() => {
            const cells = {};
            rulerLine.split("").forEach((ch, x) => {
              if (x < pad) { cells[x] = { ch, color: "gray", bold: false }; return; }
              if (x >= divX + 2 && x < divX + 2 + superTrack) {
                cells[x] = ch === "◆" ? { ch, color: superColor, bold: true } : { ch, color: "#7C5CFF", bold: false };
              } else if (ch === "┊") {
                cells[x] = { ch, color: "#7C5CFF", bold: false };
              } else {
                cells[x] = ch === "◆" ? { ch, color: liveColor, bold: true } : { ch, color: "gray", bold: false };
              }
            });
            return cells;
          })();
          const labelLine = [];
          let lineSoFar = 0;
          for (let i = 0; i < EFFORT_LEVELS.length; i++) {
            const lv = EFFORT_LEVELS[i];
            const c = Math.round(i * seg) - Math.floor(stringWidth(lv) / 2);
            const gap = Math.max(1, c - lineSoFar);
            labelLine.push(" ".repeat(gap));
            lineSoFar += gap;
            labelLine.push({ lv, active: i === effortIdx, i });
            lineSoFar += stringWidth(lv);
          }
          rowCells[3] = (() => {
            const cells = {};
            let x = pad;
            for (const part of labelLine) {
              if (typeof part === "string") {
                part.split("").forEach((ch, k) => { cells[x + k] = { ch, color: "gray", bold: false }; });
                x += part.length;
              } else {
                const { lv, active, i: li } = part;
                lv.split("").forEach((ch, k) => {
                  cells[x + k] = { ch, color: labelColor(li, active), bold: active };
                });
                if (li === 4) cx = Math.min(x + Math.floor(stringWidth(lv) / 2), rowWidth - 1); // "max" 文字中心
                x += stringWidth(lv);
              }
            }
            /* 分隔线 + supercode 标签(参照 ultracode 独立 Mode) */
            "  ┊  ".split("").forEach((ch, k) => { cells[x + k] = { ch, color: "#7C5CFF", bold: false }; });
            x += 5;
            SUPERCODE.split("").forEach((ch, k) => {
              cells[x + k] = { ch, color: superColor, bold: effortIdx === 5 };
            });
            return cells;
          })();
          /* 副注释行:不再显示文字(曾影响波纹观感),整行留空让波纹完整铺满;
           * supercode 说明由标题行 "Effort — supercode" 与 Enter 确认的 status 承担 */
          rowCells[4] = {};
          const hintLine = "←/→ adjust · Enter confirm · Esc cancel";
          rowCells[5] = cellsOf(5, hintLine, "gray", false);
          return (
            <Box flexDirection="column" height={6}>
              {[0, 1, 2, 3, 4, 5].map((y) => rrow(y, rowCells[y]))}
            </Box>
          );
        })()}
      </Box>

      <Box flexShrink={0} paddingX={1}>
        <Text dimColor>Tab agent · Ctrl+T/O/E · ⇄ sub · Esc 退出</Text>
      </Box>
    </Box>
  );
}
