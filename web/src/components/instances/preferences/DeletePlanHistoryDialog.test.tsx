/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render } from "@testing-library/react"
import { TooltipProvider } from "@/components/ui/tooltip"
import { formatBytes } from "@/lib/utils"
import type { DeletePlanRun } from "@/types"

vi.mock("react-i18next", async (importOriginal) => ({
  ...await importOriginal<typeof import("react-i18next")>(),
  // Key echo: the assertion names the key the UI picked, and the array form
  // used for a computed reason key resolves to its first element.
  useTranslation: () => ({
    t: (key: string | string[], opts?: Record<string, unknown>) => {
      const resolved = Array.isArray(key) ? key[0] : key
      return opts ? `${resolved}(${Object.values(opts).join(",")})` : resolved
    },
  }),
}))

// Stable singletons: a fresh object per render would rerun effects forever.
const { runQuery } = vi.hoisted(() => ({
  runQuery: { data: undefined as unknown, isLoading: false, isError: false },
}))

vi.mock("@/hooks/useDeletePlan", () => ({
  useDeletePlanRun: () => runQuery,
}))

import { DeletePlanHistoryDialog } from "@/components/instances/preferences/DeletePlanHistoryDialog"
import "@/i18n"

const completedRun: DeletePlanRun = {
  id: 7,
  instanceId: 3,
  status: "completed",
  targetBytes: 10_995_116_277_760,
  keys: [
    { field: "GROUP_SIZE", enabled: true, direction: "ASC" },
    { field: "SIZE", enabled: false, direction: "ASC" },
  ],
  removeDeadGroups: false,
  waitingForSync: false,
  plannedBytes: 11_045_000_000_000,
  deadBytes: 0,
  overshootBytes: 49_837_722_304,
  shortfallBytes: 0,
  targetReached: true,
  removedBytes: 5_000_000_000,
  errorMessage: "",
  warningMessage: "",
  createdAt: "2026-10-14T03:00:00Z",
  updatedAt: "2026-10-14T03:05:00Z",
  completedAt: "2026-10-14T03:05:00Z",
  groups: [
    {
      id: 91,
      runId: 7,
      contentPath: "/data/PackDir",
      sizeBytes: 5_000_000_000,
      memberCount: 2,
      dead: false,
      included: true,
      unticked: false,
      excluded: false,
      excludeReason: "",
      decidingKey: "GROUP_SIZE",
      members: [
        {
          hash: "abc123",
          name: "Pack.2024",
          sizeBytes: 5_000_000_000,
          progress: 1,
          state: "uploading",
          tags: ["cross-seed"],
          savePath: "/data",
        },
      ],
      rowStatus: "deleted",
      rowReason: "",
      rowError: "",
    },
  ],
  excludedGroups: [
    {
      id: 92,
      runId: 7,
      contentPath: "/data/Partial",
      sizeBytes: 3_000_000_000,
      memberCount: 1,
      dead: false,
      included: false,
      unticked: false,
      excluded: true,
      excludeReason: "nested_path",
      decidingKey: "",
      members: [],
      rowStatus: "pending",
      rowReason: "",
      rowError: "",
    },
  ],
}

beforeEach(() => {
  runQuery.data = completedRun
  runQuery.isLoading = false
  runQuery.isError = false
  // Radix dialog measures through ResizeObserver, which jsdom lacks.
  vi.stubGlobal("ResizeObserver", class {
    observe() {}
    unobserve() {}
    disconnect() {}
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function renderHistory() {
  return render(
    <TooltipProvider>
      <DeletePlanHistoryDialog open onOpenChange={() => {}} instanceId={3} runId={7} />
    </TooltipProvider>
  )
}

describe("DeletePlanHistoryDialog", () => {
  it("shows the configuration the run used, including the switched-off keys", () => {
    renderHistory()

    const text = document.body.textContent
    expect(text).toContain("preferences.deletePlanWizard.keysTitle")
    // The enabled key keeps its field and direction, the disabled one is marked off.
    expect(text).toContain("queryBuilder.fields.GROUP_SIZE preferences.deletePlanWizard.directionAsc")
    expect(text).toContain(
      "preferences.deletePlanHistory.disabledKey(queryBuilder.fields.SIZE preferences.deletePlanWizard.directionAsc)"
    )
    expect(text).toContain("preferences.deletePlanWizard.removeDeadGroups")
    expect(text).toContain("preferences.deletePlanHistory.disabled")
  })

  it("shows the removed bytes, the planned reclaim and the target", () => {
    renderHistory()

    const text = document.body.textContent
    expect(text).toContain("preferences.deletePlanHistory.removed")
    expect(text).toContain(formatBytes(completedRun.removedBytes))
    expect(text).toContain(formatBytes(completedRun.plannedBytes))
    expect(text).toContain(formatBytes(completedRun.targetBytes))
  })

  it("renders each excluded group with its translated reason, never the raw backend value", () => {
    renderHistory()

    const excludedTable = [...document.body.querySelectorAll("table")].at(-1)!
    const reasonCell = excludedTable.querySelector("tbody tr td:nth-child(3)")!
    expect(reasonCell.textContent).toBe("preferences.deletePlan.excludeReasons.nested_path")
  })

  it("expands a group row to its members", () => {
    const { getByRole } = renderHistory()

    expect(document.body.textContent).not.toContain("Pack.2024")
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanPreview.membersColumn" }))
    expect(document.body.textContent).toContain("Pack.2024")
  })

  it("offers no mutating action", () => {
    const { queryByRole } = renderHistory()

    expect(queryByRole("button", { name: "preferences.deletePlanConfirm.confirm" })).toBeNull()
    expect(queryByRole("button", { name: "preferences.deletePlan.cancel" })).toBeNull()
    expect(queryByRole("button", { name: "preferences.deletePlanWizard.buildPlan" })).toBeNull()
    expect(queryByRole("button", { name: "preferences.deletePlanWizard.reviewAndConfirm" })).toBeNull()
    expect(queryByRole("button", { name: "preferences.deletePlanPreview.exportCSV" })).toBeNull()
    // The only control closes the read-only view.
    expect(queryByRole("button", { name: "preferences.deletePlan.close" })).not.toBeNull()
  })

  it("shows the loading state before the run arrives", () => {
    runQuery.data = undefined
    runQuery.isLoading = true
    renderHistory()

    expect(document.body.textContent).toContain("preferences.deletePlanPreview.loading")
  })

  it("shows the load error when the run cannot be read", () => {
    runQuery.data = undefined
    runQuery.isError = true
    renderHistory()

    expect(document.body.textContent).toContain("preferences.deletePlanWizard.loadFailed")
  })
})
