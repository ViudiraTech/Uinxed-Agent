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

/* opencode 风格 thinking 折叠块
 * 显示 "Thought for Xs" 一行, Ctrl+T 切换展开 */
export default function ThinkingBlock({ reasoning, expanded, onToggle }) {
  if (!reasoning) return null;

  const lines = String(reasoning).trim().split("\n");
  const seconds = Math.max(1, Math.round(lines.length / 20));

  return (
    <Box flexDirection="column" width="100%">
      <Text bold color="#9aa3c2" wrap="wrap">
        <Text color="yellow">◈ </Text>
        <Text color={expanded ? "cyan" : "#9aa3c2"} bold={expanded}>
          {expanded ? `Thought for ${seconds}s ▾` : `Thought for ${seconds}s ▸`}
        </Text>
      </Text>
      {expanded && (
        <Box
          flexDirection="column"
          marginTop={0}
          marginLeft={3}
          borderStyle="round"
          borderColor="#3d4460"
          paddingX={1}
        >
          {lines.slice(0, 120).map((l, i) => (
            <Text key={i} color="#8a92b0" wrap="wrap">
              {l || " "}
            </Text>
          ))}
          {lines.length > 120 && <Text color="#6b7392">…（推理过长，已截断）</Text>}
        </Box>
      )}
    </Box>
  );
}
