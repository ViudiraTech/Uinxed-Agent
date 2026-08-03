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
          const rows = [t.header, ...t.rows];
          for (const row of rows) {
            const cells = row.map((c) => clean(c.tokens ? inlineText(c.tokens) : c.text || ""));
            emitPlain(` ${cells.join(" │ ")} `, width, out, { indent: (base.indent || 0) + 1, color: "white" });
          }
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

/* 纯文本按宽度换行（thinking 等非 markdown 长文本），返回行数组 */
export function wrapPlain(text, width) {
  const maxW = Math.max(width - 2, 10);
  const raw = clean(String(text ?? "")).split("\n");
  const out = [];
  for (const line of raw) {
    if (!line) { out.push(""); continue; }
    if (stringWidth(line) <= maxW) { out.push(line); continue; }
    let rest = line;
    while (stringWidth(rest) > maxW) {
      let cut = Math.max(1, Math.floor(maxW / 2));
      while (cut < rest.length && stringWidth(rest.slice(0, cut)) < maxW) cut++;
      out.push(rest.slice(0, cut));
      rest = rest.slice(cut);
    }
    if (rest) out.push(rest);
  }
  return out;
}
