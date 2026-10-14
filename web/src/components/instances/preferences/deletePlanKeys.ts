/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { CONDITION_FIELDS } from "@/components/query-builder/constants"
import type { ConditionField, KeyRule } from "@/types"

// The chain a plan ranks with when the wizard is untouched. Mirrors
// internal/services/deleteplan DefaultKeys: member count ascending, seeders
// descending, group bytes ascending, non-public before private.
export const DEFAULT_DELETE_PLAN_KEYS: KeyRule[] = [
  { field: "GROUP_SIZE", enabled: true, direction: "ASC" },
  { field: "NUM_COMPLETE", enabled: true, direction: "DESC" },
  { field: "SIZE", enabled: true, direction: "ASC" },
  { field: "PRIVATE", enabled: true, direction: "ASC" },
]

// The fields the backend accepts as a ranking key (models.ConditionField
// .IsSortableKey): the numeric and string fields the sorter compares directly,
// plus GROUP_SIZE (member count) and PRIVATE, which the plan reads as 0 or 1.
const SORTABLE_KEY_FIELDS = new Set<ConditionField>([
  "SIZE",
  "TOTAL_SIZE",
  "DOWNLOADED",
  "UPLOADED",
  "AMOUNT_LEFT",
  "FREE_SPACE",
  "ADDED_ON",
  "COMPLETION_ON",
  "LAST_ACTIVITY",
  "SEEDING_TIME",
  "TIME_ACTIVE",
  "ADDED_ON_AGE",
  "COMPLETION_ON_AGE",
  "LAST_ACTIVITY_AGE",
  "RATIO",
  "PROGRESS",
  "AVAILABILITY",
  "DL_SPEED",
  "UP_SPEED",
  "NUM_SEEDS",
  "NUM_LEECHS",
  "NUM_COMPLETE",
  "NUM_INCOMPLETE",
  "TRACKERS_COUNT",
  "SYSTEM_HOUR",
  "SYSTEM_MINUTE",
  "SYSTEM_DAY_OF_WEEK",
  "SYSTEM_DAY",
  "SYSTEM_MONTH",
  "SYSTEM_YEAR",
  "GROUP_SIZE",
  "PRIVATE",
  "NAME",
  "CATEGORY",
  "TAGS",
  "TRACKER",
  "STATE",
  "SAVE_PATH",
  "CONTENT_PATH",
  "COMMENT",
])

// The picker disables every field it is not told to allow, so this is the complement.
export const NON_SORTABLE_KEY_FIELDS = Object.keys(CONDITION_FIELDS).filter(
  (field) => !SORTABLE_KEY_FIELDS.has(field as ConditionField)
)

// moveKey reorders one key by delta and leaves the chain untouched at either end.
// Pure, so the wizard's up/down buttons are tested without a drag.
export function moveKey<T>(keys: T[], index: number, delta: number): T[] {
  const target = index + delta
  if (target < 0 || target >= keys.length) {
    return keys
  }
  const next = [...keys]
  const [moved] = next.splice(index, 1)
  next.splice(target, 0, moved)
  return next
}
