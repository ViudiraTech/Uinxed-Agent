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

import React, { memo } from "react";
import { Text } from "ink";
import { markdownLines } from "./mdlines.js";

/* 单行渲染：每行固定 1 行高 */
function Line({ line, width }) {
  const indent = " ".repeat(line.indent || 0);
  if (line.spans) {
    return (
      <Text wrap="truncate" width={Math.max(width, 10)}>
        {indent}
        {line.spans.map((s, i) => (
          <Text
            key={i}
            bold={s.bold}
            dim={s.dim || line.dim}
            color={s.color || line.color}
            backgroundColor={s.code ? "#1a1a2e" : undefined}
          >
            {s.text}
          </Text>
        ))}
      </Text>
    );
  }
  return (
    <Text wrap="truncate" width={Math.max(width, 10)} bold={line.bold} dim={line.dim} color={line.color} backgroundColor={line.code ? "#1a1a2e" : undefined}>
      {indent}{line.text}
    </Text>
  );
}

export { Line as LineRow };

/* Markdown 渲染：marked 解析 → 行模型，每行高度固定为 1，
 * 上层负责按行窗口切片，滚动精确。 */
const Markdown = memo(function Markdown({ content, width = 100 }) {
  const lines = markdownLines(content, width);
  return (
    <>
      {lines.map((l, i) => (
        <Line key={i} line={l} width={width} />
      ))}
    </>
  );
});

export default Markdown;
