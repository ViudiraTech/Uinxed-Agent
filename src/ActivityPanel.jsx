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

import React, { useRef } from "react";
import { Box, Text } from "ink";
import {
  SPINNER_FRAMES_SPIN,
  THINKING_VERBS,
  TOOL_VERBS,
  SUBAGENT_VERBS,
  TOOL_VERB_MAP,
  useAnimationTime,
  useRotating,
  fmtDuration,
  fmtTokens,
  useSmoothCounter,
  tickerChars,
  glimmer,
  sweepAt,
} from "./anim.js";
import { stringWidth } from "./mdlines.js";

/* 与 App.jsx 保持一致的动画行数预算(用于动态布局预留)。
 * 行数按实际渲染内容计算:主状态行 1 + 子 agent ≤4 + 待办头 1 + 待办 ≤5,
 * 避免预留过多空行导致消息区吃不满。 */
export const MAX_SUB_ROWS = 4;
export const MAX_TODO_ROWS = 5;

export function activityRowCount({ busy, subs, todos, showTodos }) {
  let n = 0;
  if (busy) n += 1;
  n += Math.min((subs || []).length, MAX_SUB_ROWS);
  if (showTodos && (todos || []).length > 0) n += 1 + Math.min(todos.length, MAX_TODO_ROWS);
  return n;
}

/* 两个 hex 颜色按 f∈[0,1] 插值(thinking glimmer) */
function hexInterp(a, b, f) {
  const pa = parseInt(a.slice(1), 16);
  const pb = parseInt(b.slice(1), 16);
  const mix = (x, y) => Math.round(x + (y - x) * f);
  const r = mix((pa >> 16) & 255, (pb >> 16) & 255);
  const g = mix((pa >> 8) & 255, (pb >> 8) & 255);
  const bl = mix(pa & 255, pb & 255);
  return `#${((r << 16) | (g << 8) | bl).toString(16).padStart(6, "0")}`;
}

/* HSL → hex(火焰/雷霆动画) */
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

/* effort 档位(low/medium/high/xhigh/max/supercode):每级独立配色与动效,递进"发热",
 * max 转为蓝色波纹呼吸,supercode 为紫色最快扫光(参照 Claude Code thinking levels) */
const EFFORT_INDEX = { low: 0, medium: 1, high: 2, xhigh: 3, max: 4, supercode: 5 };
const EFFORT_TIERS = [
  { base: "#3E5C4C", shimmer: null, sweep: 150, spin: 170, blue: false }, // low: 暗灰绿,无扫光,慢速
  { base: "#1B8A5A", shimmer: "#8CF0B6", sweep: 70, spin: 90, blue: false }, // medium: 绿色 shimmer
  { base: "#1B8A5A", shimmer: "#FFE066", sweep: 55, spin: 80, blue: false }, // high: 金色扫光带
  { base: "#B45309", shimmer: "#FFC46B", sweep: 38, spin: 70, blue: false }, // xhigh: 暖橙,扫光更快
  { base: "#0E4A78", shimmer: "#38BDF8", sweep: 38, spin: 70, blue: true },  // max: 蓝色波纹呼吸
  { base: "#4C1D95", shimmer: "#C084FC", sweep: 24, spin: 52, purple: true }, // supercode: 紫罗兰,扫光最快
];
const effortTier = (effort) => EFFORT_TIERS[EFFORT_INDEX[effort] ?? 1] || EFFORT_TIERS[1];

/* 蓝色呼吸(max):色相 200~215° 微流动,明度随动画呼吸;f 扫光提亮,fade 停滞渐暗 */
function blueColor(t, i, f, fade = 0) {
  const hue = 205 + 10 * Math.sin(t * 0.1 + i * 1.1);
  const l = Math.max(0.25, 0.52 + 0.2 * Math.sin(t * 0.15 + i * 0.8) + f * 0.25 - fade * 0.25);
  return hslToHex(hue, 0.92, l);
}

/* 紫色呼吸(supercode):色相 261~275° 微流动,明度更高、扫光更强 */
function purpleColor(t, i, f, fade = 0) {
  const hue = 268 + 7 * Math.sin(t * 0.11 + i * 1.1);
  const l = Math.max(0.28, 0.55 + 0.22 * Math.sin(t * 0.16 + i * 0.8) + f * 0.28 - fade * 0.25);
  return hslToHex(hue, 0.9, l);
}

/* 单个 spinner 字符,可错相位/调速 */
function SpinnerChar({ t, offset = 0, color, speed = 90 }) {
  const frame = SPINNER_FRAMES_SPIN[Math.floor(t / speed + offset) % SPINNER_FRAMES_SPIN.length];
  return <Text color={color}>{frame}</Text>;
}

/* 逐字符揭示文本(Claude Code "animated status word" ticker 效果)。
 * 文本变化时重置动画时钟:旧词瞬间被占位符(光标▌)覆盖,再逐字敲出新词。
 * 每个字符依次经历 ▌ → ·/_ → 定格,期间行宽不变、不跳格。
 * scanBase/scanShimmer 传入时,已定格字符叠加"逐字扫过高光"(Claude Code sweep):
 * 光带扫过的字符在 scanBase(静止)↔ scanShimmer(高光)之间瞬时提亮。
 * blue(max effort)时:字符呈蓝色波纹呼吸,扫光经过提亮。
 * fade∈[0,1] 为停滞褪色:活动超时无新输出时颜色向 fadeColor 渐暗,扫光同步减弱。 */
function Reveal({ text, t, color, dimColor, t0 = 0, scanBase, scanShimmer, fade = 0, fadeColor = "#0B3B26", blue = false, purple = false, sweepSpeed = 70 }) {
  const textRef = useRef(text);
  const startRef = useRef(null);
  if (textRef.current !== text) {
    textRef.current = text;
    startRef.current = t;
  }
  if (startRef.current === null) startRef.current = t;
  const chars = tickerChars(text, t - startRef.current, t0);
  return (
    <Text>
      {chars.map((c, i) => {
        if (!c.done) return <Text key={i} dimColor>{c.ch}</Text>;
        if (purple) {
          const f = sweepAt(t, i, chars.length, { speed: sweepSpeed }) * (1 - fade);
          return <Text key={i} color={purpleColor(t, i, f, fade)}>{c.ch}</Text>;
        }
        if (blue) {
          const f = sweepAt(t, i, chars.length, { speed: sweepSpeed }) * (1 - fade);
          return <Text key={i} color={blueColor(t, i, f, fade)}>{c.ch}</Text>;
        }
        if (scanBase && scanShimmer) {
          const base = fade > 0 ? hexInterp(scanBase, fadeColor, fade) : scanBase;
          const f = sweepAt(t, i, chars.length, { speed: sweepSpeed }) * (1 - fade);
          const col = f > 0 ? hexInterp(base, scanShimmer, f) : base;
          return <Text key={i} color={col}>{c.ch}</Text>;
        }
        return <Text key={i} color={color} dimColor={dimColor}>{c.ch}</Text>;
      })}
    </Text>
  );
}

/* 主状态行: spinner + 动词(轮播+揭示+glimmer) + 耗时 + token 计数。
 * 配色/动效随 effort 档位变化:low 暗静、medium 绿、high 金、xhigh 橙快、max 火焰+雷电。 */
function MainLine({ activity, t, color, since, tokens, width, effort }) {
  /* 工具名命中映射(Reading/Writing/Editing…)时固定动词,未命中才轮播彩蛋 */
  const mappedVerb = activity?.kind === "tool" ? TOOL_VERB_MAP[String(activity.target || "").toLowerCase()] : null;
  /* 保持数组引用稳定,避免 useRotating 的 interval 被每帧重置 */
  const verbList = activity?.kind === "tool" ? TOOL_VERBS : activity?.kind === "delegate" ? SUBAGENT_VERBS : THINKING_VERBS;
  const verb = useRotating(verbList, 3200, !!activity && !mappedVerb);
  const finalVerb = mappedVerb || verb;
  let text = "";
  if (activity?.kind === "tool") text = `${finalVerb} ${activity.target || ""}`;
  else if (activity?.kind === "delegate") text = `${finalVerb} · ${activity.target || "子任务"}`;
  else if (activity?.kind === "compacting") text = "压缩上下文";
  else text = finalVerb;

  const tier = effortTier(effort);
  const glow = glimmer(t);
  const idleMs = Math.max(0, Date.now() - (since || Date.now()));
  const fade = idleMs > 15000 ? Math.min(1, (idleMs - 15000) / 30000) : 0;
  /* 停滞褪色:活动超过 15s 无新输出 → 颜色渐暗、扫光渐隐(15s~45s 线性淡出) */
  const spinnerColor = tier.purple
    ? purpleColor(t, 0, glow, fade)
    : tier.blue
      ? blueColor(t, 0, glow, fade)
      : tier.shimmer
        ? fade > 0
          ? hexInterp(hexInterp(tier.base, "#0B3B26", fade), tier.shimmer, glow)
          : hexInterp(tier.base, tier.shimmer, glow)
        : fade > 0 ? hexInterp(tier.base, "#0B3B26", fade) : tier.base;
  const elapsed = fmtDuration(Date.now() - (since || Date.now()));
  const counter = useSmoothCounter(tokens || 0);

  return (
    <Text wrap="truncate" width={width}>
      <SpinnerChar t={t} color={spinnerColor} speed={tier.spin} />
      <Text> </Text>
      <Reveal
        text={text} t={t}
        scanBase={tier.base} scanShimmer={tier.shimmer}
        fade={fade} t0={3}
        blue={tier.blue} purple={tier.purple} sweepSpeed={tier.sweep}
      />
      <Text dimColor>… </Text>
      <Text dimColor>· {elapsed}</Text>
      {counter > 0 && <Text dimColor> · ↓ {fmtTokens(counter)} tok</Text>}
    </Text>
  );
}

/* 子 agent 活动行: 各自独立 spinner / 动词 / 耗时 / token */
function SubRow({ sub, t, offset, width, color, effort }) {
  const tier = effortTier(effort);
  const verb = useRotating(SUBAGENT_VERBS, 2600 + offset * 900, sub.busy);
  const elapsed = fmtDuration(Date.now() - (sub.startedAt || Date.now()));
  const counter = useSmoothCounter(sub.tokens || 0);
  if (sub.done) {
    const err = sub.error ? " 出错" : " 完成";
    return (
      <Text wrap="truncate" width={width}>
        <Text color={sub.error ? "red" : "green"} bold>{sub.error ? "✗" : "✓"}</Text>
        <Text> {sub.agentId}</Text>
        <Text dimColor> · {err}</Text>
        <Text dimColor> · {elapsed}</Text>
        {counter > 0 && <Text dimColor> · {fmtTokens(counter)} tok</Text>}
      </Text>
    );
  }
  const task = String(sub.task || "").replace(/\s+/g, " ").slice(0, 16);
  const glow = glimmer(t + offset * 500);
  /* 子 agent 同样适用停滞褪色:超过 15s 无输出渐暗、扫光渐隐 */
  const idleMs = Math.max(0, Date.now() - (sub.startedAt || Date.now()));
  const fade = idleMs > 15000 ? Math.min(1, (idleMs - 15000) / 30000) : 0;
  const spinnerColor = tier.purple
    ? purpleColor(t, offset * 6, glow, fade)
    : tier.blue
      ? blueColor(t, offset * 6, glow, fade)
      : tier.shimmer
        ? fade > 0
          ? hexInterp(hexInterp(tier.base, "#0B3B26", fade), tier.shimmer, glow)
          : hexInterp(tier.base, tier.shimmer, glow)
        : fade > 0 ? hexInterp(tier.base, "#0B3B26", fade) : tier.base;
  return (
    <Text wrap="truncate" width={width}>
      <SpinnerChar t={t} offset={offset} color={spinnerColor} speed={tier.spin} />
      <Text> {sub.agentId}</Text>
      <Text dimColor> · {verb}</Text>
      <Reveal
        text={task ? ` ${task}` : "…"} t={t} t0={offset + 2}
        scanBase={tier.base} scanShimmer={tier.shimmer} fade={fade}
        blue={tier.blue} purple={tier.purple} sweepSpeed={tier.sweep}
      />
      <Text dimColor> · {elapsed}</Text>
      {counter > 0 && <Text dimColor> · {fmtTokens(counter)} tok</Text>}
    </Text>
  );
}

/* 任务清单(TodoWrite/TaskUpdate 可视化,仿 Claude Code checklist) */
function TodoRows({ todos, t, width, color }) {
  const done = todos.filter((x) => x.status === "completed").length;
  const rows = todos.slice(0, MAX_TODO_ROWS);
  return (
    <>
      <Text wrap="truncate" width={width} dimColor bold>
        待办 ({done}/{todos.length}){todos.length > rows.length ? ` · 还有 ${todos.length - rows.length} 项` : ""}
      </Text>
      {rows.map((todo, i) => {
        if (todo.status === "completed") {
          return (
            <Text key={i} wrap="truncate" width={width} dimColor>
              <Text color="green">✓</Text> <Text dimColor>{todo.subject}</Text>
            </Text>
          );
        }
        if (todo.status === "in_progress") {
          const todoGlow = hexInterp("#1B8A5A", "#8CF0B6", glimmer(t + i * 500));
          return (
            <Text key={i} wrap="truncate" width={width}>
              <SpinnerChar t={t} offset={i} color={todoGlow} />
              <Text> {todo.subject}</Text>
              <Text color="#4ADE80" dimColor> · 进行中</Text>
            </Text>
          );
        }
        return (
          <Text key={i} wrap="truncate" width={width} dimColor>
            <Text color="gray">○</Text> <Text dimColor>{todo.subject}</Text>
          </Text>
        );
      })}
    </>
  );
}

/*
 * Claude Code 风格活动面板:busy 时显示主状态动画行 + 每个子 agent 活动行 + 任务清单。
 * 面板自身持有动画时钟,App 仅在状态变化时重渲染,动画不引发整屏重绘。
 */
export default function ActivityPanel({
  busy,
  activity,
  sinceRef,
  tokensRef,
  subs,
  todos,
  showTodos,
  width,
  color,
  enabled = true,
  effort,
}) {
  const t = useAnimationTime(50, enabled);

  return (
    <Box flexDirection="column">
      {busy && (
        <MainLine
          activity={activity}
          t={t}
          color={color}
          since={sinceRef?.current}
          tokens={tokensRef?.current}
          width={width}
          effort={effort}
        />
      )}
      {subs.slice(0, MAX_SUB_ROWS).map((sub, i) => (
        <SubRow key={sub.id} sub={sub} t={t} offset={i} width={width} color={color} effort={effort} />
      ))}
      {showTodos && todos.length > 0 && <TodoRows todos={todos} t={t} width={width} color={color} />}
    </Box>
  );
}

export { stringWidth };
