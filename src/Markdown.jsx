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

/* 行内样式解析: **粗体** `代码` *斜体* */
function parseInline(seg, baseColor = "white") {
  const parts = [];
  const re = /(\*\*[^*]+\*\*|`[^`]+`|\*[^*]+\*)/g;
  let last = 0;
  let m;
  while ((m = re.exec(seg))) {
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
  return parts.map((p) => (
    <Text
      key={Math.random()}
      bold={p.bold}
      italic={p.italic}
      color={p.code ? "yellow" : baseColor}
      backgroundColor={p.code ? "#1a1a2e" : undefined}
    >
      {p.text}
    </Text>
  ));
}

/* 单行渲染(带行内解析) */
function renderLine(line, baseColor, width) {
  const words = line.split(/(\s+)/);
  const chunks = [];
  let cur = "";
  const flush = () => {
    if (cur) {
      chunks.push(cur);
      cur = "";
    }
  };
  for (const w of words) {
    if ((cur + w).length > width - 6) {
      flush();
      cur = w;
    } else {
      cur += w;
    }
  }
  flush();
  return chunks.length
    ? chunks.map((c) => <Text key={Math.random()} color={baseColor} wrap="wrap">{parseInline(c, baseColor)}</Text>)
    : <Text color={baseColor}>{parseInline(line, baseColor)}</Text>;
}

/* Markdown 块解析为行数组 */
export function markdownToLines(md, width) {
  const lines = [];
  const src = String(md || "").replace(/\r\n/g, "\n").split("\n");
  let i = 0;
  while (i < src.length) {
    const line = src[i];

    /* 代码块 */
    if (/^```/.test(line.trim())) {
      const lang = line.trim().slice(3).trim();
      const code = [];
      i++;
      while (i < src.length && !/^```/.test(src[i].trim())) {
        code.push(src[i]);
        i++;
      }
      i++; // 跳过闭合
      lines.push({ type: "code", text: code.join("\n"), lang });
      continue;
    }

    /* 标题 */
    const h = /^(#{1,4})\s+(.*)$/.exec(line);
    if (h) {
      lines.push({ type: "heading", level: h[1].length, text: h[2] });
      i++;
      continue;
    }

    /* 列表项 */
    const li = /^(\s*)[-*+]\s+(.*)$/.exec(line);
    if (li) {
      lines.push({ type: "list", indent: li[1].length, text: li[2] });
      i++;
      continue;
    }

    /* 有序列表 */
    const oi = /^(\s*)\d+[.)]\s+(.*)$/.exec(line);
    if (oi) {
      lines.push({ type: "list", indent: oi[1].length, ordered: true, text: oi[2] });
      i++;
      continue;
    }

    /* 引用 */
    const qt = /^>\s?(.*)$/.exec(line);
    if (qt) {
      lines.push({ type: "quote", text: qt[1] });
      i++;
      continue;
    }

    /* 分隔线 */
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

export default function Markdown({ content, width = 100 }) {
  const lines = markdownToLines(content, width);
  return (
    <Box flexDirection="column">
      {lines.map((l, idx) => {
        if (l.type === "code") {
          const codeLines = l.text.split("\n");
          return (
            <Box key={idx} flexDirection="column" marginY={1} borderStyle="round" borderColor="gray">
              {l.lang && (
                <Text dimColor backgroundColor="#1a1a2e">
                  {" "}{l.lang}{" "}
                </Text>
              )}
              {codeLines.map((cl, ci) => (
                <Text key={ci} color="yellow" backgroundColor="#1a1a2e" wrap="wrap">
                  {cl || " "}
                </Text>
              ))}
            </Box>
          );
        }
        if (l.type === "heading") {
          const color = l.level === 1 ? "cyan" : l.level === 2 ? "blue" : "white";
          return (
            <Text key={idx} bold color={color}>
              {"#".repeat(l.level)} {l.text}
            </Text>
          );
        }
        if (l.type === "list") {
          return (
            <Text key={idx} color="white" wrap="wrap">
              {"  ".repeat(Math.min(l.indent, 4) / 2 || 0)}
              {"  "}{l.ordered ? "" : "• "}{parseInline(l.text)}
            </Text>
          );
        }
        if (l.type === "quote") {
          return (
            <Text key={idx} color="gray" wrap="wrap">
              {"  "}▍{parseInline(l.text, "gray")}
            </Text>
          );
        }
        if (l.type === "hr") {
          return <Text key={idx} dimColor>{"─".repeat(Math.min(width - 6, 40))}</Text>;
        }
        return (
          <Text key={idx} color="white" wrap="wrap">
            {parseInline(l.text)}
          </Text>
        );
      })}
    </Box>
  );
}
