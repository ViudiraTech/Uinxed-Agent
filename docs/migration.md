# Node/Ink → Go migration

## Upgrade invariants

The migration treats existing user-visible behavior and user data as compatibility contracts. Rewriting in Go is not permission to discard sessions, providers, keys, tools, skills, Todo state, reasoning display, context compaction or multi-agent behavior.

## Legacy sources

The upgrader understands:

1. `~/.config/ux-agent/config.json` containing `history`, `conversation`, `sessions` and `activeSessionId` from the Node application.
2. The previous `better-sqlite3` `sessions` table with JSON `history` / `conversation` columns.
3. Legacy provider configuration, model selection, plaintext API key fields and AES-GCM encrypted key fields.

## Config migration

When legacy config sessions are present:

1. Parse all sessions without mutating the file.
2. Copy the exact config file to `config.pre-go-migration.<UTC timestamp>.json`.
3. Import each session into the new SQLite schema.
4. Re-load every imported session and verify message counts.
5. Only after complete verification, set `storage=db` and keep the active session ID.
6. The successful config rewrite removes stale legacy session arrays, preventing a future boot from overwriting newer database data.

If any import or verification step fails, the old config remains the source of truth and the timestamped backup is retained.

## SQLite schema migration

When a `sessions` table has the legacy JSON columns but not the v2 normalized columns:

1. Create a timestamped database backup before schema changes.
2. Begin a transaction.
3. Read legacy rows.
4. Rename the old table to `legacy_sessions_v1`.
5. Create v2 normalized tables and indexes.
6. Convert legacy messages into normalized rows.
7. Commit only if every row succeeds.

A rollback leaves the pre-migration database available for recovery.

## Secrets

Existing plaintext API keys can still be read for migration/compatibility. When a key is written by the Go version it is stored as AES-256-GCM ciphertext. UI and logs use redacted key forms.

## Storage switching

`/storage db` migrates config sessions to SQLite. `/storage config` writes sessions into the compatibility representation. SQLite remains the recommended production mode.

## Removed runtime dependency

After migration, the application does not require Node.js, npm, React, Ink, `better-sqlite3`, esbuild or `node_modules`. The Go build produces the `ux-agent` binary directly.
