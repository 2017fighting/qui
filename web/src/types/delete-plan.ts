
import type { KeyRule } from "./automation"

export type DeletePlanRunStatus =
  | "pending"
  | "planning"
  | "preview_ready"
  | "deleting"
  | "completed"
  | "failed"
  | "canceled"

export type DeletePlanExcludeReason =
  | "incomplete"
  | "no_delete_tag"
  | "partial_overlap"
  | "unreadable_files"
  | "nested_path"

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

  sizeBytes: number
  memberCount: number
  dead: boolean
  included: boolean

  unticked: boolean
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

  shortfallBytes: number
  targetReached: boolean
  removedBytes: number
  errorMessage: string

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

  keys?: KeyRule[]
  removeDeadGroups?: boolean
}
