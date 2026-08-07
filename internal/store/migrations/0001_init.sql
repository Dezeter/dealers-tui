-- Schema v1 — TZ §8. Commit-reveal persistence is the load-bearing part:
-- pending_actions must survive a process restart so no reveal window is missed.

CREATE TABLE dealers (
  token_id          INTEGER PRIMARY KEY,
  nickname          TEXT,
  wallet_address    TEXT NOT NULL,
  network           TEXT NOT NULL DEFAULT 'mainnet',
  is_active         INTEGER NOT NULL DEFAULT 1,
  last_synced_block INTEGER,
  last_synced_at    TEXT,
  cached_state_json TEXT,
  created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE pending_actions (
  seq             INTEGER PRIMARY KEY,           -- from the *Committed event
  token_id        INTEGER NOT NULL REFERENCES dealers(token_id),
  kind            TEXT NOT NULL,                 -- pve|pvp|heist_stage|breakout|wanted_poster
  commit_block    INTEGER NOT NULL,
  reveal_block    INTEGER NOT NULL,              -- commit_block + REVEAL_OFFSET
  expiry_block    INTEGER NOT NULL,              -- commit_block + EXPIRY_WINDOW
  status          TEXT NOT NULL DEFAULT 'COMMITTED', -- COMMITTED|RESOLVED|EXPIRED|FAILED
  tx_hash_commit  TEXT NOT NULL,
  tx_hash_resolve TEXT,
  meta_json       TEXT,                          -- drugId/amount/choice/heistId/stage…
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  resolved_at     TEXT
);

-- Hot query for the resolve worker: "which committed actions are due?"
CREATE INDEX idx_pending_status_reveal
  ON pending_actions (status, reveal_block);

CREATE TABLE heists (
  heist_id      INTEGER PRIMARY KEY,
  token_id      INTEGER NOT NULL,
  family        TEXT NOT NULL,   -- SUPPLY|CASH
  difficulty    TEXT NOT NULL,   -- easy|medium|hard
  stake         INTEGER NOT NULL,
  eth_addon     INTEGER NOT NULL DEFAULT 0,
  status        TEXT NOT NULL,   -- mirrors HeistStatus enum
  current_stage INTEGER NOT NULL DEFAULT 0,
  current_pot   INTEGER,
  started_at    TEXT,
  updated_at    TEXT
);

CREATE TABLE action_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  token_id   INTEGER NOT NULL,
  ts         TEXT NOT NULL DEFAULT (datetime('now')),
  kind       TEXT NOT NULL,
  summary    TEXT,
  cash_delta INTEGER,
  rep_delta  INTEGER,
  heat_after INTEGER,
  tx_hash    TEXT
);

CREATE INDEX idx_action_log_token_ts ON action_log (token_id, ts);

CREATE TABLE area_price_cache (
  area_id          INTEGER NOT NULL,
  drug_id          INTEGER NOT NULL,
  price            INTEGER,
  fetched_at_block INTEGER,
  PRIMARY KEY (area_id, drug_id)
);
