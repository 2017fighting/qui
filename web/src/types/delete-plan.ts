/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import type { KeyRule } from "./automation"

// Delete plan types. The wire shape is frozen in the delete plan API contract;
// the backend that serves it lands alongside this frontend ticket.
export type DeletePlanRunStatus =
  | "pending"
  | "planning"
  | "preview_ready"
  | "deleting"
  | "completed"
  | "failed"
  | "canceled"

// The hard protections a group can be dropped by, plus the row-level
// `members_changed` the deletion re-check records. `excludeReason` is "" when the
// group is not excluded.
export type DeletePlanExcludeReason =
  | "incomplete"
  | "no_delete_tag"
  | "partial_overlap"
  | "unreadable_files"
  | "nested_path"

// A row's removal outcome. Pending until the deletion reaches it.
export type DeletePlanRowStatus = "pending" | "deleted" | "skipped" | "failed"

export interface DeletePlanMember {
  hash: string
  name: string
  sizeBytes: number
  progress: number
  state: string
  tags: string[]
  savePath: string
}

export interface DeletePlanRow {
  id: number
  runId: number
  contentPath: string
  // The largest member's size: a content group's bytes count once.
  sizeBytes: number
  memberCount: number
  dead: boolean
  included: boolean
  excluded: boolean
  excludeReason: DeletePlanExcludeReason | ""
  decidingKey: string
  members: DeletePlanMember[]
  rowStatus: DeletePlanRowStatus
  rowReason: string
  rowError: string
}

export interface DeletePlanRunSummary {
  id: number
  instanceId: number
  status: DeletePlanRunStatus
  targetBytes: number
  keys: KeyRule[]
  removeDeadGroups: boolean
  waitingForSync: boolean
  plannedBytes: number
  deadBytes: number
  overshootBytes: number
  targetReached: boolean
  removedBytes: number
  errorMessage: string
  // Failure summary of a completed run that lost some chunks. Empty otherwise.
  warningMessage: string
  createdAt: string
  updatedAt: string
  completedAt: string | null
}

export interface DeletePlanRun extends DeletePlanRunSummary {
  groups: DeletePlanRow[]
  excludedGroups: DeletePlanRow[]
}

export interface DeletePlanCreateInput {
  targetBytes: number
  // Omitted means the server uses its default key chain.
  keys?: KeyRule[]
  removeDeadGroups?: boolean
}
