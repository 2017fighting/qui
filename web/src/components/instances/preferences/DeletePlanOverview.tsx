/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { DeletePlanHistoryDialog } from "@/components/instances/preferences/DeletePlanHistoryDialog"
import { DeletePlanRunProgress } from "@/components/instances/preferences/DeletePlanRunProgress"
import { DeletePlanWizardDialog } from "@/components/instances/preferences/DeletePlanWizardDialog"
import { deletePlanStatusLabelKey } from "@/components/instances/preferences/deletePlanLabels"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { FieldHelp } from "@/components/ui/field-help"
import { useCancelDeletePlanRun, useDeletePlanRuns } from "@/hooks/useDeletePlan"
import { useInstances } from "@/hooks/useInstances"
import { formatBytes } from "@/lib/utils"
import type { DeletePlanRunStatus, Instance } from "@/types"
import { Eye, Loader2, Play, X } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

// A second plan is refused while any of these is the instance's latest run.
const ACTIVE_STATUSES: DeletePlanRunStatus[] = ["pending", "planning", "preview_ready", "deleting"]

interface WizardTarget {
  instanceId: number
  instanceName: string
  runId?: number
}

function InstanceDeletePlanRow({
  instance,
  onNewPlan,
  onViewPlan,
}: {
  instance: Instance
  onNewPlan: () => void
  onViewPlan: (runId: number) => void
}) {
  const { t } = useTranslation("instances")
  const runsQuery = useDeletePlanRuns(instance.id, { limit: 5 })
  const cancelMutation = useCancelDeletePlanRun(instance.id)
  const [historyRunId, setHistoryRunId] = useState<number | null>(null)

  const latestRun = runsQuery.data?.[0]
  const isActive = !!latestRun && ACTIVE_STATUSES.includes(latestRun.status)
  // Finished runs read back read-only; the active run is handled by the wizard.
  const pastRuns = (runsQuery.data ?? []).filter((run) => !ACTIVE_STATUSES.includes(run.status))

  const handleCancel = () => {
    if (!latestRun) return
    cancelMutation.mutate(latestRun.id, {
      onSuccess: () => {
        toast.success(t("preferences.deletePlanOverview.toast.planCanceled"), { description: instance.name })
      },
      onError: (error) => {
        toast.error(t("preferences.deletePlanOverview.toast.cancelFailed"), {
          description: error instanceof Error ? error.message : undefined,
        })
      },
    })
  }

  return (
    <div className="space-y-2 rounded-lg border p-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className="font-medium wrap-anywhere">{instance.name}</span>
          {latestRun && (
            <Badge variant="outline" className="text-xs">
              {t([deletePlanStatusLabelKey(latestRun.status), "preferences.deletePlan.statusLabels.unknown"])}
            </Badge>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {isActive && (
            <Button
              variant="outline"
              size="sm"
              className="h-8"
              onClick={handleCancel}
              disabled={cancelMutation.isPending}
            >
              {cancelMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <>
                  <X className="mr-2 h-4 w-4" />
                  {t("preferences.deletePlan.cancel")}
                </>
              )}
            </Button>
          )}
          {latestRun?.status === "preview_ready" && (
            <Button
              variant="outline"
              size="sm"
              className="h-8"
              onClick={() => onViewPlan(latestRun.id)}
            >
              <Eye className="mr-2 h-4 w-4" />
              {t("preferences.deletePlanOverview.viewPlan")}
            </Button>
          )}
          <Button
            size="sm"
            className="h-8"
            onClick={onNewPlan}
            disabled={isActive}
          >
            <Play className="mr-2 h-4 w-4" />
            {t("preferences.deletePlanOverview.newPlan")}
          </Button>
        </div>
      </div>

      {latestRun && isActive && (
        <div className="space-y-1.5">
          <p className="text-xs text-muted-foreground">{t("preferences.deletePlanOverview.activePlan")}</p>
          <DeletePlanRunProgress run={latestRun} />
        </div>
      )}

      {latestRun && !isActive && <DeletePlanRunProgress run={latestRun} />}

      {pastRuns.length > 0 && (
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">{t("preferences.deletePlanOverview.history")}</p>
          <div className="divide-y rounded-md border">
            {pastRuns.map((run) => (
              <div key={run.id} className="flex flex-wrap items-center justify-between gap-2 p-2">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                  <Badge variant="outline" className="text-xs">
                    {t([deletePlanStatusLabelKey(run.status), "preferences.deletePlan.statusLabels.unknown"])}
                  </Badge>
                  <span>{t("preferences.deletePlanPreview.target")} {formatBytes(run.targetBytes)}</span>
                  <span>{t("preferences.deletePlanPreview.planned")} {formatBytes(run.plannedBytes)}</span>
                  <span>{t("preferences.deletePlanHistory.removed")} {formatBytes(run.removedBytes)}</span>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8"
                  onClick={() => setHistoryRunId(run.id)}
                  aria-label={t("preferences.deletePlanHistory.viewRun", { id: run.id })}
                >
                  <Eye className="mr-2 h-4 w-4" />
                  {t("preferences.deletePlanHistory.view")}
                </Button>
              </div>
            ))}
          </div>
        </div>
      )}

      {!latestRun && !runsQuery.isLoading && (
        <p className="text-xs text-muted-foreground">{t("preferences.deletePlanOverview.noPlanYet")}</p>
      )}

      {historyRunId !== null && (
        <DeletePlanHistoryDialog
          open
          onOpenChange={(open) => !open && setHistoryRunId(null)}
          instanceId={instance.id}
          runId={historyRunId}
        />
      )}
    </div>
  )
}

export function DeletePlanOverview() {
  const { t } = useTranslation("instances")
  const { instances } = useInstances()
  const [wizardTarget, setWizardTarget] = useState<WizardTarget | null>(null)

  const activeInstances = useMemo(
    () => (instances ?? []).filter((inst) => inst.isActive),
    [instances]
  )

  if (!instances || instances.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-lg font-semibold">{t("preferences.deletePlanOverview.title")}</CardTitle>
          <CardDescription>
            {t("preferences.deletePlanOverview.noInstancesDescription")}
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader className="space-y-2">
        <div className="flex items-center gap-2">
          <CardTitle className="text-lg font-semibold">{t("preferences.deletePlanOverview.title")}</CardTitle>
          <FieldHelp>{t("preferences.deletePlanOverview.tooltip")}</FieldHelp>
        </div>
        <CardDescription>{t("preferences.deletePlanOverview.description")}</CardDescription>
      </CardHeader>

      <CardContent className="space-y-3">
        {activeInstances.map((instance) => (
          <InstanceDeletePlanRow
            key={instance.id}
            instance={instance}
            onNewPlan={() =>
              setWizardTarget({ instanceId: instance.id, instanceName: instance.name })
            }
            onViewPlan={(runId) =>
              setWizardTarget({ instanceId: instance.id, instanceName: instance.name, runId })
            }
          />
        ))}
      </CardContent>

      {/* Mounted only while open, so every open starts from the first step. */}
      {wizardTarget && (
        <DeletePlanWizardDialog
          open
          onOpenChange={(open) => !open && setWizardTarget(null)}
          instanceId={wizardTarget.instanceId}
          instanceName={wizardTarget.instanceName}
          initialRunId={wizardTarget.runId}
        />
      )}
    </Card>
  )
}
