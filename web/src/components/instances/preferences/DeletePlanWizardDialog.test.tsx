/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render } from "@testing-library/react"
import { TooltipProvider } from "@/components/ui/tooltip"
import { APIError } from "@/lib/api"
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
const { createMutation, runQuery, cancelMutation } = vi.hoisted(() => ({
  createMutation: { isPending: false, mutate: vi.fn() },
  runQuery: { data: undefined as unknown, isLoading: false, isError: false },
  cancelMutation: { isPending: false, mutate: vi.fn() },
}))

vi.mock("@/hooks/useDeletePlan", () => ({
  useCreateDeletePlan: () => createMutation,
  useDeletePlanRun: () => runQuery,
  useCancelDeletePlanRun: () => cancelMutation,
}))

import { DeletePlanWizardDialog } from "@/components/instances/preferences/DeletePlanWizardDialog"
import "@/i18n"

const previewRun: DeletePlanRun = {
  id: 7,
  instanceId: 3,
  status: "preview_ready",
  targetBytes: 10_995_116_277_760,
  keys: [{ field: "GROUP_SIZE", enabled: true, direction: "ASC" }],
  removeDeadGroups: true,
  waitingForSync: false,
  plannedBytes: 11_045_000_000_000,
  deadBytes: 0,
  overshootBytes: 49_837_722_304,
  targetReached: true,
  removedBytes: 0,
  errorMessage: "",
  createdAt: "2026-10-14T03:00:00Z",
  updatedAt: "2026-10-14T03:00:04Z",
  completedAt: null,
  groups: [
    {
      id: 91,
      runId: 7,
      contentPath: "/data/PackDir",
      sizeBytes: 5_000_000_000,
      memberCount: 2,
      dead: false,
      included: true,
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
      excluded: true,
      excludeReason: "partial_overlap",
      decidingKey: "",
      members: [],
    },
  ],
}

beforeEach(() => {
  runQuery.data = previewRun
  createMutation.mutate.mockReset()
  // Radix dialog and select measure through ResizeObserver, which jsdom lacks.
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

function renderWizard(props: { initialRunId?: number } = {}) {
  return render(
    <TooltipProvider>
      <DeletePlanWizardDialog
        open
        onOpenChange={() => {}}
        instanceId={3}
        instanceName="seedbox"
        initialRunId={props.initialRunId}
      />
    </TooltipProvider>
  )
}

describe("DeletePlanWizardDialog", () => {
  it("shows the instance and the target size field on the first step", () => {
    const { getByRole } = renderWizard()

    expect(document.body.textContent).toContain("seedbox")
    expect(document.body.textContent).toContain("preferences.deletePlanWizard.targetLabel")
    expect(getByRole("button", { name: "preferences.deletePlanWizard.buildPlan" })).toBeDefined()
  })

  it("names the target, the planned reclaim and the overshoot on the preview", () => {
    renderWizard({ initialRunId: 7 })

    // Each label sits next to its formatted value, so the pair proves the wiring.
    const text = document.body.textContent
    expect(text).toContain(`preferences.deletePlanPreview.target${formatBytes(previewRun.targetBytes)}`)
    expect(text).toContain(`preferences.deletePlanPreview.planned${formatBytes(previewRun.plannedBytes)}`)
    expect(text).toContain(`preferences.deletePlanPreview.overshoot${formatBytes(previewRun.overshootBytes)}`)
  })

  it("says the target cannot be reached when the plan includes everything", () => {
    runQuery.data = { ...previewRun, targetReached: false }
    renderWizard({ initialRunId: 7 })

    expect(document.body.textContent).toContain("preferences.deletePlanPreview.targetNotReached")
  })

  it("renders each excluded group with its translated reason, never the raw backend value", () => {
    renderWizard({ initialRunId: 7 })

    // The reason cell holds the i18n key, not the backend's excludeReason string.
    const excludedTable = [...document.body.querySelectorAll("table")].at(-1)!
    const reasonCell = excludedTable.querySelector("tbody tr td:nth-child(3)")!
    expect(reasonCell.textContent).toBe("preferences.deletePlan.excludeReasons.partial_overlap")
  })

  it("expands a group row to its members", () => {
    const { getByRole } = renderWizard({ initialRunId: 7 })

    expect(document.body.textContent).not.toContain("Pack.2024")
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanPreview.membersColumn" }))
    expect(document.body.textContent).toContain("Pack.2024")
    expect(document.body.textContent).toContain("stateLabels.uploading")
  })

  it("shows a waiting state while the run waits for a fresh sync", () => {
    runQuery.data = {
      ...previewRun,
      status: "pending",
      waitingForSync: true,
      plannedBytes: 0,
      overshootBytes: 0,
      groups: [],
      excludedGroups: [],
    }
    renderWizard({ initialRunId: 7 })

    const text = document.body.textContent
    expect(text).toContain("preferences.deletePlanWizard.waitingTitle")
    expect(text).toContain("preferences.deletePlanWizard.waitingDescription")
    expect(text).not.toContain("preferences.deletePlanPreview.planned")
  })

  it("shows the conflict message when create answers 409", () => {
    createMutation.mutate.mockImplementation((_input, opts: { onError: (e: unknown) => void }) => {
      opts.onError(new APIError("a delete plan is already active for this instance", 409))
    })
    const { getByRole } = renderWizard()

    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.buildPlan" }))

    expect(document.body.textContent).toContain("preferences.deletePlanWizard.conflict")
  })

  it("keeps the confirm action disabled and states that other instances are not checked", () => {
    const { getByRole } = renderWizard({ initialRunId: 7 })

    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.reviewAndConfirm" }))

    const confirm = getByRole("button", { name: "preferences.deletePlanConfirm.confirm" })
    expect(confirm.hasAttribute("disabled")).toBe(true)
    expect(document.body.textContent).toContain("preferences.deletePlanConfirm.otherInstances")
  })
})
