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

/* 工具执行 worker:接收 {name, args, cwd},在独立线程里执行重型/同步工具
 * (bash / read_file / write_file / edit_file / list_dir / grep / glob),
 * 完成后把可序列化结果发回主线程,线程随即退出。
 * 阻塞型工具(execSync、大文件读写、目录遍历)不再卡住 UI 主线程。 */

import { parentPort } from "node:worker_threads";
import { executeTool } from "./tools.js";

parentPort.on("message", async (job) => {
  try {
    const result = await executeTool(job.name, job.args || {}, job.cwd || process.cwd(), {});
    parentPort.postMessage(result);
  } catch (e) {
    parentPort.postMessage({ error: String(e?.message || e) });
  }
});
