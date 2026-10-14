-- Copyright (c) 2025-2026, s0up and the autobrr contributors.
-- SPDX-License-Identifier: GPL-2.0-or-later

-- A delete plan run: one target size, one key chain, and one frozen set of rows.
CREATE TABLE IF NOT EXISTS delete_plan_runs (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id        INTEGER NOT NULL,
    status             TEXT NOT NULL,
    target_bytes       INTEGER NOT NULL,
    keys_json          TEXT NOT NULL DEFAULT '[]',
    remove_dead_groups INTEGER NOT NULL DEFAULT 1,
    planned_bytes      INTEGER NOT NULL DEFAULT 0,
    dead_bytes         INTEGER NOT NULL DEFAULT 0,
    overshoot_bytes    INTEGER NOT NULL DEFAULT 0,
    target_reached     INTEGER NOT NULL DEFAULT 0,
    removed_bytes      INTEGER NOT NULL DEFAULT 0,
    error_message      TEXT,
    created_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at       DATETIME,
    FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_delete_plan_runs_instance_created
    ON delete_plan_runs(instance_id, created_at DESC);

-- One frozen content group per row, in plan order.
CREATE TABLE IF NOT EXISTS delete_plan_rows (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id         INTEGER NOT NULL,
    content_path   TEXT NOT NULL,
    size_bytes     INTEGER NOT NULL,
    member_count   INTEGER NOT NULL,
    dead           INTEGER NOT NULL DEFAULT 0,
    included       INTEGER NOT NULL DEFAULT 1,
    excluded       INTEGER NOT NULL DEFAULT 0,
    exclude_reason TEXT NOT NULL DEFAULT '',
    deciding_key   TEXT NOT NULL DEFAULT '',
    members        TEXT NOT NULL DEFAULT '[]',
    FOREIGN KEY (run_id) REFERENCES delete_plan_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_delete_plan_rows_run ON delete_plan_rows(run_id);
