-- Copyright (c) 2025-2026, s0up and the autobrr contributors.
-- SPDX-License-Identifier: GPL-2.0-or-later

-- Ticket #9: a run carries the failure summary of a partial deletion separately
-- from a hard failure, and each row carries its removal outcome.
ALTER TABLE delete_plan_runs ADD COLUMN warning_message TEXT NOT NULL DEFAULT '';

ALTER TABLE delete_plan_rows ADD COLUMN row_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE delete_plan_rows ADD COLUMN row_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE delete_plan_rows ADD COLUMN row_error TEXT NOT NULL DEFAULT '';
