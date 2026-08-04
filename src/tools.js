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

import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

/* ============ 工具注册表 ============ */

export const TOOL_DEFS = [
  {
    type: "function",
    function: {
      name: "bash",
      description: "在指定工作目录执行 shell 命令，输出 stdout/stderr。适合运行构建、测试、git 操作等。",
      parameters: {
        type: "object",
        properties: {
          cmd: { type: "string", description: "要执行的 shell 命令" },
          timeout: { type: "number", description: "超时秒数，默认 30" },
        },
        required: ["cmd"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "read_file",
      description: "读取文本文件内容（最多 5000 行）。",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "文件路径" },
          offset: { type: "number", description: "起始行号（1 开始）" },
          limit: { type: "number", description: "读取行数，默认 500" },
        },
        required: ["path"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "write_file",
      description: "覆盖写入文件（会创建目录）。用于创建或整体重写文件。",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "文件路径" },
          content: { type: "string", description: "文件完整内容" },
        },
        required: ["path", "content"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "edit_file",
      description: "在文件中精确替换一段文本（old 必须唯一）。用于修改已有文件。",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "文件路径" },
          old: { type: "string", description: "要替换的原文" },
          new: { type: "string", description: "替换后的内容" },
        },
        required: ["path", "old", "new"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "list_dir",
      description: "列出目录内容（含大小与类型）。",
      parameters: {
        type: "object",
        properties: {
          path: { type: "string", description: "目录路径，默认当前工作目录" },
        },
      },
    },
  },
  {
    type: "function",
    function: {
      name: "grep",
      description: "在目录中按正则搜索文件内容，返回匹配的文件与行号。",
      parameters: {
        type: "object",
        properties: {
          pattern: { type: "string", description: "正则表达式" },
          path: { type: "string", description: "搜索目录，默认当前工作目录" },
          include: { type: "string", description: "文件通配，如 *.js" },
        },
        required: ["pattern"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "glob",
      description: "按通配模式查找文件路径。",
      parameters: {
        type: "object",
        properties: {
          pattern: { type: "string", description: "通配模式，如 **/*.js" },
          path: { type: "string", description: "搜索根目录，默认当前工作目录" },
        },
        required: ["pattern"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "fetch_url",
      description: "抓取网页内容（markdown 简化文本）。用于查文档、看接口返回等。",
      parameters: {
        type: "object",
        properties: {
          url: { type: "string", description: "完整 URL" },
        },
        required: ["url"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "web_search",
      description: "在互联网搜索关键词，返回标题、链接与摘要。适合查询最新资讯、文档、教程、解决方案等 fetch_url 不知道地址的场景。",
      parameters: {
        type: "object",
        properties: {
          query: { type: "string", description: "搜索关键词，中文英文均可" },
          max: { type: "number", description: "返回结果条数，默认 6，最大 10" },
        },
        required: ["query"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "delegate",
      description: "把子任务委托给子 agent 执行并等待返回结果。explorer=快速只读探索代码库；general=可执行多步任务（含写文件/运行命令）。适合把可独立的小任务并行拆分：一次回复中可以多次调用 delegate（每次调用都会并发运行一个子 agent），最后汇总。",
      parameters: {
        type: "object",
        properties: {
          agent: { type: "string", description: "子 agent 名称：explorer 或 general" },
          task: { type: "string", description: "交给子 agent 的任务描述" },
        },
        required: ["agent", "task"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "todo_write",
      description: "创建或重置任务清单（用于多步任务的进度可视化，界面会实时显示清单与勾选状态）。一次调用传入完整清单，会覆盖之前的清单。建议在开始多步任务前调用。",
      parameters: {
        type: "object",
        properties: {
          todos: {
            type: "array",
            items: {
              type: "object",
              properties: {
                subject: { type: "string", description: "任务描述（简短，如：实现登录接口）" },
                status: {
                  type: "string",
                  enum: ["pending", "in_progress", "completed"],
                  description: "任务状态，默认 pending",
                },
              },
              required: ["subject"],
            },
            description: "完整任务清单，将覆盖当前清单",
          },
        },
        required: ["todos"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "todo_update",
      description: "更新任务清单中某一项的状态。按列表序号（index，从 1 开始）或 subject 匹配。",
      parameters: {
        type: "object",
        properties: {
          index: { type: "number", description: "任务序号（1 开始），与 subject 二选一" },
          subject: { type: "string", description: "按任务描述匹配，与 index 二选一" },
          status: {
            type: "string",
            enum: ["pending", "in_progress", "completed"],
            description: "目标状态",
          },
          reason: { type: "string", description: "变更原因（可选，仅展示）" },
        },
        required: ["status"],
      },
    },
  },
  {
    type: "function",
    function: {
      name: "get_current_time",
      description: "获取当前系统时间和日期。用户询问时间/日期/今天是几号时使用。",
      parameters: {
        type: "object",
        properties: {
          format: { type: "string", description: "格式：iso / date / time / full，默认 full" },
        },
      },
    },
  },
  {
    type: "function",
    function: {
      name: "calc",
      description: "安全计算数学表达式（无 eval，仅支持 + - * / % 和括号）。",
      parameters: {
        type: "object",
        properties: {
          expr: { type: "string", description: "数学表达式" },
        },
        required: ["expr"],
      },
    },
  },
];

/* 简单安全计算器 */
function safeCalc(expr) {
  const s = String(expr).replace(/[^0-9+\-*/().%\s]/g, "");
  if (!/^[0-9+\-*/().%\s]+$/.test(s) || !s.trim()) return { error: "表达式不合法" };
  const fn = new Function(`return (${s})`);
  const v = fn();
  return { result: v };
}

/* HTML → 纯文本 */
function htmlToText(html) {
  return String(html)
    .replace(/<script[\s\S]*?<\/script>/gi, "")
    .replace(/<style[\s\S]*?<\/style>/gi, "")
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<\/p>/gi, "\n")
    .replace(/<[^>]+>/g, " ")
    .replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&nbsp;/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

/* 网页搜索:优先 DuckDuckGo HTML 端点(无需 key),失败则回退 Bing */
async function webSearch(query, max = 6) {
  const q = encodeURIComponent(query);
  const limit = Math.min(10, Math.max(1, parseInt(max, 10) || 6));
  const ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36";

  /* DuckDuckGo */
  try {
    const res = await fetch(`https://html.duckduckgo.com/html/?q=${q}`, {
      headers: { "User-Agent": ua },
      signal: AbortSignal.timeout(15000),
    });
    if (res.ok) {
      const html = await res.text();
      const results = [];
      const blocks = html.split(/<div class="result[^"]*">/);
      for (const b of blocks.slice(1)) {
        const titleM = b.match(/class="result__a"[^>]*>(.*?)<\/a>/);
        const urlM = b.match(/class="result__a"[^>]*href="([^"]+)"/);
        const snipM = b.match(/class="result__snippet"[^>]*>(.*?)<\/a>/) || b.match(/class="result__snippet"[^>]*>(.*?)<\/(?:a|div)>/);
        if (!titleM || !urlM) continue;
        let url = urlM[1];
        /* 跳过广告 */
        if (url.includes("ad_domain") || url.includes("/y.js?")) continue;
        /* 解码 DDG 跳转链接 */
        const uddg = url.match(/uddg=([^&]+)/);
        if (uddg) url = decodeURIComponent(uddg[1]);
        results.push({
          title: htmlToText(titleM[1]),
          url,
          snippet: htmlToText(snipM ? snipM[1] : ""),
        });
        if (results.length >= limit) break;
      }
      if (results.length) return { engine: "duckduckgo", results, count: results.length };
    }
  } catch {}

  /* Bing 回退 */
  try {
    const res = await fetch(`https://www.bing.com/search?q=${q}&count=${limit}`, {
      headers: { "User-Agent": ua },
      signal: AbortSignal.timeout(15000),
    });
    if (res.ok) {
      const html = await res.text();
      const results = [];
      const blocks = html.split(/<li class="b_algo"/);
      for (const b of blocks.slice(1)) {
        const titleM = b.match(/<h2[^>]*><a[^>]*href="([^"]+)"[^>]*>(.*?)<\/a><\/h2>/);
        const snipM = b.match(/<p[^>]*>(.*?)<\/p>/);
        if (!titleM) continue;
        results.push({
          title: htmlToText(titleM[2]),
          url: titleM[1],
          snippet: htmlToText(snipM ? snipM[1] : ""),
        });
        if (results.length >= limit) break;
      }
      if (results.length) return { engine: "bing", results, count: results.length };
    }
  } catch {}

  return { error: "搜索失败（两个引擎均不可用）" };
}

/* 工具执行器:返回可序列化结果。
 * ctx 提供与 App 状态联动的回调: { todoWrite, todoUpdate } */
export async function executeTool(name, args, cwd, ctx = {}) {
  switch (name) {
    case "bash": {
      const timeout = Math.max(5, parseInt(args.timeout || 30, 10) || 30) * 1000;
      try {
        const out = execSync(args.cmd, {
          cwd,
          encoding: "utf8",
          shell: "/bin/bash",
          timeout,
          stdio: ["pipe", "pipe", "pipe"],
        });
        return { stdout: out.slice(0, 30000), exitCode: 0 };
      } catch (e) {
        return {
          stdout: String(e.stdout || "").slice(0, 30000),
          stderr: String(e.stderr || e.message).slice(0, 10000),
          exitCode: e.status ?? 1,
        };
      }
    }
    case "read_file": {
      try {
        const p = path.resolve(cwd, args.path);
        const content = fs.readFileSync(p, "utf8");
        const lines = content.split("\n");
        const offset = Math.max(1, parseInt(args.offset || 1, 10) || 1);
        const limit = Math.min(5000, parseInt(args.limit || 500, 10) || 500);
        const slice = lines.slice(offset - 1, offset - 1 + limit);
        const out = slice.map((l, i) => `${offset + i}: ${l}`).join("\n");
        return {
          content: out,
          totalLines: lines.length,
          truncated: lines.length > offset - 1 + limit,
        };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "write_file": {
      try {
        const p = path.resolve(cwd, args.path);
        fs.mkdirSync(path.dirname(p), { recursive: true });
        fs.writeFileSync(p, String(args.content ?? ""), "utf8");
        return { ok: true, path: p, bytes: String(args.content ?? "").length };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "edit_file": {
      try {
        const p = path.resolve(cwd, args.path);
        const content = fs.readFileSync(p, "utf8");
        const oldText = String(args.old ?? "");
        if (!oldText) return { error: "old 不能为空" };
        const count = content.split(oldText).length - 1;
        if (count === 0) return { error: "未找到要替换的文本（old 不匹配）" };
        if (count > 1) return { error: `old 文本出现 ${count} 次，不唯一，请包含更多上下文` };
        const next = content.replace(oldText, String(args.new ?? ""));
        fs.writeFileSync(p, next, "utf8");
        return { ok: true, path: p, replaced: 1 };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "list_dir": {
      try {
        const p = path.resolve(cwd, args.path || ".");
        const entries = fs.readdirSync(p, { withFileTypes: true });
        return {
          path: p,
          entries: entries.slice(0, 500).map((e) => {
            let size = "";
            let isDir = e.isDirectory();
            if (e.isFile()) {
              try { size = fs.statSync(path.join(p, e.name)).size; } catch {}
            }
            return { name: e.name, type: isDir ? "dir" : "file", size };
          }),
          total: entries.length,
          truncated: entries.length > 500,
        };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "grep": {
      try {
        const root = path.resolve(cwd, args.path || ".");
        const pattern = new RegExp(args.pattern, "i");
        const include = args.include ? new RegExp(args.include.replace(/\*/g, ".*")) : null;
        const results = [];
        const walk = (dir, depth) => {
          if (depth > 6 || results.length >= 200) return;
          let entries;
          try { entries = fs.readdirSync(dir, { withFileTypes: true }); } catch { return; }
          for (const e of entries) {
            if (e.name.startsWith(".") || e.name === "node_modules" || e.name === ".git") continue;
            const full = path.join(dir, e.name);
            if (e.isDirectory()) walk(full, depth + 1);
            else if (e.isFile() && (!include || include.test(e.name))) {
              try {
                const lines = fs.readFileSync(full, "utf8").split("\n");
                for (let i = 0; i < lines.length; i++) {
                  if (pattern.test(lines[i])) {
                    results.push({ file: full, line: i + 1, text: lines[i].slice(0, 200) });
                    if (results.length >= 200) break;
                  }
                }
              } catch {}
            }
          }
        };
        walk(root, 0);
        return { matches: results, count: results.length };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "glob": {
      try {
        const root = path.resolve(cwd, args.path || ".");
        const pat = String(args.pattern || "**/*");
        const re = new RegExp(
          "^" + pat.split("/").map((s) => s.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*\*/g, ".*").replace(/\*/g, "[^/]*")).join("/") + "$"
        );
        const results = [];
        const walk = (dir, depth) => {
          if (depth > 6 || results.length >= 500) return;
          let entries;
          try { entries = fs.readdirSync(dir, { withFileTypes: true }); } catch { return; }
          for (const e of entries) {
            if (e.name.startsWith(".") || e.name === "node_modules" || e.name === ".git") continue;
            const full = path.join(dir, e.name);
            const rel = path.relative(root, full);
            if (e.isDirectory()) walk(full, depth + 1);
            else if (re.test(rel)) results.push(rel);
          }
        };
        walk(root, 0);
        return { files: results, count: results.length };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "fetch_url": {
      try {
        const res = await fetch(args.url, {
          headers: { "User-Agent": "ux-agent/1.0" },
          signal: AbortSignal.timeout(20000),
        });
        const text = await res.text();
        return {
          status: res.status,
          contentType: res.headers.get("content-type") || "",
          body: htmlToText(text).slice(0, 20000),
        };
      } catch (e) {
        return { error: e.message };
      }
    }
    case "web_search":
      return webSearch(args.query, args.max);
    case "delegate":
      /* delegate 由 App 层实现(需要访问子会话状态),这里仅占位提示 */
      return { error: "内部工具: delegate 应由 agent 运行时处理" };
    case "todo_write": {
      if (typeof ctx.todoWrite !== "function") return { error: "todo_write 不可用" };
      const list = Array.isArray(args.todos) ? args.todos : [];
      return ctx.todoWrite(
        list.map((t, i) => ({
          id: i + 1,
          subject: String(t.subject || "").slice(0, 120),
          status: ["pending", "in_progress", "completed"].includes(t.status) ? t.status : "pending",
        }))
      );
    }
    case "todo_update": {
      if (typeof ctx.todoUpdate !== "function") return { error: "todo_update 不可用" };
      const status = ["pending", "in_progress", "completed"].includes(args.status) ? args.status : null;
      if (!status) return { error: `未知状态: ${args.status}` };
      return ctx.todoUpdate({
        index: parseInt(args.index, 10) || 0,
        subject: args.subject ? String(args.subject) : null,
        status,
      });
    }
    case "get_current_time": {
      const d = new Date();
      const p = (n) => String(n).padStart(2, "0");
      const full = `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
      const f = args.format || "full";
      return {
        full,
        iso: d.toISOString(),
        date: `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`,
        time: `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`,
        weekday: ["日", "一", "二", "三", "四", "五", "六"][d.getDay()],
        format: f,
      };
    }
    case "calc":
      return safeCalc(args.expr);
    default:
      return { error: `未知工具: ${name}` };
  }
}
