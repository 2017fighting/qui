import { describe, expect, it } from "vitest"

import { makeAutomation } from "./workflowAutomation"

import { firstActionCondition } from "@/lib/workflow-utils"
import type { ActionConditions, RuleCondition } from "@/types"

describe("firstActionCondition finds the condition of any running action, so the overview never shows an All torrents badge for a rule that has one", () => {
  const condition: RuleCondition = {
    operator: "AND",
    conditions: [{ field: "STATE", operator: "EQUAL", value: "downloading" }],
  }

  it.each([
    "speedLimits",
    "shareLimits",
    "pause",
    "resume",
    "recheck",
    "reannounce",
    "autoManagement",
    "delete",
    "tag",
    "category",
    "move",
    "externalProgram",
    "exportToInstance",
    "skipSmallFiles",
  ] as const)("finds the condition on a %s-only rule", (action) => {
    expect(firstActionCondition({ ...makeAutomation(), conditions: { schemaVersion: "1", [action]: { enabled: true, condition } } as ActionConditions })).toEqual(condition)
  })

  it("finds a condition on any entry of the tags array", () => {
    expect(firstActionCondition({ ...makeAutomation(), conditions: {
      schemaVersion: "1",
      tags: [{ enabled: true, tags: ["a"], mode: "full" }, { enabled: true, tags: ["b"], mode: "full", condition }],
    } })).toEqual(condition)
  })

  it("reports no condition when every action is disabled or carries none", () => {
    expect(firstActionCondition({ ...makeAutomation(), conditions: { schemaVersion: "1" } })).toBeNull()
    expect(firstActionCondition({ ...makeAutomation(), conditions: { schemaVersion: "1", pause: { enabled: true } } })).toBeNull()
    expect(firstActionCondition({ ...makeAutomation(), conditions: { schemaVersion: "1", skipSmallFiles: { enabled: false, maxSizeBytes: 1, condition } } })).toBeNull()
  })
})
