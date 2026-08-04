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

import { useEffect, useState } from "react";

/* ============ Claude Code 风格动画原语 ============ */

/* Claude Code 的星形 spinner 帧(正反循环,同宽字符避免跳格) */
export const SPINNER_FRAMES = ["·", "✢", "*", "✶", "✻", "✽"];
export const SPINNER_FRAMES_SPIN = [
  ...SPINNER_FRAMES,
  ...[...SPINNER_FRAMES].reverse(),
];

/* 思考动词(英文,参考 Claude Code 的 spinnerVerbs) */
export const THINKING_VERBS = [
  "Thinking", "Reasoning", "Analyzing", "Composing", "Formulating",
  "Pondering", "Deliberating", "Synthesizing", "Evaluating", "Planning",
  "Mapping", "Outlining", "Verifying", "Tracing", "Weighing", "Concluding",
];

/* 工具执行动词 */
export const TOOL_VERBS = [
  "Running", "Executing", "Processing", "Calling", "Scanning", "Reading",
  "Writing", "Editing", "Searching", "Computing", "Fetching", "Generating",
];

/* 子 agent 工作动词 */
export const SUBAGENT_VERBS = [
  "Exploring", "Researching", "Investigating", "Combing", "Locating",
  "Scanning", "Comparing", "Studying", "Reasoning", "Executing",
  "Implementing", "Advancing", "Verifying", "Reporting",
];

export const pick = (arr) => arr[Math.floor(Math.random() * arr.length)];

/* 动画时钟:每 interval ms 触发一次重渲染,返回单调递增时间(ms) */
export function useAnimationTime(interval = 50, enabled = true) {
  const [t, setT] = useState(0);
  useEffect(() => {
    if (!enabled) return;
    const id = setInterval(() => setT((x) => x + interval), interval);
    return () => clearInterval(id);
  }, [interval, enabled]);
  return t;
}

/* 每 changeEvery ms 随机切换一次(用于动词轮播) */
export function useRotating(choices, changeEvery = 3200, enabled = true) {
  const [idx, setIdx] = useState(() => Math.floor(Math.random() * choices.length));
  useEffect(() => {
    if (!enabled) return;
    const id = setInterval(() => setIdx((i) => (i + 1) % choices.length), changeEvery);
    return () => clearInterval(id);
  }, [choices, changeEvery, enabled]);
  return choices[idx];
}

/* 耗时格式化: 45s / 3m 12s / 1h 05m */
export function fmtDuration(ms) {
  const total = Math.max(0, Math.floor(ms / 1000));
  const s = total % 60;
  const m = Math.floor(total / 60) % 60;
  const h = Math.floor(total / 3600);
  if (h > 0) return `${h}h ${String(m).padStart(2, "0")}m`;
  if (m > 0) return `${m}m ${String(s).padStart(2, "0")}s`;
  return `${s}s`;
}

/* token 数格式化: 1.2k / 340k / 1.0M */
export function fmtTokens(n) {
  n = Math.max(0, Math.round(n || 0));
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

/* 平滑计数器:向目标值渐进,避免逐字跳动(仿 Claude Code token 计数动画) */
export function useSmoothCounter(target, speed = 60) {
  const [val, setVal] = useState(target);
  useEffect(() => {
    if (target <= val) { setVal(target); return; }
    const gap = target - val;
    const step = gap < 80 ? 3 : Math.max(10, Math.ceil(gap * 0.12));
    const id = setTimeout(() => setVal((v) => Math.min(v + step, target)), 40);
    return () => clearTimeout(id);
  }, [target, val]);
  return val;
}

/* 稳定哈希:同一 i 每次得到相同占位字符 */
function placeholder(i, phase) {
  if (phase === 0) return "▌";
  if (phase === 1) return i % 3 === 0 ? "·" : "_";
  return i % 2 === 0 ? "_" : "·";
}

/* 逐字符揭示的 ticker 文本(Claude Code 的 "animated status word" 效果)。
 * time 为动画时钟;每个字符经历 ▌ → ·/_ → 若隐若现 → 定格。
 * 占位符按原字符宽度补齐,保证动画期间行宽不变、不跳格。 */
export function tickerChars(text, time, t0 = 0) {
  const chars = [...String(text)];
  return chars.map((ch, i) => {
    const phase = Math.floor(time / 45) - i - t0;
    if (phase >= 3) return { ch, done: true };
    if (phase < 0) return { ch: " ".repeat(widthOf(ch)), done: false };
    const w = widthOf(ch);
    const p = placeholder(i, phase);
    return { ch: w > 1 ? p + " " : p, done: false };
  });
}

/* 终端显示宽度(CJK 双宽) */
function widthOf(ch) {
  const c = ch.codePointAt(0);
  if (c >= 0x2e80 && c <= 0x9fff) return 2; // CJK 统一表意文字
  if (c >= 0xf900 && c <= 0xfaff) return 2; // 兼容表意文字
  if (c >= 0xff00 && c <= 0xffef) return 2; // 全角
  if (c >= 0x3000 && c <= 0x303f) return 2; // 中日韩标点
  return 1;
}

/* 思考 glimmer:正弦波透明度,返回 0..1 (仿 Claude "thinking shimmer") */
export function glimmer(time, period = 2600) {
  return (Math.sin((time / period) * Math.PI * 2 - Math.PI / 2) + 1) / 2;
}