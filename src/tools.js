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

/* 工具执行器:返回可序列化结果 */
export async function executeTool(name, args, cwd) {
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
          body: text
            .replace(/<script[\s\S]*?<\/script>/gi, "")
            .replace(/<style[\s\S]*?<\/style>/gi, "")
            .replace(/<[^>]+>/g, " ")
            .replace(/\s+/g, " ")
            .slice(0, 20000),
        };
      } catch (e) {
        return { error: e.message };
      }
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
