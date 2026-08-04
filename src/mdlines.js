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

/* Markdown → 带样式行数组（供 TUI 精确滚动渲染）。
 * 解析用 marked（标准库），输出为 {text, color, bold, dim, indent} 行。
 * 每行渲染后固定 1 行高度，滚动即行索引偏移，不会因布局估算崩坏。 */

import { marked } from "marked";
import wrapAnsi from "wrap-ansi";
import stringWidth from "string-width";

marked.setOptions({ gfm: true, breaks: false });

/* 行模型: { text, color?, bold?, dim?, indent? } */
function clean(t) {
  return String(t == null ? "" : t)
    .replace(/\u001b\[[0-9;]*m/g, "")
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, "")
    .replace(/\u200b/g, "");
}

/* 行内 token → 带样式片段 */
function inlineSpans(tokens, base) {
  const spans = [];
  const push = (text, style) => {
    text = clean(text);
    if (!text) return;
    spans.push({ text, ...base, ...style });
  };
  const walk = (ts) => {
    for (const t of ts || []) {
      switch (t.type) {
        case "text": push(t.text, {}); break;
        case "strong": walk(t.tokens); break;
        case "em": walk(t.tokens); break;
        case "codespan": push(t.text, { color: "yellow" }); break;
        case "link": walk(t.tokens); break;
        case "del": walk(t.tokens); break;
        case "br": push(" ", {}); break;
        default:
          if (t.tokens) walk(t.tokens);
          else if (t.text) push(t.text, {});
      }
    }
  };
  walk(tokens);
  if (!spans.length) push("", {});
  return spans;
}

/* 行内文本(无样式) */
function inlineText(tokens) {
  return tokens?.map((t) => t.text || "").join("") || "";
}

/* wrap 后按行输出（每行固定 1 高度） */
function emitWrapped(spans, width, out, style = {}) {
  let line = "";
  let lineSpans = [];
  const flushLine = () => {
    if (!line) return;
    out.push({ spans: lineSpans, indent: style.indent || 0, color: style.color, bold: style.bold, dim: style.dim });
    line = "";
    lineSpans = [];
  };
  const maxW = Math.max(width - 2, 10);
  for (const s of spans) {
    /* 按空格切词，逐词 wrap，避免截断 CJK */
    const words = String(s.text).split(/(\s+)/);
    for (const w of words) {
      const ww = stringWidth(w);
      if (ww > maxW) {
        /* 超长词硬切 */
        let rest = w;
        while (stringWidth(rest) > maxW) {
          let cut = 0;
          let acc = 0;
          while (cut < rest.length && acc + stringWidth(rest[cut]) <= maxW) {
            acc += stringWidth(rest[cut]);
            cut++;
          }
          if (cut === 0) cut = 1;
          const piece = rest.slice(0, cut);
          if (line) flushLine();
          out.push({
            spans: [{ text: piece, color: s.color, bold: s.bold, dim: s.dim }],
            indent: style.indent || 0,
            color: style.color, bold: style.bold, dim: style.dim,
          });
          rest = rest.slice(cut);
          line = "";
        }
        if (rest) { line = rest; lineSpans = [{ text: rest, color: s.color, bold: s.bold, dim: s.dim }]; }
      } else if (stringWidth(line) + ww > maxW) {
        flushLine();
        line = w;
        lineSpans = [{ text: w, color: s.color, bold: s.bold, dim: s.dim }];
      } else {
        line += w;
        lineSpans.push({ text: w, color: s.color, bold: s.bold, dim: s.dim });
      }
    }
  }
  flushLine();
}

/* 行级简易 wrap（无样式文本） */
function emitPlain(text, width, out, style = {}) {
  const maxW = Math.max(width - 2, 10);
  if (stringWidth(text) <= maxW) {
    out.push({ text, spans: null, indent: style.indent || 0, color: style.color, bold: style.bold, dim: style.dim });
    return;
  }
  emitWrapped([{ text, color: style.color, bold: style.bold, dim: style.dim }], width, out, style);
}

/* 单元格文本 → 按列宽换行后补到等宽的多行数组（CJK/emoji 按显示宽度,代理对不截断） */
function cellWrap(text, w) {
  if (w <= 0) return [" ".repeat(1)];
  const out = [];
  let rest = text;
  while (stringWidth(rest) > w) {
    let cut = 0, acc = 0;
    for (const ch of rest) {
      if (acc + stringWidth(ch) > w) break;
      acc += stringWidth(ch);
      cut += ch.length;
    }
    if (cut === 0) cut = 1;
    out.push(rest.slice(0, cut) + " ".repeat(w - acc));
    rest = rest.slice(cut);
  }
  if (rest || !out.length) out.push(rest + " ".repeat(Math.max(0, w - stringWidth(rest))));
  return out;
}

/* 单元格 → 纯文本 */
function cellText(c) {
  if (c == null) return "";
  const raw = c.tokens ? inlineText(c.tokens) : c.text || "";
  return clean(raw).replace(/\s+/g, " ").trim();
}

/* Markdown 字符串 → 行数组。style: {indent, color} 基线样式 */
export function markdownLines(md, width, style = {}) {
  const out = [];
  const base = { ...style };
  const tokens = marked.lexer(String(md ?? ""));

  const walk = (tokens) => {
    for (const t of tokens) {
      switch (t.type) {
        case "heading": {
          const color = t.depth <= 1 ? "cyan" : t.depth === 2 ? "blue" : "white";
          out.push({
            spans: inlineSpans(t.tokens, { color, bold: true }),
            indent: base.indent || 0,
            color, bold: true,
            header: true,
          });
          break;
        }
        case "code": {
          const codeStyle = { color: "yellow", bold: false };
          const lines = clean(t.text).split("\n");
          if (t.lang) {
            out.push({ text: t.lang, spans: null, indent: (base.indent || 0) + 1, color: "gray", dim: true, code: true });
          }
          for (const l of lines) {
            if (!l && lines.length === 1) continue;
            emitPlain(l || " ", width, out, { indent: (base.indent || 0) + 1, color: codeStyle.color, code: true });
          }
          break;
        }
        case "list": {
          let idx = 1;
          for (const item of t.items) {
            const mark = t.ordered ? `${idx}.` : "•";
            const first = inlineSpans(item.tokens, {});
            const rest = item.tokens?.length > 1 ? item.tokens.slice(1) : [];
            /* 第一行：前缀 + 首段 inline */
            const prefix = { text: ` ${mark} `, color: base.color, dim: true };
            const headSpans = [prefix, ...inlineSpans(item.tokens[0]?.tokens || [], {})];
            emitWrapped(headSpans, width, out, { indent: (base.indent || 0) + 2 });
            /* 后续段落/嵌套 */
            for (const st of item.tokens.slice(1)) {
              if (st.type === "list") walk([st]);
              else if (st.type === "text") {
                emitWrapped(inlineSpans(st.tokens || [], {}), width, out, { indent: (base.indent || 0) + 4 });
              }
            }
            idx++;
          }
          break;
        }
        case "blockquote": {
          const qLines = [];
          const qTokens = t.tokens || [];
          for (const qt of qTokens) {
            if (qt.type === "paragraph") {
              emitWrapped(inlineSpans(qt.tokens || [], { color: "gray" }), width, out, { indent: (base.indent || 0) + 2, color: "gray", dim: true });
            } else if (qt.type === "list") walk([qt]);
          }
          break;
        }
        case "paragraph": {
          emitWrapped(inlineSpans(t.tokens || [], {}), width, out, base);
          break;
        }
        case "hr":
          out.push({ text: "─".repeat(Math.min(width - 4, 40)), spans: null, indent: base.indent || 0, color: "gray", dim: true });
          break;
        case "table": {
          /* 列宽对齐 + 外边框 + 单元格内联样式保留(加粗/代码等)。
           * 短单元格保留 spans 渲染，超宽则回退纯文本换行。 */
          const headerCells = t.header || [];
          const bodyRows = t.rows || [];
          const numCols = Math.max(headerCells.length, ...bodyRows.map((r) => r.length), 1);
          const pad = (arr) => {
            const a = arr.slice(0, numCols);
            while (a.length < numCols) a.push({ tokens: [], text: "" });
            return a;
          };
          const allRows = [pad(headerCells), ...bodyRows.map(pad)];
          const widths = [];
          for (let c = 0; c < numCols; c++) {
            let w = 1;
            for (const r of allRows) w = Math.max(w, stringWidth(cellText(r[c])));
            widths.push(w + 2);
          }
          const maxW = Math.max(width - 2 - (base.indent || 0), 8);
          let totalW = 2 + widths.reduce((a, b) => a + b, 0) + (numCols - 1);
          let guard = 0;
          while (totalW > maxW && guard < 200) {
            guard++;
            let mi = 0;
            for (let i = 1; i < numCols; i++) if (widths[i] > widths[mi]) mi = i;
            if (widths[mi] <= 1) break;
            widths[mi]--;
            totalW--;
          }
          /* 单元格按列宽换行，返回 span[][]（每行一个 span 数组） */
          const cellSpanLines = (cell, colW) => {
            const tokens = cell.tokens || [];
            const spans = inlineSpans(tokens, { color: "white" });
            const text = inlineText(tokens);
            const contentW = colW - 2; // 1 space padding each side
            if (stringWidth(text) <= contentW) {
              const padded = text + " ".repeat(contentW - stringWidth(text));
              return [[{ text: " " + padded + " ", color: "white" }]];
            }
            const wrapped = cellWrap(text, contentW);
            return wrapped.map((l) => [{ text: " " + l + " ".repeat(contentW - stringWidth(l)), color: "white" }]);
          };
          const rowSpanSegs = (cells) => cells.map((cell, i) => cellSpanLines(cell, widths[i]));
          /* 顶部边框 */
          out.push({
            text: "┌" + widths.map((w) => "─".repeat(w)).join("┬") + "┐",
            spans: null, indent: base.indent || 0, color: "gray",
          });
          /* 逐行渲染：每个单元格可能多行（换行），跨单元格对齐 */
          const emitBody = (cells, spanSegs, isLast) => {
            const n = Math.max(...spanSegs.map((s) => s.length));
            for (let l = 0; l < n; l++) {
              const lineSpans = [{ text: "│", color: "gray" }];
              for (let c = 0; c < numCols; c++) {
                if (l < spanSegs[c].length) {
                  lineSpans.push(...spanSegs[c][l]);
                } else {
                  lineSpans.push({ text: " ".repeat(widths[c]), color: "white" });
                }
                if (c < numCols - 1) lineSpans.push({ text: "│", color: "gray" });
              }
              lineSpans.push({ text: "│", color: "gray" });
              const isBottom = isLast && l === n - 1;
              out.push({
                spans: lineSpans, indent: base.indent || 0,
                color: isBottom ? "gray" : "white", bold: false, dim: isBottom,
              });
            }
          };
          emitBody(allRows[0], rowSpanSegs(allRows[0]), allRows.length === 1);
          if (allRows.length > 1) {
            out.push({
              text: "├" + widths.map((w) => "─".repeat(w)).join("┼") + "┤",
              spans: null, indent: base.indent || 0, color: "gray",
            });
          }
          for (let r = 1; r < allRows.length; r++) {
            emitBody(allRows[r], rowSpanSegs(allRows[r]), r === allRows.length - 1);
          }
          out.push({
            text: "└" + widths.map((w) => "─".repeat(w)).join("┴") + "┘",
            spans: null, indent: base.indent || 0, color: "gray",
          });
          break;
        }
        case "space": break;
        case "html": break;
        default:
          if (t.type === "text" && t.tokens) {
            emitWrapped(inlineSpans(t.tokens, {}), width, out, base);
          }
      }
    }
  };

  /* 段落相邻合并空行 */
  walk(tokens);
  const merged = [];
  for (const l of out) {
    const last = merged[merged.length - 1];
    if (l.text === "" || (l.spans && l.spans.every((s) => !s.text))) continue;
    merged.push(l);
  }
  return merged;
}

export { stringWidth };

/* unified diff → 带颜色行数组（+绿/-红/头蓝/@@青）。返回 {text,color,bold,dim} 行 */
function classifyDiffLine(line) {
  if (line.startsWith("diff --git") || line.startsWith("index ") ||
      line.startsWith("--- ") || line.startsWith("+++ ")) {
    return { color: "blue", bold: false };
  }
  if (line.startsWith("@")) return { color: "cyan", bold: true };
  if (line.startsWith("+")) return { color: "green", bold: false };
  if (line.startsWith("-")) return { color: "red", bold: false };
  return { color: "white", bold: false };
}

export function diffLines(diffText, width) {
  const out = [];
  const maxW = Math.max(width - 2, 10);
  const raw = clean(String(diffText ?? "")).split("\n");
  for (const line of raw) {
    if (!line.trim() && !line.startsWith("+") && !line.startsWith("-")) {
      if (out.length) out.push({ text: "", spans: null, indent: 0 });
      continue;
    }
    const { color, bold } = classifyDiffLine(line);
    if (stringWidth(line) <= maxW) {
      out.push({ text: line, spans: null, indent: 0, color, bold });
    } else {
      for (const piece of wrapPlain(line, maxW)) {
        out.push({ text: piece, spans: null, indent: 0, color, bold });
      }
    }
  }
  return out.filter((l) => l.text !== "");
}

/* 纯文本按宽度换行（thinking 等非 markdown 长文本），返回行数组。
 * O(n) 逐字符累计显示宽度,避免对巨型单行反复 stringWidth(全行) 导致 O(n²)。 */
export function wrapPlain(text, width) {
  const maxW = Math.max(width - 2, 10);
  const raw = clean(String(text ?? "")).split("\n");
  const out = [];
  for (const line of raw) {
    if (!line) { out.push(""); continue; }
    if (stringWidth(line) <= maxW) { out.push(line); continue; }
    let pos = 0;
    while (pos < line.length) {
      let end = pos + 1;
      while (end <= line.length && stringWidth(line.slice(pos, end)) <= maxW) end++;
      end--;
      if (end <= pos) end = pos + 1;
      out.push(line.slice(pos, end));
      pos = end;
    }
  }
  return out;
}
