#!/usr/bin/env node
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
import { render } from "ink";
import App from "./App.jsx";
import { loadConfig, DEFAULT_BASE_URL } from "./config.js";

const args = process.argv.slice(2);
if (args.includes("--version") || args.includes("-v")) {
  console.log("ux-agent 1.0.0");
  process.exit(0);
}
if (args.includes("--help") || args.includes("-h")) {
  console.log(`Uinxed AI TUI Agent

用法:
  ux-agent                启动交互界面
  ux-agent --key <key>    设置 API Key 并启动
  ux-agent --base <url>   设置接口地址(默认 ${DEFAULT_BASE_URL})
  ux-agent --model <id>   设置默认模型
  ux-agent --reset        清除全部配置与历史
  ux-agent --version      版本信息
`);
  process.exit(0);
}

if (args.includes("--reset")) {
  const fs = await import("node:fs");
  const os = await import("node:os");
  const path = await import("node:path");
  fs.rmSync(path.join(os.homedir(), ".config", "ux-agent"), { recursive: true, force: true });
  console.log("配置已清除");
  process.exit(0);
}

const cliConfig = {};
for (let i = 0; i < args.length; i++) {
  if (args[i] === "--key" && args[i + 1]) cliConfig.apiKey = args[i + 1];
  if (args[i] === "--base" && args[i + 1]) cliConfig.baseUrl = args[i + 1];
  if (args[i] === "--model" && args[i + 1]) cliConfig.model = args[i + 1];
  if (args[i] === "--provider" && args[i + 1]) cliConfig.provider = args[i + 1];
}
if (cliConfig.apiKey || cliConfig.baseUrl || cliConfig.model) {
  const { saveConfig, setProviderApiKey, setActiveProvider } = await import("./config.js");
  if (cliConfig.provider) setActiveProvider(cliConfig.provider);
  const active = (await import("./config.js")).getActiveProvider();
  if (cliConfig.apiKey) setProviderApiKey(active.id, cliConfig.apiKey);
  if (cliConfig.baseUrl || cliConfig.model) saveConfig(cliConfig);
  console.log(
    `已配置: ${Object.entries(cliConfig)
      .map(([k, v]) => `${k}=${k === "apiKey" ? v.slice(0, 8) + "…" : v}`)
      .join(", ")}`
  );
}

const cfg = loadConfig();
if (!cfg.apiKey) {
  console.log("提示: 未检测到 API Key。可直接在界面内输入，或用 --key 参数指定。");
}

render(React.createElement(App));
