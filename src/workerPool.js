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

/* 工具 worker 线程池:最多 MAX_WORKERS 个 worker 并发执行重型工具调用,
 * 每个 worker 在执行完自己当前任务(队列清空)后退出,再按需拉起新线程。
 * 工具执行全程不占用主线程事件循环,UI 动画/流式输出不再卡顿。 */

import { Worker } from "node:worker_threads";
import path from "node:path";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
/* 生产(dist)下 worker 被 esbuild 单独打包为 tool-worker.mjs;
 * 开发(node src/index.js)下直接用源码 toolWorker.js。 */
let WORKER_FILE = path.join(HERE, "tool-worker.mjs");
if (!fs.existsSync(WORKER_FILE)) WORKER_FILE = path.join(HERE, "toolWorker.js");

const MAX_WORKERS = 3;
const SAFETY_TIMEOUT_MS = 15 * 60 * 1000;

let active = 0;
const queue = [];

function pump() {
  while (active < MAX_WORKERS && queue.length > 0) {
    const job = queue.shift();
    active += 1;
    spawn(job);
  }
}

function spawn(job) {
  const worker = new Worker(WORKER_FILE);
  let done = false;
  const finish = (result) => {
    if (done) return;
    done = true;
    clearTimeout(job.timer);
    worker.terminate().catch(() => {});
    active -= 1;
    job.resolve(result);
    pump();
  };
  job.timer = setTimeout(() => finish({ error: "工具执行超时(15 分钟),线程已终止" }), SAFETY_TIMEOUT_MS);
  worker.once("message", finish);
  worker.once("error", (err) => finish({ error: String(err?.message || err) }));
  worker.once("exit", (code) => finish({ error: `工具线程异常退出(code=${code})` }));
  worker.postMessage({ name: job.name, args: job.args, cwd: job.cwd });
}

/* 在 worker 线程中执行工具,返回 Promise<可序列化结果> */
export function runToolInWorker(name, args, cwd) {
  return new Promise((resolve) => {
    queue.push({ name, args, cwd, resolve, timer: null });
    pump();
  });
}
