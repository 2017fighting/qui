/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, render } from "@testing-library/react"
import type { DeletePlanRunProgressRun } from "@/components/instances/preferences/DeletePlanRunProgress"

vi.mock("react-i18next", async (importOriginal) => ({
  ...await importOriginal<typeof import("react-i18next")>(),
  // Key echo: the assertion names the key the UI picked.
  useTranslation: () => ({
    t: (key: string | string[], opts?: Record<string, unknown>) => {
      const resolved = Array.isArray(key) ? key[0] : key
      return opts ? `${resolved}(${Object.values(opts).join(",")})` : resolved
    },
  }),
}))

import { DeletePlanRunProgress } from "@/components/instances/preferences/DeletePlanRunProgress"
import "@/i18n"

afterEach(cleanup)

function run(overrides: Partial<DeletePlanRunProgressRun>): DeletePlanRunProgressRun {
  return {
    status: "deleting",
    removedBytes: 0,
    plannedBytes: 0,
    errorMessage: "",
    warningMessage: "",
    ...overrides,
  }
}

describe("DeletePlanRunProgress", () => {
  it("renders nothing before the deletion starts", () => {
    const { container } = render(<DeletePlanRunProgress run={run({ status: "preview_ready" })} />)

    expect(container.textContent).toBe("")
  })

  it("shows the progress while deleting", () => {
    render(<DeletePlanRunProgress run={run({ status: "deleting", plannedBytes: 1000, removedBytes: 250 })} />)

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.deleting")
    expect(document.body.textContent).toContain("preferences.deletePlanOverview.progress(")
  })

  it("shows the removed bytes on a completed run", () => {
    render(<DeletePlanRunProgress run={run({ status: "completed", removedBytes: 5_000_000_000 })} />)

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.completedResult(")
  })

  it("shows the warning of a completed run that lost chunks", () => {
    render(<DeletePlanRunProgress run={run({ status: "completed", warningMessage: "one chunk failed" })} />)

    expect(document.body.textContent).toContain("one chunk failed")
  })

  it("shows the error of a failed run", () => {
    render(<DeletePlanRunProgress run={run({ status: "failed", errorMessage: "read torrents: boom" })} />)

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.failedResult")
    expect(document.body.textContent).toContain("read torrents: boom")
  })

  it("shows the canceled result", () => {
    render(<DeletePlanRunProgress run={run({ status: "canceled" })} />)

    expect(document.body.textContent).toContain("preferences.deletePlanOverview.canceledResult")
  })
})
