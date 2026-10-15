/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { afterEach, describe, expect, it } from "vitest"
import { http, HttpResponse } from "msw"
import { server } from "@/test/msw/server"
import "@/i18n"
import { api, APIError } from "@/lib/api"
import type { DeletePlanRun } from "@/types"

// The run body is echoed back, so the test only needs the fields it asserts on.
const run: DeletePlanRun = {
  id: 7,
  instanceId: 3,
  status: "preview_ready",
  targetBytes: 250,
  keys: [],
  removeDeadGroups: true,
  waitingForSync: false,
  plannedBytes: 300,
  deadBytes: 0,
  overshootBytes: 50,
  shortfallBytes: 0,
  targetReached: true,
  removedBytes: 0,
  errorMessage: "",
  warningMessage: "",
  createdAt: "2026-10-14T03:00:00Z",
  updatedAt: "2026-10-14T03:00:04Z",
  completedAt: null,
  groups: [],
  excludedGroups: [],
}

describe("api delete plan — contract", () => {
  afterEach(() => {
    server.resetHandlers()
  })

  it("PATCHes the unticked set and returns the run as-is", async () => {
    let captured: Request | undefined
    server.use(
      http.patch("*/api/instances/3/delete-plan/runs/7/rows", async ({ request }) => {
        captured = request.clone()
        return HttpResponse.json(run)
      })
    )

    const result = await api.updateDeletePlanRows(3, 7, [11, 12])

    expect(captured?.method).toBe("PATCH")
    const url = new URL(captured!.url)
    expect(url.pathname).toBe("/api/instances/3/delete-plan/runs/7/rows")
    expect(await captured!.json()).toEqual({ unticked: [11, 12] })
    expect(result).toEqual(run)
  })

  it("POSTs the refill with no body", async () => {
    let captured: Request | undefined
    server.use(
      http.post("*/api/instances/3/delete-plan/runs/7/refill", ({ request }) => {
        captured = request.clone()
        return HttpResponse.json({ ...run, overshootBytes: 150 })
      })
    )

    const result = await api.refillDeletePlanRun(3, 7)

    expect(captured?.method).toBe("POST")
    expect(new URL(captured!.url).pathname).toBe("/api/instances/3/delete-plan/runs/7/refill")
    expect(await captured!.text()).toBe("")
    expect(result.overshootBytes).toBe(150)
  })

  it("POSTs a new plan with the target, the chain and the dead-groups switch", async () => {
    let captured: Request | undefined
    server.use(
      http.post("*/api/instances/3/delete-plan/runs", async ({ request }) => {
        captured = request.clone()
        return HttpResponse.json(run, { status: 202 })
      })
    )

    await api.createDeletePlan(3, {
      targetBytes: 250,
      keys: [{ field: "GROUP_SIZE", enabled: true, direction: "ASC" }],
      removeDeadGroups: false,
    })

    expect(await captured!.json()).toEqual({
      targetBytes: 250,
      keys: [{ field: "GROUP_SIZE", enabled: true, direction: "ASC" }],
      removeDeadGroups: false,
    })
  })

  it("lists runs with the limit query and unwraps the runs array", async () => {
    let captured: Request | undefined
    server.use(
      http.get("*/api/instances/3/delete-plan/runs", ({ request }) => {
        captured = request.clone()
        return HttpResponse.json({ runs: [run] })
      })
    )

    const result = await api.listDeletePlanRuns(3, { limit: 5 })

    expect(new URL(captured!.url).search).toBe("?limit=5")
    expect(result).toEqual([run])
  })

  it("surfaces the 409 conflict message when a plan is already active", async () => {
    server.use(
      http.post("*/api/instances/3/delete-plan/runs", () =>
        HttpResponse.json({ error: "a delete plan is already active for this instance" }, { status: 409 })
      )
    )

    const error = await api.createDeletePlan(3, { targetBytes: 250 }).catch((e) => e)

    expect(error).toBeInstanceOf(APIError)
    expect((error as APIError).status).toBe(409)
    expect((error as APIError).message).toBe("a delete plan is already active for this instance")
  })
})
