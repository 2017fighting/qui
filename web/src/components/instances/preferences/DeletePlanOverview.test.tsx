/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render } from "@testing-library/react"
import { TooltipProvider } from "@/components/ui/tooltip"
import { formatBytes } from "@/lib/utils"
import type { DeletePlanRunSummary, Instance } from "@/types"

vi.mock("react-i18next", async (importOriginal) => ({
  ...await importOriginal<typeof import("react-i18next")>(),
  useTranslation: () => ({
    t: (key: string | string[], opts?: Record<string, unknown>) => {
      const resolved = Array.isArray(key) ? key[0] : key
      return opts ? `${resolved}(${Object.values(opts).join(",")})` : resolved
    },
  }),
}))

// Stable singletons: a fresh object per render would rerun effects forever.
const { instancesQuery, runsQuery, cancelMutation, createMutation, confirmMutation, runQuery } = vi.hoisted(() => {
  const instance = {
    id: 1,
    name: "seedbox-europe-west-archive-01",
    isActive: true,
    filesystemMode: "local",
    capabilities: { read: true, identity: true, write: true, content: true },
  }
  const run = {
    id: 7,
    instanceId: 1,
    status: "preview_ready",
    targetBytes: 10_995_116_277_760,
    keys: [],
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
  }
  return {
    instancesQuery: { instances: [instance as unknown as Instance] },
    runsQuery: { data: [run as unknown as DeletePlanRunSummary], isLoading: false },
    cancelMutation: { isPending: false, mutate: vi.fn() },
    createMutation: { isPending: false, mutate: vi.fn() },
    confirmMutation: { isPending: false, mutate: vi.fn() },
    runQuery: { data: undefined as unknown, isLoading: false, isError: false },
  }
})

vi.mock("@/hooks/useInstances", () => ({
  useInstances: () => instancesQuery,
}))

vi.mock("@/hooks/useDeletePlan", () => ({
  useDeletePlanRuns: () => runsQuery,
  useDeletePlanRun: () => runQuery,
  useCreateDeletePlan: () => createMutation,
  useCancelDeletePlanRun: () => cancelMutation,
  useConfirmDeletePlanRun: () => confirmMutation,
}))

import { DeletePlanOverview } from "@/components/instances/preferences/DeletePlanOverview"
import "@/i18n"

beforeEach(() => {
  instancesQuery.instances = [
    {
      id: 1,
      name: "seedbox-europe-west-archive-01",
      isActive: true,
      filesystemMode: "local",
      capabilities: { read: true, identity: true, write: true, content: true },
    } as unknown as Instance,
  ]
  runsQuery.data = [
    {
      id: 7,
      instanceId: 1,
      status: "preview_ready",
      targetBytes: 10_995_116_277_760,
      keys: [],
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
    } as unknown as DeletePlanRunSummary,
  ]
  cancelMutation.mutate.mockReset()
  // Radix dialog and tooltip measure through ResizeObserver, which jsdom lacks.
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

function renderCard() {
  return render(
    <TooltipProvider>
      <DeletePlanOverview />
    </TooltipProvider>
  )
}

describe("DeletePlanOverview card", () => {
  it("lists the instance and opens the wizard from New Plan", () => {
    runsQuery.data = []
    const { getByRole } = renderCard()

    expect(document.body.textContent).toContain("seedbox-europe-west-archive-01")
    fireEvent.click(getByRole("button", { name: "preferences.deletePlanOverview.newPlan" }))

    expect(document.body.textContent).toContain("preferences.deletePlanWizard.title")
  })

  it("shows the latest run status and cancels it by id", () => {
    const { getByRole } = renderCard()

    expect(document.body.textContent).toContain("preferences.deletePlan.statusLabels.preview_ready")
    expect(document.body.textContent).toContain("preferences.deletePlanOverview.activePlan")

    fireEvent.click(getByRole("button", { name: "preferences.deletePlan.cancel" }))
    expect(cancelMutation.mutate).toHaveBeenCalledWith(7, expect.any(Object))
  })

  it("disables New Plan while a run is active and offers View Plan", () => {
    const { getByRole } = renderCard()

    expect(getByRole("button", { name: "preferences.deletePlanOverview.newPlan" }).hasAttribute("disabled")).toBe(true)
    expect(getByRole("button", { name: "preferences.deletePlanOverview.viewPlan" })).toBeDefined()
  })

  it("allows New Plan when the latest run finished", () => {
    runsQuery.data = [{ ...runsQuery.data![0], status: "completed" }]
    const { getByRole } = renderCard()

    expect(getByRole("button", { name: "preferences.deletePlanOverview.newPlan" }).hasAttribute("disabled")).toBe(false)
    expect(document.body.textContent).toContain("preferences.deletePlan.statusLabels.completed")
  })

  it("shows the removal result when the latest run completed", () => {
    runsQuery.data = [{ ...runsQuery.data![0], status: "completed", removedBytes: 5_000_000_000 }]
    renderCard()

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.completedResult(")
  })

  it("shows the warning when a completed run lost chunks", () => {
    runsQuery.data = [{ ...runsQuery.data![0], status: "completed", warningMessage: "one chunk failed" }]
    renderCard()

    expect(document.body.textContent).toContain("one chunk failed")
  })

  it("shows the progress while the latest run deletes", () => {
    runsQuery.data = [{ ...runsQuery.data![0], status: "deleting", plannedBytes: 1000, removedBytes: 250 }]
    renderCard()

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.deleting")
    expect(document.body.textContent).toContain("preferences.deletePlanOverview.progress(")
  })

  it("shows the error when the latest run failed", () => {
    runsQuery.data = [{ ...runsQuery.data![0], status: "failed", errorMessage: "read torrents: boom" }]
    renderCard()

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.failedResult")
    expect(document.body.textContent).toContain("read torrents: boom")
  })

  it("shows the canceled result", () => {
    runsQuery.data = [{ ...runsQuery.data![0], status: "canceled" }]
    renderCard()

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.canceledResult")
  })

  it("shows the empty state when no instance is configured", () => {
    instancesQuery.instances = []
    renderCard()

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.noInstancesDescription")
  })
})

describe("DeletePlanOverview history", () => {
  it("lists finished runs with their status, target, planned reclaim, and removed bytes", () => {
    runsQuery.data = [{
      ...runsQuery.data![0],
      id: 9,
      status: "completed",
      targetBytes: 10_995_116_277_760,
      plannedBytes: 11_045_000_000_000,
      removedBytes: 5_000_000_000,
    }]
    renderCard()

    const text = document.body.textContent
    expect(text).toContain("preferences.deletePlanOverview.history")
    expect(text).toContain("preferences.deletePlan.statusLabels.completed")
    expect(text).toContain("preferences.deletePlanPreview.target")
    expect(text).toContain("preferences.deletePlanPreview.planned")
    expect(text).toContain("preferences.deletePlanHistory.removed")
    expect(text).toContain(formatBytes(5_000_000_000))
  })

  it("does not list the active run as history", () => {
    runsQuery.data = [{ ...runsQuery.data![0], status: "preview_ready" }]
    renderCard()

    expect(document.body.textContent).not.toContain("preferences.deletePlanOverview.history")
  })

  it("opens a finished run read-only from the history list", () => {
    const completed = { ...runsQuery.data![0], id: 9, status: "completed" as const }
    runsQuery.data = [completed]
    runQuery.data = { ...completed, groups: [], excludedGroups: [] }
    const { getByRole, queryByRole } = renderCard()

    fireEvent.click(getByRole("button", { name: "preferences.deletePlanHistory.viewRun(9)" }))

    expect(document.body.textContent).toContain("preferences.deletePlanHistory.title")
    expect(queryByRole("button", { name: "preferences.deletePlanConfirm.confirm" })).toBeNull()
    expect(queryByRole("button", { name: "preferences.deletePlan.cancel" })).toBeNull()
  })
})
