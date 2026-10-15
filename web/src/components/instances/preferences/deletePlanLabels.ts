/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

// The delete plan card and wizard read the same status and reason key paths.
export function deletePlanStatusLabelKey(status: string): string {
  return `preferences.deletePlan.statusLabels.${status}`
}

export function deletePlanExcludeReasonKey(reason: string): string {
  return `preferences.deletePlan.excludeReasons.${reason}`
}
