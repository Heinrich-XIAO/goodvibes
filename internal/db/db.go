// SPDX-License-Identifier: AGPL-3.0-or-later
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open initializes the GoodVibes (Hackatime Go port) database in WAL mode.
// Mirrors db/schema.rb 1-to-1 while adding GoodVibes vibecoding columns
// (agent_seconds, struggle_score, harness, model, evidence_hash, audit_submissions).
func Open(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		dbPath = "data/goodvibes.db"
	}
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir db dir: %w", err)
		}
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1) // SQLite WAL single-writer safety

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}

	if _, err := conn.Exec(schemaSQL); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return conn, nil
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  hackclub_id TEXT UNIQUE,
  slack_uid TEXT UNIQUE,
  username TEXT UNIQUE,
  display_name TEXT DEFAULT '',
  slack_username TEXT DEFAULT '',
  slack_avatar_url TEXT DEFAULT '',
  uses_slack_status INTEGER NOT NULL DEFAULT 0,
  slack_access_token TEXT DEFAULT '',
  hackatime_extension_text_type INTEGER NOT NULL DEFAULT 0,
  timezone TEXT NOT NULL DEFAULT 'UTC',
  github_uid TEXT DEFAULT '',
  github_avatar_url TEXT DEFAULT '',
  github_access_token TEXT DEFAULT '',
  github_username TEXT DEFAULT '',
  slack_neighborhood_channel TEXT DEFAULT '',
  trust_level INTEGER NOT NULL DEFAULT 0,
  country_code TEXT DEFAULT '',
  verification_status TEXT DEFAULT '',
  ysws_eligible INTEGER NOT NULL DEFAULT 0,
  bio TEXT DEFAULT '',
  allow_public_stats_lookup INTEGER NOT NULL DEFAULT 1,
  default_timezone_leaderboard INTEGER NOT NULL DEFAULT 1,
  admin_level INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  impersonater_user_id INTEGER,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS email_addresses (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL DEFAULT 'default',
  token TEXT NOT NULL UNIQUE,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS admin_api_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token TEXT NOT NULL UNIQUE,
  revoked_at DATETIME,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS raw_heartbeat_uploads (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_headers TEXT NOT NULL DEFAULT '{}',
  request_body TEXT NOT NULL DEFAULT '{}',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS heartbeats (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  branch TEXT DEFAULT '',
  category TEXT NOT NULL DEFAULT 'coding',
  dependencies TEXT DEFAULT '[]',
  editor TEXT DEFAULT '',
  entity TEXT DEFAULT '',
  language TEXT DEFAULT '',
  machine TEXT DEFAULT '',
  operating_system TEXT DEFAULT '',
  project TEXT DEFAULT '',
  type TEXT DEFAULT 'file',
  user_agent TEXT DEFAULT '',
  line_additions INTEGER DEFAULT 0,
  line_deletions INTEGER DEFAULT 0,
  lineno INTEGER DEFAULT 0,
  lines INTEGER DEFAULT 0,
  cursorpos INTEGER DEFAULT 0,
  project_root_count INTEGER DEFAULT 0,
  time REAL NOT NULL,
  is_write INTEGER DEFAULT 0,
  fields_hash TEXT,
  source_type INTEGER NOT NULL DEFAULT 0,
  ip_address TEXT DEFAULT '',
  ysws_program INTEGER NOT NULL DEFAULT 0,
  deleted_at DATETIME,
  raw_data TEXT DEFAULT '{}',
  raw_heartbeat_upload_id INTEGER REFERENCES raw_heartbeat_uploads(id),
  -- GoodVibes vibecoding productive-struggle extensions:
  human_seconds INTEGER NOT NULL DEFAULT 0,
  agent_seconds INTEGER NOT NULL DEFAULT 0,
  harness TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  prompt_word_count INTEGER NOT NULL DEFAULT 0,
  prompt_tech_terms INTEGER NOT NULL DEFAULT 0,
  keystrokes INTEGER NOT NULL DEFAULT 0,
  struggle_score INTEGER NOT NULL DEFAULT 0,
  flags_json TEXT NOT NULL DEFAULT '[]',
  evidence_hash TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_hb_user_time ON heartbeats(user_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_hb_user_project_time ON heartbeats(user_id, project, time);
CREATE INDEX IF NOT EXISTS idx_hb_user_category_time ON heartbeats(user_id, category, time);
CREATE INDEX IF NOT EXISTS idx_hb_evidence ON heartbeats(user_id, evidence_hash);
CREATE UNIQUE INDEX IF NOT EXISTS idx_hb_fields_hash_active ON heartbeats(fields_hash) WHERE deleted_at IS NULL AND fields_hash IS NOT NULL AND fields_hash != '';

CREATE TABLE IF NOT EXISTS repositories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  url TEXT UNIQUE,
  host TEXT DEFAULT '',
  owner TEXT DEFAULT '',
  name TEXT DEFAULT '',
  stars INTEGER DEFAULT 0,
  description TEXT DEFAULT '',
  language TEXT DEFAULT '',
  languages TEXT DEFAULT '',
  commit_count INTEGER DEFAULT 0,
  last_commit_at DATETIME,
  last_synced_at DATETIME,
  homepage TEXT DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS project_repo_mappings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project_name TEXT NOT NULL,
  repo_url TEXT NOT NULL,
  repository_id INTEGER REFERENCES repositories(id),
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, project_name)
);

CREATE TABLE IF NOT EXISTS commits (
  sha TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  repository_id INTEGER REFERENCES repositories(id),
  github_raw TEXT DEFAULT '{}',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS leaderboards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  start_date TEXT NOT NULL,
  finished_generating_at DATETIME,
  deleted_at DATETIME,
  period_type INTEGER NOT NULL DEFAULT 0,
  timezone_utc_offset INTEGER DEFAULT 0,
  timezone_offset INTEGER DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS leaderboard_entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  leaderboard_id INTEGER NOT NULL REFERENCES leaderboards(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  total_seconds INTEGER NOT NULL DEFAULT 0,
  rank INTEGER DEFAULT 0,
  streak_count INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(leaderboard_id, user_id)
);

CREATE TABLE IF NOT EXISTS trust_level_audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  changed_by_id INTEGER NOT NULL REFERENCES users(id),
  previous_trust_level TEXT NOT NULL,
  new_trust_level TEXT NOT NULL,
  reason TEXT DEFAULT '',
  notes TEXT DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_submissions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project_name TEXT NOT NULL,
  program TEXT NOT NULL DEFAULT 'YSWS',
  status TEXT NOT NULL DEFAULT 'pending',
  claimed_human_seconds INTEGER NOT NULL DEFAULT 0,
  approved_human_seconds INTEGER,
  excluded_agent_seconds INTEGER NOT NULL DEFAULT 0,
  avg_struggle_score REAL NOT NULL DEFAULT 0,
  chain_valid INTEGER NOT NULL DEFAULT 0,
  matched_heartbeats INTEGER NOT NULL DEFAULT 0,
  total_bundle_entries INTEGER NOT NULL DEFAULT 0,
  trust_score INTEGER NOT NULL DEFAULT 0,
  notes TEXT DEFAULT '',
  reviewer_notes TEXT DEFAULT '',
  reviewed_by INTEGER REFERENCES users(id),
  bundle_json TEXT NOT NULL,
  verification_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  reviewed_at INTEGER
);
`
