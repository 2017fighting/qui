import type { ActionConditions, Automation } from "@/types"

export const automationConditions: ActionConditions = { schemaVersion: "1" }

export function makeAutomation(overrides: Partial<Automation> = {}): Automation {
  return {
    id: 1,
    instanceId: 1,
    name: "My workflow",
    trackerPattern: "",
    conditions: automationConditions,
    enabled: true,
    dryRun: false,
    notify: true,
    sortOrder: 0,
    ...overrides,
  }
}
