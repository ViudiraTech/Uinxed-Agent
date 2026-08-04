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

import React from "react";
import { Box, Text } from "ink";
import {
  SPINNER_FRAMES_SPIN,
  THINKING_VERBS,
  TOOL_VERBS,
  SUBAGENT_VERBS,
  useAnimationTime,
  useRotating,
  fmtDuration,
  fmtTokens,
  useSmoothCounter,
  tickerChars,
  glimmer,
} from "./anim.js";
import { stringWidth } from "./mdlines.js";

/* 与 App.jsx 保持一致的动画行数预算(用于动态布局预留)。
 * 折叠 1 个问题:面板高度恒定为 AC inLayoutFix,忙碌时不再随行数跳变,
 * 消息区高度稳定,动画位置不抖动。 */
export const MAX_SUB_ROWS = 4;
export const MAX_TODO_ROWS = 5;
/* 面板固定高度:主状态行 1 + 子 agent 最多 4 + 待办头 1 + 待办 3 → 常量 6 */
export const ACTIVITY_HEIGHT = 6;

export function activityRowCount({ busy, subs, todos, showTodos }) {
  const hasAny = busy || subs.length || (showTodos && todos.length > 0);
  return hasAny ? ACTIVITY_HEIGHT : 0;
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

/* 单个 spinner 字符,可错相位 */
function SpinnerChar({ t, offset = 0, color }) {
  const frame = SPINNER_FRAMES_SPIN[Math.floor(t / 90 + offset) % SPINNER_FRAMES_SPIN.length];
  return <Text color={color}>{frame}</Text>;
}

/* 逐字符揭示文本(Claude Code "animated status word" ticker 效果) */
function Reveal({ text, t, color, dimColor, t0 = 0 }) {
  const chars = tickerChars(text, t, t0);
  return (
    <Text color={color} dimColor={dimColor}>
      {chars.map((c) => c.ch).join("")}
    </Text>
  );
}

/* 主状态行: spinner + 动词(轮播+揭示+glimmer) + 耗时 + token 计数 */
function MainLine({ activity, t, color, since, tokens, width }) {
  /* 保持数组引用稳定,避免 useRotating 的 interval 被每帧重置 */
  const verbList = activity?.kind === "tool" ? TOOL_VERBS : activity?.kind === "delegate" ? SUBAGENT_VERBS : THINKING_VERBS;
  const verb = useRotating(verbList, 3200, !!activity);
  let text = "";
  if (activity?.kind === "tool") text = `${verb} ${activity.target || ""}`;
  else if (activity?.kind === "delegate") text = `${verb} · ${activity.target || "子任务"}`;
  else if (activity?.kind === "compacting") text = "压缩上下文";
  else text = verb;

  const glow = glimmer(t);
  const mainColor = color || "magenta";
  const glimmerColor = hexInterp("#8a92b0", mainColor, glow);
  const elapsed = fmtDuration(Date.now() - (since || Date.now()));
  const counter = useSmoothCounter(tokens || 0);

  return (
    <Text wrap="truncate" width={width}>
      <SpinnerChar t={t} color={mainColor} />
      <Text> </Text>
      <Reveal text={text} t={t} color={glimmerColor} t0={3} />
      <Text dimColor>… </Text>
      <Text dimColor>· {elapsed}</Text>
      {counter > 0 && <Text dimColor> · ↓ {fmtTokens(counter)} tok</Text>}
    </Text>
  );
}

/* 子 agent 活动行: 各自独立 spinner / 动词 / 耗时 / token */
function SubRow({ sub, t, offset, width, color }) {
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
  return (
    <Text wrap="truncate" width={width}>
      <SpinnerChar t={t} offset={offset} color={color} />
      <Text> {sub.agentId}</Text>
      <Text dimColor> · {verb}</Text>
      <Reveal text={task ? ` ${task}` : "…"} t={t} t0={offset + 2} dimColor />
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
          return (
            <Text key={i} wrap="truncate" width={width}>
              <SpinnerChar t={t} offset={i} color={color} />
              <Text> {todo.subject}</Text>
              <Text color={color} dimColor> · 进行中</Text>
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
        />
      )}
      {subs.slice(0, MAX_SUB_ROWS).map((sub, i) => (
        <SubRow key={sub.id} sub={sub} t={t} offset={i} width={width} color={color} />
      ))}
      {showTodos && todos.length > 0 && <TodoRows todos={todos} t={t} width={width} color={color} />}
    </Box>
  );
}

export { stringWidth };
