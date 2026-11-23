
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { useActivityStream } from "@/contexts/SyncStreamContext"
import { api } from "@/lib/api"
import type {
  DeletePlanCreateInput,
  DeletePlanRun,
  DeletePlanRunStatus,
  DeletePlanRunSummary
} from "@/types"

const ACTIVE_STATUSES: DeletePlanRunStatus[] = ["pending", "planning", "deleting"]

function isActive(status: DeletePlanRunStatus): boolean {
  return ACTIVE_STATUSES.includes(status)
}

export function useDeletePlanRuns(
  instanceId: number,
  options?: { limit?: number; enabled?: boolean }
) {
  const limit = options?.limit
  const shouldEnable = (options?.enabled ?? true) && instanceId > 0

  useActivityStream(shouldEnable)

  return useQuery({
    queryKey: ["delete-plan", instanceId, "runs", limit ?? null],
    queryFn: () =>
      api.listDeletePlanRuns(instanceId, {
        ...(limit !== undefined ? { limit } : {}),
      }),
    enabled: shouldEnable,
    refetchInterval: (query) => {
      const runs = query.state.data as DeletePlanRunSummary[] | undefined
      if (!runs) {
        return false
      }
      return runs.some((run) => isActive(run.status)) ? 1_000 : false
    },
  })
}

export function useDeletePlanRun(
  instanceId: number,
  runId: number | undefined,
  options?: { enabled?: boolean }
) {
  const shouldEnable = (options?.enabled ?? true) && instanceId > 0 && !!runId

  useActivityStream(shouldEnable)

  return useQuery({
    queryKey: ["delete-plan", instanceId, "run", runId ?? null],
    queryFn: () => api.getDeletePlanRun(instanceId, runId as number),
    enabled: shouldEnable,
    refetchInterval: (query) => {
      const run = query.state.data as DeletePlanRun | undefined
      if (!run) {
        return false
      }
      return isActive(run.status) ? 1_000 : false
    },
  })
}

export function useCreateDeletePlan(instanceId: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: DeletePlanCreateInput) => api.createDeletePlan(instanceId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["delete-plan", instanceId, "runs"] })
    },
  })
}

export function useCancelDeletePlanRun(instanceId: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (runId: number) => api.cancelDeletePlanRun(instanceId, runId),
    onSuccess: (_data, runId) => {
      queryClient.invalidateQueries({ queryKey: ["delete-plan", instanceId, "runs"] })
      queryClient.invalidateQueries({ queryKey: ["delete-plan", instanceId, "run", runId] })
    },
  })
}

export function useConfirmDeletePlanRun(instanceId: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (runId: number) => api.confirmDeletePlanRun(instanceId, runId),
    onSuccess: (_data, runId) => {
      queryClient.invalidateQueries({ queryKey: ["delete-plan", instanceId, "runs"] })
      queryClient.invalidateQueries({ queryKey: ["delete-plan", instanceId, "run", runId] })
    },
  })
}

export function useUpdateDeletePlanRows(instanceId: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ runId, unticked }: { runId: number; unticked: number[] }) =>
      api.updateDeletePlanRows(instanceId, runId, unticked),
    onSuccess: (_data, { runId }) => {
      queryClient.invalidateQueries({ queryKey: ["delete-plan", instanceId, "run", runId] })
    },
  })
}

export function useRefillDeletePlanRun(instanceId: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (runId: number) => api.refillDeletePlanRun(instanceId, runId),
    onSuccess: (_data, runId) => {
      queryClient.invalidateQueries({ queryKey: ["delete-plan", instanceId, "run", runId] })
    },
  })
}
