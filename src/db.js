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

/* ============ SQLite 会话数据库 ============
 * 每个会话一行(sessions 表):history/conversation 以 JSON blob 存储,
 * 替代塞满 config.json 的大数组。config.json 只保留配置(提供商等)。
 * 兼容模式:未迁移前仍读写 config.json 的 history/conversation/sessions。
 */

import Database from "better-sqlite3";
import fs from "node:fs";
import path from "node:path";
import { CONFIG_DIR } from "./config.js";

export const DB_FILE = path.join(CONFIG_DIR, "ux-agent.db");

let db = null;

export function initDb() {
  if (db) return db;
  try {
    fs.mkdirSync(CONFIG_DIR, { recursive: true });
    db = new Database(DB_FILE);
    db.pragma("journal_mode = WAL");
    db.exec(`
      CREATE TABLE IF NOT EXISTS sessions (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        agent_id TEXT,
        cwd TEXT,
        updated_at INTEGER NOT NULL,
        history TEXT NOT NULL DEFAULT '[]',
        conversation TEXT NOT NULL DEFAULT '[]'
      );
    `);
  } catch (e) {
    /* 数据库打不开(权限/损坏):退回 config.json 兼容模式 */
    try { db?.close(); } catch {}
    db = null;
  }
  return db;
}

export function closeDb() {
  try { db?.close(); } catch {}
  db = null;
}

export function dbReady() {
  return !!initDb();
}

export function dbSessionCount() {
  try {
    return initDb().prepare("SELECT COUNT(*) AS n FROM sessions").get().n;
  } catch { return 0; }
}

export function dbLoadSessions() {
  try {
    const rows = initDb().prepare("SELECT * FROM sessions ORDER BY updated_at DESC").all();
    return rows.map((r) => ({
      id: r.id,
      name: r.name,
      agentId: r.agent_id || "build",
      cwd: r.cwd || null,
      updatedAt: r.updated_at,
      history: safeJson(r.history),
      conversation: safeJson(r.conversation),
    }));
  } catch { return []; }
}

export function dbSaveSession(s) {
  if (!dbReady()) return false;
  const sess = {
    id: s.id,
    name: s.name || s.id,
    agentId: s.agentId || "build",
    cwd: s.cwd || null,
    updatedAt: s.updatedAt || Date.now(),
    history: JSON.stringify(s.history || []),
    conversation: JSON.stringify(s.conversation || []),
  };
  try {
    initDb().prepare(`
      INSERT INTO sessions (id, name, agent_id, cwd, updated_at, history, conversation)
      VALUES (@id, @name, @agentId, @cwd, @updatedAt, @history, @conversation)
      ON CONFLICT(id) DO UPDATE SET
        name=@name, agent_id=@agentId, cwd=@cwd, updated_at=@updatedAt,
        history=@history, conversation=@conversation
    `).run(sess);
    return true;
  } catch { return false; }
}

export function dbDeleteSession(id) {
  if (!dbReady()) return;
  try { initDb().prepare("DELETE FROM sessions WHERE id = ?").run(id); } catch {}
}

function safeJson(s) {
  try { return JSON.parse(s || "[]"); } catch { return []; }
}
