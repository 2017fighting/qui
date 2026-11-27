
import { DeletePlanRunProgress } from "@/components/deleteplan/DeletePlanRunProgress"
import { DeletePlanTargetStep } from "@/components/deleteplan/DeletePlanTargetStep"
import {
  DeletePlanChainSummary,
  DeletePlanExcludedTable,
  DeletePlanGroupsTable,
  DeletePlanTotals,
  DeletePlanTotalsGrid
} from "@/components/deleteplan/DeletePlanRunDetails"
import { deletePlanExcludeReasonKey, deletePlanStatusLabelKey } from "@/components/deleteplan/deletePlanLabels"
import { DEFAULT_DELETE_PLAN_KEYS, NON_SORTABLE_KEY_FIELDS } from "@/components/deleteplan/deletePlanKeys"
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
import { useCancelDeletePlanRun, useConfirmDeletePlanRun, useCreateDeletePlan, useDeletePlanRun, useRefillDeletePlanRun, useUpdateDeletePlanRows } from "@/hooks/useDeletePlan"
import { APIError } from "@/lib/api"
import { type CsvColumn, downloadBlob, toCsv } from "@/lib/csv-export"
import { BYTES_PER_UNIT, type ByteUnit } from "@/lib/unit-format"
import { formatBytes } from "@/lib/utils"
import type { DeletePlanRow, DeletePlanRunStatus, KeyRule } from "@/types"
import { Download, Loader2, RefreshCw, X } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

interface DeletePlanWizardDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  instanceId: number
  instanceName?: string

  initialRunId?: number
}

type WizardStep = "target" | "preview" | "confirm"

const DEFAULT_TARGET_UNIT: ByteUnit = "TiB"
const DEFAULT_TARGET_VALUE = "10"

const REBUILDABLE_STATUSES: DeletePlanRunStatus[] = ["pending", "planning", "preview_ready"]

export function DeletePlanWizardDialog({
  open,
  onOpenChange,
  instanceId,
  instanceName,
  initialRunId,
}: DeletePlanWizardDialogProps) {
  const { t } = useTranslation("instances")
  const [step, setStep] = useState<WizardStep>(initialRunId ? "preview" : "target")
  const [targetValue, setTargetValue] = useState(DEFAULT_TARGET_VALUE)
  const [targetUnit, setTargetUnit] = useState<ByteUnit>(DEFAULT_TARGET_UNIT)
  const [chain, setChain] = useState<KeyRule[]>(() => DEFAULT_DELETE_PLAN_KEYS.map((key) => ({ ...key })))
  const [removeDeadGroups, setRemoveDeadGroups] = useState(true)
  const [runId, setRunId] = useState<number | undefined>(initialRunId)
  const [conflict, setConflict] = useState(false)
  const [targetInvalid, setTargetInvalid] = useState(false)

  const createMutation = useCreateDeletePlan(instanceId)
  const cancelMutation = useCancelDeletePlanRun(instanceId)
  const confirmMutation = useConfirmDeletePlanRun(instanceId)
  const updateRowsMutation = useUpdateDeletePlanRows(instanceId)
  const refillMutation = useRefillDeletePlanRun(instanceId)
  const runQuery = useDeletePlanRun(instanceId, runId, { enabled: open })
  const run = runQuery.data

  const keyDisabledFields = useMemo(
    () => NON_SORTABLE_KEY_FIELDS.map((field) => ({ field, reason: t("preferences.deletePlanWizard.notSortableField") })),
    [t]
  )

  const rankedGroups = run ? run.groups.filter((group) => !group.dead) : []
  const deadGroups = run ? run.groups.filter((group) => group.dead) : []
  const untickedGroups = run ? run.groups.filter((group) => group.unticked) : []
  const canRefill = untickedGroups.length > 0
  const includedRankedCount = rankedGroups.filter((group) => group.included).length

  const targetBytes = Math.round(Number(targetValue) * BYTES_PER_UNIT[targetUnit])

  const updateChainKey = (index: number, patch: Partial<KeyRule>) => {
    setChain((prev) => prev.map((key, i) => (i === index ? { ...key, ...patch } : key)))
  }

  const handleBuild = () => {
    if (!Number.isFinite(targetBytes) || targetBytes <= 0) {
      setTargetInvalid(true)
      return
    }

    setTargetInvalid(false)
    setConflict(false)

    const create = () => {
      createMutation.mutate(
        {
          targetBytes,
          keys: chain.map(({ field, enabled, direction }) => ({ field, enabled, direction })),
          removeDeadGroups,
        },
        {
          onSuccess: (created) => {
            setRunId(created.id)
            setStep("preview")
          },
          onError: (error) => {
            if (error instanceof APIError && error.status === 409) {
              setConflict(true)
              return
            }
            toast.error(t("preferences.deletePlanOverview.toast.planStartFailed"), {
              description: error instanceof Error ? error.message : undefined,
            })
          },
        }
      )
    }

    if (run && runId !== undefined && REBUILDABLE_STATUSES.includes(run.status)) {
      cancelMutation.mutate(run.id, {
        onSuccess: create,
        onError: (error) => {
          toast.error(t("preferences.deletePlanOverview.toast.cancelFailed"), {
            description: error instanceof Error ? error.message : undefined,
          })
        },
      })
      return
    }

    create()
  }

  const handleCancelRun = () => {
    if (!run) return
    cancelMutation.mutate(run.id, {
      onSuccess: () => {
        toast.success(t("preferences.deletePlanOverview.toast.planCanceled"))
        onOpenChange(false)
      },
      onError: (error) => {
        toast.error(t("preferences.deletePlanOverview.toast.cancelFailed"), {
          description: error instanceof Error ? error.message : undefined,
        })
      },
    })
  }

  const handleConfirm = () => {
    if (!run) return
    confirmMutation.mutate(run.id, {
      onError: (error) => {
        toast.error(t("preferences.deletePlanConfirm.startFailed"), {
          description: error instanceof Error ? error.message : undefined,
        })
      },
    })
  }

  const handleToggleGroup = (row: DeletePlanRow) => {
    if (!run) return
    const currentUnticked = run.groups.filter((group) => group.unticked).map((group) => group.id)
    const unticked = row.included
      ? [...currentUnticked, row.id]
      : currentUnticked.filter((id) => id !== row.id)
    updateRowsMutation.mutate({ runId: run.id, unticked }, {
      onError: (error) => {
        toast.error(t("preferences.deletePlanPreview.untickFailed"), {
          description: error instanceof Error ? error.message : undefined,
        })
      },
    })
  }

  const handleRefill = () => {
    if (!run) return
    refillMutation.mutate(run.id, {
      onSuccess: (updated) => {
        toast.success(t("preferences.deletePlanPreview.refilled", { overshoot: formatBytes(updated.overshootBytes) }))
      },
      onError: (error) => {
        toast.error(t("preferences.deletePlanPreview.refillFailed"), {
          description: error instanceof Error ? error.message : undefined,
        })
      },
    })
  }

  const exportRows: CsvColumn<DeletePlanRow>[] = [
    { header: "Content path", accessor: (row) => row.contentPath },
    { header: "Size", accessor: (row) => formatBytes(row.sizeBytes) },
    { header: "Size (bytes)", accessor: (row) => row.sizeBytes },
    { header: "Members", accessor: (row) => row.memberCount },
    { header: "Status", accessor: (row) => (row.excluded ? "excluded" : "planned") },
    { header: "Reason", accessor: (row) => (row.excludeReason ? t([deletePlanExcludeReasonKey(row.excludeReason), "preferences.deletePlan.excludeReasons.unknown"]) : "") },
  ]

  const handleExport = () => {
    if (!run) return
    const rows = [...run.groups, ...run.excludedGroups]
    if (rows.length === 0) return

    try {
      downloadBlob(toCsv(rows, exportRows), `delete_plan_${run.id}.csv`)
      toast.success(t("preferences.deletePlanPreview.exported"))
    } catch (error) {
      toast.error(t("preferences.deletePlanPreview.exportFailed"), {
        description: error instanceof Error ? error.message : undefined,
      })
    }
  }

  const isWaiting = run?.status === "pending" && run.waitingForSync
  const isPlanning = run?.status === "pending" || run?.status === "planning"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-4xl max-h-[85dvh] flex flex-col">
        <DialogHeader>
          <DialogTitle>{t("preferences.deletePlanWizard.title")}</DialogTitle>
          <DialogDescription>{t("preferences.deletePlanWizard.description")}</DialogDescription>
        </DialogHeader>

        {step === "target" && (
          <DeletePlanTargetStep
            instanceId={instanceId}
            instanceName={instanceName}
            targetValue={targetValue}
            setTargetValue={setTargetValue}
            targetUnit={targetUnit}
            setTargetUnit={setTargetUnit}
            targetInvalid={targetInvalid}
            conflict={conflict}
            chain={chain}
            setChain={setChain}
            updateChainKey={updateChainKey}
            keyDisabledFields={keyDisabledFields}
            removeDeadGroups={removeDeadGroups}
            setRemoveDeadGroups={setRemoveDeadGroups}
          />
        )}

        {step === "preview" && (
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

            {run && (isWaiting || isPlanning) && (
              <div className="flex flex-col items-center justify-center gap-3 py-12 text-center">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                {isWaiting ? (
                  <>
                    <p className="text-sm font-medium">{t("preferences.deletePlanWizard.waitingTitle")}</p>
                    <p className="max-w-md text-xs text-muted-foreground">{t("preferences.deletePlanWizard.waitingDescription")}</p>
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">{t("preferences.deletePlanWizard.planning")}</p>
                )}
              </div>
            )}

            {run && run.status === "preview_ready" && (
              <>
                <DeletePlanTotals run={run} canRefill={canRefill} />
                <DeletePlanChainSummary keys={run.keys} />
                <p className="text-xs text-muted-foreground">
                  {t("preferences.deletePlanPreview.includedGroups", { count: includedRankedCount })}
                  {" · "}
                  {t("preferences.deletePlanPreview.excludedGroups", { count: run.excludedGroups.length })}
                </p>
                <div className="rounded-lg border overflow-hidden">
                  <div className="overflow-auto max-h-[40vh]">
                    <DeletePlanGroupsTable
                      groups={rankedGroups}
                      selectable
                      disabled={updateRowsMutation.isPending || refillMutation.isPending}
                      onToggle={handleToggleGroup}
                    />
                  </div>
                </div>
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

            {run && run.status !== "preview_ready" && !isWaiting && !isPlanning && (
              <div className="space-y-2">
                <Badge variant="outline">{t([deletePlanStatusLabelKey(run.status), "preferences.deletePlan.statusLabels.unknown"])}</Badge>
                {run.errorMessage && (
                  <p className="whitespace-pre-wrap break-all text-sm text-destructive">{run.errorMessage}</p>
                )}
              </div>
            )}
          </div>
        )}

        {step === "confirm" && run && (
          <div className="space-y-4 py-2">
            <DeletePlanTotalsGrid run={run} />
            {run.deadBytes > 0 && (
              <p className="text-xs text-muted-foreground">
                {t("preferences.deletePlanConfirm.deadTotal", { dead: formatBytes(run.deadBytes) })}
              </p>
            )}
            {run.status === "preview_ready" ? (
              <div className="rounded-md border border-warning/20 bg-warning/10 p-3 text-sm text-warning">
                <p className="font-medium">{t("preferences.deletePlanConfirm.title")}</p>
                <p>{t("preferences.deletePlanConfirm.description")}</p>
                <p className="mt-1">{t("preferences.deletePlanConfirm.otherInstances")}</p>
              </div>
            ) : (
              <DeletePlanRunProgress run={run} />
            )}
          </div>
        )}

        <DialogFooter className="mt-4 sm:justify-between">
          <div className="flex gap-2">
            {step !== "target" && !(step === "confirm" && run && run.status !== "preview_ready") && (
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep(step === "confirm" ? "preview" : "target")}
              >
                {t("preferences.deletePlan.back")}
              </Button>
            )}
            {step === "preview" && run && (isWaiting || isPlanning) && (
              <Button
                type="button"
                variant="ghost"
                onClick={handleCancelRun}
                disabled={cancelMutation.isPending}
              >
                {cancelMutation.isPending ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <X className="mr-2 h-4 w-4" />
                )}
                {t("preferences.deletePlan.cancel")}
              </Button>
            )}
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("preferences.deletePlan.close")}
            </Button>

            {step === "target" && (
              <Button type="button" onClick={handleBuild} disabled={createMutation.isPending}>
                {createMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t("preferences.deletePlanWizard.buildPlan")}
              </Button>
            )}

            {step === "preview" && (
              <>
                {run?.status === "preview_ready" && canRefill && (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleRefill}
                    disabled={refillMutation.isPending}
                  >
                    {refillMutation.isPending ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <RefreshCw className="mr-2 h-4 w-4" />
                    )}
                    {t("preferences.deletePlanPreview.refill")}
                  </Button>
                )}
                {run?.status === "preview_ready" && (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleExport}
                  >
                    <Download className="mr-2 h-4 w-4" />
                    {t("preferences.deletePlanPreview.exportCSV")}
                  </Button>
                )}
                <Button
                  type="button"
                  onClick={() => setStep("confirm")}
                  disabled={run?.status !== "preview_ready"}
                >
                  {t("preferences.deletePlanWizard.reviewAndConfirm")}
                </Button>
              </>
            )}

            {step === "confirm" && run?.status === "preview_ready" && (
              <Button
                type="button"
                variant="destructive"
                onClick={handleConfirm}
                disabled={confirmMutation.isPending}
              >
                {confirmMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t("preferences.deletePlanConfirm.confirm")}
              </Button>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
