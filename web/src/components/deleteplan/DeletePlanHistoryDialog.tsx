
import { DeletePlanRunProgress } from "@/components/deleteplan/DeletePlanRunProgress"
import {
  DeletePlanConfiguration,
  DeletePlanExcludedTable,
  DeletePlanGroupsTable,
  DeletePlanStatGrid
} from "@/components/deleteplan/DeletePlanRunDetails"
import { deletePlanStatusLabelKey } from "@/components/deleteplan/deletePlanLabels"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog"
import { useDeletePlanRun } from "@/hooks/useDeletePlan"
import { formatBytes } from "@/lib/utils"
import { Loader2 } from "lucide-react"
import { useTranslation } from "react-i18next"

interface DeletePlanHistoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  instanceId: number
  runId: number
}

export function DeletePlanHistoryDialog({
  open,
  onOpenChange,
  instanceId,
  runId,
}: DeletePlanHistoryDialogProps) {
  const { t } = useTranslation("instances")
  const runQuery = useDeletePlanRun(instanceId, runId, { enabled: open })
  const run = runQuery.data

  const rankedGroups = run ? run.groups.filter((group) => !group.dead) : []
  const deadGroups = run ? run.groups.filter((group) => group.dead) : []
  const includedRankedCount = rankedGroups.filter((group) => group.included).length

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-4xl max-h-[85dvh] flex flex-col">
        <DialogHeader>
          <DialogTitle>{t("preferences.deletePlanHistory.title")}</DialogTitle>
          <DialogDescription>{t("preferences.deletePlanHistory.description")}</DialogDescription>
        </DialogHeader>

        <div className="flex-1 min-h-0 overflow-y-auto space-y-4 py-2">
          {runQuery.isError && (
            <p className="text-sm text-destructive">{t("preferences.deletePlanWizard.loadFailed")}</p>
          )}

          {runQuery.isLoading && !run && (
            <div className="flex flex-col items-center justify-center gap-3 py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t("preferences.deletePlanPreview.loading")}</p>
            </div>
          )}

          {run && (
            <>
              <Badge variant="outline" size="sm">
                {t([deletePlanStatusLabelKey(run.status), "preferences.deletePlan.statusLabels.unknown"])}
              </Badge>

              <DeletePlanRunProgress run={run} />

              <DeletePlanStatGrid
                stats={[
                  { label: t("preferences.deletePlanPreview.target"), value: formatBytes(run.targetBytes) },
                  { label: t("preferences.deletePlanPreview.planned"), value: formatBytes(run.plannedBytes) },
                  { label: t("preferences.deletePlanHistory.removed"), value: formatBytes(run.removedBytes) },
                ]}
              />

              <DeletePlanConfiguration keys={run.keys} removeDeadGroups={run.removeDeadGroups} />

              <p className="text-xs text-muted-foreground">
                {t("preferences.deletePlanPreview.includedGroups", { count: includedRankedCount })}
                {" · "}
                {t("preferences.deletePlanPreview.excludedGroups", { count: run.excludedGroups.length })}
              </p>

              {rankedGroups.length > 0 && (
                <div className="rounded-lg border overflow-hidden">
                  <div className="overflow-auto max-h-[40vh]">
                    <DeletePlanGroupsTable groups={rankedGroups} />
                  </div>
                </div>
              )}

              {deadGroups.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">
                    {t("preferences.deletePlanPreview.deadGroups", { count: deadGroups.length })}
                    {" · "}
                    {t("preferences.deletePlanPreview.deadTotal", { total: formatBytes(run.deadBytes) })}
                  </p>
                  <div className="rounded-lg border overflow-hidden">
                    <div className="overflow-auto max-h-[30vh]">
                      <DeletePlanGroupsTable groups={deadGroups} />
                    </div>
                  </div>
                </div>
              )}

              {run.excludedGroups.length > 0 && (
                <div className="rounded-lg border overflow-hidden">
                  <div className="overflow-auto max-h-[30vh]">
                    <DeletePlanExcludedTable groups={run.excludedGroups} />
                  </div>
                </div>
              )}
            </>
          )}
        </div>

        <DialogFooter className="mt-4">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("preferences.deletePlan.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
