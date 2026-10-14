/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render } from "@testing-library/react"
import { TooltipProvider } from "@/components/ui/tooltip"
import { APIError } from "@/lib/api"
import { BYTES_PER_UNIT } from "@/lib/unit-format"
import { formatBytes } from "@/lib/utils"
import type { DeletePlanCreateInput, DeletePlanRun } from "@/types"

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
const { createMutation, runQuery, cancelMutation, confirmMutation } = vi.hoisted(() => ({
  createMutation: { isPending: false, mutate: vi.fn() },
  runQuery: { data: undefined as unknown, isLoading: false, isError: false },
  cancelMutation: { isPending: false, mutate: vi.fn() },
  confirmMutation: { isPending: false, mutate: vi.fn() },
}))

vi.mock("@/hooks/useDeletePlan", () => ({
  useCreateDeletePlan: () => createMutation,
  useDeletePlanRun: () => runQuery,
  useCancelDeletePlanRun: () => cancelMutation,
  useConfirmDeletePlanRun: () => confirmMutation,
}))

import { DeletePlanWizardDialog } from "@/components/instances/preferences/DeletePlanWizardDialog"
import { DEFAULT_DELETE_PLAN_KEYS, moveKey } from "@/components/instances/preferences/deletePlanKeys"
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
  warningMessage: "",
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
      rowStatus: "pending",
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
      excluded: true,
      excludeReason: "partial_overlap",
      decidingKey: "",
      members: [],
      rowStatus: "pending",
      rowReason: "",
      rowError: "",
    },
  ],
}

beforeEach(() => {
  runQuery.data = previewRun
  createMutation.mutate.mockReset()
  confirmMutation.mutate.mockReset()
  cancelMutation.mutate.mockReset()
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

describe("moveKey", () => {
  it("moves a key by one and leaves the chain untouched at either end", () => {
    const moved = moveKey(DEFAULT_DELETE_PLAN_KEYS, 0, 1)

    expect(moved.map((key) => key.field)).toEqual(["NUM_COMPLETE", "GROUP_SIZE", "SIZE", "PRIVATE"])
    expect(moveKey(DEFAULT_DELETE_PLAN_KEYS, 0, -1)).toBe(DEFAULT_DELETE_PLAN_KEYS)
    expect(moveKey(DEFAULT_DELETE_PLAN_KEYS, 3, 1)).toBe(DEFAULT_DELETE_PLAN_KEYS)
  })
})

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

  it("confirms the plan and states that other instances are not checked", () => {
    const { getByRole } = renderWizard({ initialRunId: 7 })

    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.reviewAndConfirm" }))

    expect(document.body.textContent).toContain("preferences.deletePlanConfirm.otherInstances")
    const confirm = getByRole("button", { name: "preferences.deletePlanConfirm.confirm" })
    expect(confirm.hasAttribute("disabled")).toBe(false)

    fireEvent.click(confirm)
    expect(confirmMutation.mutate).toHaveBeenCalledWith(7, expect.any(Object))
  })

  it("names the dead total on the confirm step when the plan removes dead groups", () => {
    runQuery.data = { ...previewRun, deadBytes: 2_000_000_000 }
    const { getByRole } = renderWizard({ initialRunId: 7 })

    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.reviewAndConfirm" }))

    expect(document.body.textContent).toContain("preferences.deletePlanConfirm.deadTotal(")
  })

  it("shows the run progress on the confirm step once the deletion starts", () => {
    const { getByRole, queryByRole, rerender } = renderWizard({ initialRunId: 7 })
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.reviewAndConfirm" }))

    runQuery.data = { ...previewRun, status: "deleting", plannedBytes: 1000, removedBytes: 250 }
    rerender(
      <TooltipProvider>
        <DeletePlanWizardDialog
          open
          onOpenChange={() => {}}
          instanceId={3}
          instanceName="seedbox"
          initialRunId={7}
        />
      </TooltipProvider>
    )

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.deleting")
    expect(document.body.textContent).toContain("preferences.deletePlanOverview.progress(")
    // The confirm action is gone once the deletion is running.
    expect(queryByRole("button", { name: "preferences.deletePlanConfirm.confirm" })).toBeNull()
  })

  it("sends the chain in the order the wizard shows", () => {
    const { getAllByRole, getByRole } = renderWizard()

    // The first key moves down one place, so the build must carry the new order.
    fireEvent.click(getAllByRole("button", { name: "preferences.deletePlanWizard.moveKeyDown" })[0])
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.buildPlan" }))

    const payload = createMutation.mutate.mock.calls[0][0] as DeletePlanCreateInput
    expect(payload.keys?.map((key) => key.field)).toEqual(["NUM_COMPLETE", "GROUP_SIZE", "SIZE", "PRIVATE"])
    expect(payload.removeDeadGroups).toBe(true)
    expect(payload.targetBytes).toBe(10 * BYTES_PER_UNIT.TiB)
  })

  it("sends a switched-off key as disabled so it cannot move a group", () => {
    const { getAllByRole, getByRole } = renderWizard()

    fireEvent.click(getAllByRole("switch", { name: "preferences.deletePlanWizard.enableKey" })[0])
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.buildPlan" }))

    const payload = createMutation.mutate.mock.calls[0][0] as DeletePlanCreateInput
    expect(payload.keys?.[0]).toEqual({ field: "GROUP_SIZE", enabled: false, direction: "ASC" })
  })

  it("sends the dead groups switch as the user left it", () => {
    const { getByRole } = renderWizard()

    fireEvent.click(getByRole("switch", { name: "preferences.deletePlanWizard.removeDeadGroups" }))
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.buildPlan" }))

    const payload = createMutation.mutate.mock.calls[0][0] as DeletePlanCreateInput
    expect(payload.removeDeadGroups).toBe(false)
  })

  it("cancels the active run before it rebuilds over it", () => {
    const { getByRole } = renderWizard({ initialRunId: 7 })

    fireEvent.click(getByRole("button", { name: "preferences.deletePlan.back" }))
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanWizard.buildPlan" }))

    // One active run per instance: the create waits for the cancel to succeed.
    expect(createMutation.mutate).not.toHaveBeenCalled()
    expect(cancelMutation.mutate).toHaveBeenCalledWith(7, expect.any(Object))

    const cancelOptions = cancelMutation.mutate.mock.calls[0][1] as { onSuccess: () => void }
    cancelOptions.onSuccess()
    expect(createMutation.mutate).toHaveBeenCalledTimes(1)
  })

  it("shows the dead groups in their own section with their own total", () => {
    const ranked = previewRun.groups[0]
    const dead = { ...ranked, id: 93, contentPath: "/data/DeadGroup", dead: true, decidingKey: "SIZE" }
    runQuery.data = { ...previewRun, groups: [ranked, dead], deadBytes: 2_000_000_000 }
    renderWizard({ initialRunId: 7 })

    const text = document.body.textContent
    expect(text).toContain("/data/DeadGroup")
    expect(text).toContain("preferences.deletePlanPreview.deadGroups(1)")
    expect(text).toContain(`preferences.deletePlanPreview.deadTotal(${formatBytes(2_000_000_000)})`)
  })

  it("names the key that decided a group's position", () => {
    runQuery.data = { ...previewRun, groups: [{ ...previewRun.groups[0], decidingKey: "NUM_COMPLETE" }] }
    renderWizard({ initialRunId: 7 })

    expect(document.body.textContent).toContain("queryBuilder.fields.NUM_COMPLETE")
  })

  it("shows the chain the run used, in order", () => {
    runQuery.data = {
      ...previewRun,
      keys: [
        { field: "NUM_COMPLETE", enabled: true, direction: "DESC" },
        { field: "GROUP_SIZE", enabled: true, direction: "ASC" },
      ],
      groups: [{ ...previewRun.groups[0], decidingKey: "SIZE" }],
    }
    renderWizard({ initialRunId: 7 })

    expect(document.body.textContent).toContain(
      "preferences.deletePlanPreview.ranking: queryBuilder.fields.NUM_COMPLETE preferences.deletePlanWizard.directionDesc · queryBuilder.fields.GROUP_SIZE preferences.deletePlanWizard.directionAsc"
    )
  })
})
