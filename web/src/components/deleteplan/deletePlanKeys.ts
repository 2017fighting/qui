
import { CONDITION_FIELDS } from "@/components/query-builder/constants"
import type { ConditionField, KeyRule } from "@/types"

export const DEFAULT_DELETE_PLAN_KEYS: KeyRule[] = [
  { field: "GROUP_SIZE", enabled: true, direction: "ASC" },
  { field: "NUM_COMPLETE", enabled: true, direction: "DESC" },
  { field: "SIZE", enabled: true, direction: "ASC" },
  { field: "PRIVATE", enabled: true, direction: "ASC" },
]

const SORTABLE_KEY_FIELDS: ReadonlySet<string> = new Set<ConditionField>([
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

export const NON_SORTABLE_KEY_FIELDS = Object.keys(CONDITION_FIELDS).filter(
  (field) => !SORTABLE_KEY_FIELDS.has(field)
)

export function isSortableKeyField(field: string): field is ConditionField {
  return SORTABLE_KEY_FIELDS.has(field)
}

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
