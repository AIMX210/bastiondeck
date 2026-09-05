-- BastionDeck initial schema (migration 0001).
-- All timestamps are RFC3339 millisecond strings in UTC.

PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS schema_migrations(
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users(
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL CHECK(role IN ('owner','admin','operator','viewer')),
  password_hash TEXT NOT NULL,
  totp_secret_enc BLOB,
  totp_enabled INTEGER NOT NULL DEFAULT 0,
  disabled INTEGER NOT NULL DEFAULT 0,
  must_change_password INTEGER NOT NULL DEFAULT 0,
  last_login_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions(
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  user_agent TEXT,
  ip TEXT,
  revoked INTEGER NOT NULL DEFAULT 0,
  revoke_reason TEXT
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS login_attempts(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL,
  ip TEXT NOT NULL,
  ok INTEGER NOT NULL,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_login_attempts_at ON login_attempts(at);

CREATE TABLE IF NOT EXISTS credentials(
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('password','private_key')),
  ciphertext BLOB NOT NULL,
  fingerprint TEXT,
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS host_groups(
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  parent_id TEXT REFERENCES host_groups(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS hosts(
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  address TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  username TEXT NOT NULL,
  credential_id TEXT REFERENCES credentials(id) ON DELETE SET NULL,
  auth_kind TEXT NOT NULL DEFAULT 'credential' CHECK(auth_kind IN ('credential','agent')),
  agent_id TEXT,
  jump_host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
  group_id TEXT REFERENCES host_groups(id) ON DELETE SET NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  notes TEXT NOT NULL DEFAULT '',
  favorite INTEGER NOT NULL DEFAULT 0,
  known_host_key TEXT,
  known_host_key_type TEXT,
  first_seen_at TEXT,
  last_connected_at TEXT,
  last_status TEXT NOT NULL DEFAULT 'unknown',
  last_status_at TEXT,
  options_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hosts_group ON hosts(group_id);
CREATE INDEX IF NOT EXISTS idx_hosts_jump ON hosts(jump_host_id);

CREATE TABLE IF NOT EXISTS host_recents(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  at TEXT NOT NULL,
  kind TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_recents_user ON host_recents(user_id, at);

CREATE TABLE IF NOT EXISTS snippets(
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs(
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK(kind IN ('oneshot','scheduled','adhoc')),
  name TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL,
  target_ids_json TEXT NOT NULL,
  snippet_id TEXT REFERENCES snippets(id) ON DELETE SET NULL,
  schedule_expr TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  timeout_ms INTEGER NOT NULL DEFAULT 60000,
  concurrency INTEGER NOT NULL DEFAULT 5,
  created_by TEXT REFERENCES users(id),
  last_run_at TEXT,
  next_run_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS job_runs(
  id TEXT PRIMARY KEY,
  job_id TEXT REFERENCES jobs(id) ON DELETE CASCADE,
  trigger TEXT NOT NULL CHECK(trigger IN ('manual','schedule','retry')),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK(status IN ('pending','running','success','failed','timeout','cancelled','lost')),
  started_at TEXT,
  ended_at TEXT,
  summary_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_status ON job_runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_created ON job_runs(created_at);

CREATE TABLE IF NOT EXISTS run_targets(
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
  host_id TEXT NOT NULL REFERENCES hosts(id),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK(status IN ('pending','running','success','failed','timeout','cancelled','lost','skipped')),
  exit_code INTEGER,
  started_at TEXT,
  ended_at TEXT,
  error_code TEXT,
  error_text TEXT,
  stdout_path TEXT,
  stderr_path TEXT,
  stdout_preview TEXT DEFAULT '',
  stderr_preview TEXT DEFAULT '',
  bytes_out INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_run_targets_run ON run_targets(run_id);
CREATE INDEX IF NOT EXISTS idx_run_targets_status ON run_targets(status);

CREATE TABLE IF NOT EXISTS term_sessions(
  id TEXT PRIMARY KEY,
  host_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  started_at TEXT,
  ended_at TEXT,
  cols INTEGER,
  rows INTEGER
);

CREATE TABLE IF NOT EXISTS tunnels(
  id TEXT PRIMARY KEY,
  host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('local','remote')),
  local_host TEXT NOT NULL DEFAULT '127.0.0.1',
  local_port INTEGER NOT NULL,
  remote_host TEXT NOT NULL,
  remote_port INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'starting',
  started_at TEXT,
  stopped_at TEXT,
  started_by TEXT REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS metric_points(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  at TEXT NOT NULL,
  kind TEXT NOT NULL,
  value REAL NOT NULL,
  extra_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_metric_host_time ON metric_points(host_id, at);

CREATE TABLE IF NOT EXISTS audit_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id TEXT NOT NULL UNIQUE,
  at TEXT NOT NULL,
  actor_id TEXT,
  actor_name TEXT,
  action TEXT NOT NULL,
  object_type TEXT,
  object_id TEXT,
  result TEXT NOT NULL,
  detail_json TEXT NOT NULL DEFAULT '{}',
  prev_hash TEXT NOT NULL DEFAULT '',
  hash TEXT NOT NULL,
  ip TEXT
);
CREATE INDEX IF NOT EXISTS idx_audit_at ON audit_logs(at);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);

CREATE TABLE IF NOT EXISTS agents(
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  enroll_secret_hash TEXT NOT NULL,
  registered_at TEXT,
  last_seen_at TEXT,
  version TEXT,
  facts_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK(status IN ('pending','approved','blocked'))
);

CREATE TABLE IF NOT EXISTS settings(
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
