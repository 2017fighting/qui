/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { deletePlanExcludeReasonKey, deletePlanStatusLabelKey } from "@/components/instances/preferences/deletePlanLabels"
import { DEFAULT_DELETE_PLAN_KEYS, NON_SORTABLE_KEY_FIELDS, moveKey } from "@/components/instances/preferences/deletePlanKeys"
import { FieldCombobox } from "@/components/query-builder/FieldCombobox"
import { getFieldLabel } from "@/components/query-builder/constants"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { PathCell } from "@/components/ui/path-cell"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useCancelDeletePlanRun, useCreateDeletePlan, useDeletePlanRun } from "@/hooks/useDeletePlan"
import { APIError } from "@/lib/api"
import { type CsvColumn, downloadBlob, toCsv } from "@/lib/csv-export"
import { getStateLabel } from "@/lib/torrent-state-utils"
import { BYTES_PER_UNIT, unitLabel, type ByteUnit } from "@/lib/unit-format"
import { formatBytes } from "@/lib/utils"
import type { ConditionField, DeletePlanRow, DeletePlanRun, DeletePlanRunStatus, KeyRule } from "@/types"
import { AlertTriangle, ArrowDown, ArrowUp, ChevronDown, ChevronRight, ChevronUp, Download, Loader2, X } from "lucide-react"
import { Fragment, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

interface DeletePlanWizardDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  instanceId: number
  instanceName?: string
  // Set when the card reopens a frozen plan that is still the instance's active run.
  initialRunId?: number
}

type WizardStep = "target" | "preview" | "confirm"

const TARGET_UNITS: ByteUnit[] = ["MiB", "GiB", "TiB"]
const DEFAULT_TARGET_UNIT: ByteUnit = "TiB"
const DEFAULT_TARGET_VALUE = "10"

// Rebuilding over the run this wizard opened cancels it first, but only a run
// that has not started deleting can be canceled and recreated.
const REBUILDABLE_STATUSES: DeletePlanRunStatus[] = ["pending", "planning", "preview_ready"]

function TotalsGrid({ run }: { run: DeletePlanRun }) {
  const { t } = useTranslation("instances")

  return (
    <div className="grid grid-cols-3 gap-3">
      <div className="rounded-lg border p-3">
        <p className="text-xs text-muted-foreground">{t("preferences.deletePlanPreview.target")}</p>
        <p className="font-mono text-sm">{formatBytes(run.targetBytes)}</p>
      </div>
      <div className="rounded-lg border p-3">
        <p className="text-xs text-muted-foreground">{t("preferences.deletePlanPreview.planned")}</p>
        <p className="font-mono text-sm">{formatBytes(run.plannedBytes)}</p>
      </div>
      <div className="rounded-lg border p-3">
        <p className="text-xs text-muted-foreground">{t("preferences.deletePlanPreview.overshoot")}</p>
        <p className="font-mono text-sm">{formatBytes(run.overshootBytes)}</p>
      </div>
    </div>
  )
}

function Totals({ run }: { run: DeletePlanRun }) {
  const { t } = useTranslation("instances")

  return (
    <div className="space-y-3">
      <TotalsGrid run={run} />

      {!run.targetReached && (
        <div role="alert" className="flex items-start gap-2 rounded-md border border-yellow-500/20 bg-yellow-500/10 p-3 text-sm text-yellow-600 dark:text-yellow-400">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>{t("preferences.deletePlanPreview.targetNotReached")}</p>
        </div>
      )}
    </div>
  )
}

function ChainSummary({ keys }: { keys: KeyRule[] }) {
  const { t } = useTranslation("instances")
  const { t: tAutomations } = useTranslation("automations")
  const enabled = keys.filter((key) => key.enabled)

  if (enabled.length === 0) {
    return null
  }

  return (
    <p className="text-xs text-muted-foreground">
      {t("preferences.deletePlanPreview.ranking")}:{" "}
      {enabled
        .map((key) => {
          const direction = key.direction === "ASC"
            ? t("preferences.deletePlanWizard.directionAsc")
            : t("preferences.deletePlanWizard.directionDesc")
          return `${getFieldLabel(key.field, tAutomations)} ${direction}`
        })
        .join(" · ")}
    </p>
  )
}

function GroupsTable({ groups }: { groups: DeletePlanRow[] }) {
  const { t } = useTranslation("instances")
  const { t: tTorrents } = useTranslation("torrents")
  const { t: tAutomations } = useTranslation("automations")
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  const toggle = (id: number) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  if (groups.length === 0) {
    return <p className="p-6 text-center text-sm text-muted-foreground">{t("preferences.deletePlanPreview.noGroups")}</p>
  }

  return (
    <table className="w-full min-w-[560px] text-sm">
      <thead className="sticky top-0">
        <tr className="border-b">
          <th className="w-8 p-2 bg-muted" />
          <th className="text-left p-2 font-medium bg-muted">{t("preferences.deletePlanPreview.contentPath")}</th>
          <th className="text-right p-2 font-medium bg-muted">{t("preferences.deletePlanPreview.size")}</th>
          <th className="text-right p-2 font-medium bg-muted">{t("preferences.deletePlanPreview.membersColumn")}</th>
          <th className="text-left p-2 font-medium bg-muted">{t("preferences.deletePlanPreview.decidingKey")}</th>
        </tr>
      </thead>
      <tbody>
        {groups.map((group) => {
          const isExpanded = expanded.has(group.id)
          return (
            <Fragment key={group.id}>
              <tr className="border-b last:border-0 hover:bg-muted/30">
                <td className="p-2">
                  <button
                    type="button"
                    aria-expanded={isExpanded}
                    aria-label={t("preferences.deletePlanPreview.membersColumn")}
                    onClick={() => toggle(group.id)}
                    className="flex h-5 w-5 items-center justify-center rounded hover:bg-muted"
                  >
                    <ChevronRight className={`h-3.5 w-3.5 transition-transform ${isExpanded ? "rotate-90" : ""}`} />
                  </button>
                </td>
                <td className="p-2 max-w-0 w-full">
                  <PathCell path={group.contentPath} />
                </td>
                <td className="p-2 text-right font-mono whitespace-nowrap">{formatBytes(group.sizeBytes)}</td>
                <td className="p-2 text-right text-muted-foreground whitespace-nowrap">{group.memberCount}</td>
                <td className="p-2 text-muted-foreground whitespace-nowrap">
                  {group.decidingKey ? getFieldLabel(group.decidingKey, tAutomations) : ""}
                </td>
              </tr>
              {isExpanded && (
                <tr className="border-b last:border-0">
                  <td colSpan={5} className="bg-muted/20 p-0">
                    <table className="w-full text-xs">
                      <thead>
                        <tr>
                          <th className="text-left p-2 font-medium text-muted-foreground">{t("preferences.deletePlanPreview.memberName")}</th>
                          <th className="text-right p-2 font-medium text-muted-foreground">{t("preferences.deletePlanPreview.size")}</th>
                          <th className="text-left p-2 font-medium text-muted-foreground">{t("preferences.deletePlanPreview.memberState")}</th>
                          <th className="text-right p-2 font-medium text-muted-foreground">{t("preferences.deletePlanPreview.memberProgress")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {group.members.map((member) => (
                          <tr key={member.hash}>
                            <td className="p-2 break-all">{member.name}</td>
                            <td className="p-2 text-right font-mono whitespace-nowrap">{formatBytes(member.sizeBytes)}</td>
                            <td className="p-2">{getStateLabel(member.state, tTorrents)}</td>
                            <td className="p-2 text-right font-mono whitespace-nowrap">{`${Math.round(member.progress * 100)}%`}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </td>
                </tr>
              )}
            </Fragment>
          )
        })}
      </tbody>
    </table>
  )
}

function ExcludedTable({ groups }: { groups: DeletePlanRow[] }) {
  const { t } = useTranslation("instances")

  return (
    <table className="w-full min-w-[560px] text-sm">
      <thead className="sticky top-0">
        <tr className="border-b">
          <th className="text-left p-2 font-medium bg-muted">{t("preferences.deletePlanPreview.contentPath")}</th>
          <th className="text-right p-2 font-medium bg-muted">{t("preferences.deletePlanPreview.size")}</th>
          <th className="text-left p-2 font-medium bg-muted">{t("preferences.deletePlanPreview.reason")}</th>
        </tr>
      </thead>
      <tbody>
        {groups.map((group) => (
          <tr key={group.id} className="border-b last:border-0">
            <td className="p-2 max-w-0 w-full">
              <PathCell path={group.contentPath} />
            </td>
            <td className="p-2 text-right font-mono whitespace-nowrap">{formatBytes(group.sizeBytes)}</td>
            <td className="p-2 text-muted-foreground">
              {t([deletePlanExcludeReasonKey(group.excludeReason), "preferences.deletePlan.excludeReasons.unknown"])}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

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
  const runQuery = useDeletePlanRun(instanceId, runId, { enabled: open })
  const run = runQuery.data

  const keyDisabledFields = useMemo(
    () => NON_SORTABLE_KEY_FIELDS.map((field) => ({ field, reason: t("preferences.deletePlanWizard.notSortableField") })),
    [t]
  )

  const rankedGroups = run ? run.groups.filter((group) => !group.dead) : []
  const deadGroups = run ? run.groups.filter((group) => group.dead) : []

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

    // One active run per instance: rebuilding over the run this wizard opened must
    // cancel it first, or the create answers 409.
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

  const exportRows: CsvColumn<DeletePlanRow>[] = [
    { header: "Content path", accessor: (row) => row.contentPath },
    { header: "Size", accessor: (row) => formatBytes(row.sizeBytes) },
    { header: "Size (bytes)", accessor: (row) => row.sizeBytes },
    { header: "Members", accessor: (row) => row.memberCount },
    { header: "Status", accessor: (row) => (row.excluded ? "excluded" : "planned") },
    { header: "Reason", accessor: (row) => (row.excludeReason ? t(deletePlanExcludeReasonKey(row.excludeReason)) : "") },
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
          <div className="flex-1 min-h-0 overflow-y-auto space-y-4 py-2">
            <div className="space-y-1.5">
              <Label>{t("preferences.deletePlanWizard.instance")}</Label>
              <p className="text-sm font-medium wrap-anywhere">{instanceName ?? instanceId}</p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="delete-plan-target">{t("preferences.deletePlanWizard.targetLabel")}</Label>
              <div className="flex items-center gap-2">
                <Input
                  id="delete-plan-target"
                  type="number"
                  min="1"
                  inputMode="numeric"
                  value={targetValue}
                  onChange={(e) => setTargetValue(e.target.value)}
                  placeholder={t("preferences.deletePlanWizard.targetPlaceholder")}
                  className="max-w-[12rem]"
                />
                <Select value={targetUnit} onValueChange={(value) => setTargetUnit(value as ByteUnit)}>
                  <SelectTrigger className="w-[7rem]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {TARGET_UNITS.map((unit) => (
                      <SelectItem key={unit} value={unit}>
                        {unitLabel(unit)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {(targetInvalid || conflict) && (
              <div role="alert" className="rounded-md border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive">
                {conflict
                  ? t("preferences.deletePlanWizard.conflict")
                  : t("preferences.deletePlanWizard.targetRequired")}
              </div>
            )}

            <div className="space-y-2">
              <div className="space-y-1">
                <Label>{t("preferences.deletePlanWizard.keysTitle")}</Label>
                <p className="text-xs text-muted-foreground">{t("preferences.deletePlanWizard.keysDescription")}</p>
              </div>
              {chain.map((key, index) => (
                <div key={index} className="flex flex-wrap items-center gap-2 rounded-md border p-2">
                  <Switch
                    checked={key.enabled}
                    onCheckedChange={(checked) => updateChainKey(index, { enabled: checked })}
                    aria-label={t("preferences.deletePlanWizard.enableKey")}
                  />
                  <FieldCombobox
                    value={key.field}
                    onChange={(value) => updateChainKey(index, { field: value as ConditionField })}
                    disabledFields={keyDisabledFields}
                  />
                  <div className="flex items-center rounded-md border">
                    <Button
                      type="button"
                      variant={key.direction === "ASC" ? "secondary" : "ghost"}
                      size="sm"
                      className="h-8 rounded-r-none px-2"
                      onClick={() => updateChainKey(index, { direction: "ASC" })}
                    >
                      <ArrowUp className="mr-1 h-3.5 w-3.5" />
                      {t("preferences.deletePlanWizard.directionAsc")}
                    </Button>
                    <div className="h-4 w-px bg-border" />
                    <Button
                      type="button"
                      variant={key.direction === "DESC" ? "secondary" : "ghost"}
                      size="sm"
                      className="h-8 rounded-l-none px-2"
                      onClick={() => updateChainKey(index, { direction: "DESC" })}
                    >
                      <ArrowDown className="mr-1 h-3.5 w-3.5" />
                      {t("preferences.deletePlanWizard.directionDesc")}
                    </Button>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="ml-auto h-8 w-8"
                    disabled={index === 0}
                    aria-label={t("preferences.deletePlanWizard.moveKeyUp")}
                    onClick={() => setChain((prev) => moveKey(prev, index, -1))}
                  >
                    <ChevronUp className="h-4 w-4" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    disabled={index === chain.length - 1}
                    aria-label={t("preferences.deletePlanWizard.moveKeyDown")}
                    onClick={() => setChain((prev) => moveKey(prev, index, 1))}
                  >
                    <ChevronDown className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>

            <div className="flex items-start gap-2 rounded-md border p-3">
              <Switch
                id="delete-plan-remove-dead"
                checked={removeDeadGroups}
                onCheckedChange={setRemoveDeadGroups}
              />
              <div className="space-y-1">
                <Label htmlFor="delete-plan-remove-dead">{t("preferences.deletePlanWizard.removeDeadGroups")}</Label>
                <p className="text-xs text-muted-foreground">{t("preferences.deletePlanWizard.removeDeadGroupsDescription")}</p>
              </div>
            </div>
          </div>
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
                <Totals run={run} />
                <ChainSummary keys={run.keys} />
                <p className="text-xs text-muted-foreground">
                  {t("preferences.deletePlanPreview.includedGroups", { count: rankedGroups.length })}
                  {" · "}
                  {t("preferences.deletePlanPreview.excludedGroups", { count: run.excludedGroups.length })}
                </p>
                <div className="rounded-lg border overflow-hidden">
                  <div className="overflow-auto max-h-[40vh]">
                    <GroupsTable groups={rankedGroups} />
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
                        <GroupsTable groups={deadGroups} />
                      </div>
                    </div>
                  </div>
                )}
                {run.excludedGroups.length > 0 && (
                  <div className="rounded-lg border overflow-hidden">
                    <div className="overflow-auto max-h-[30vh]">
                      <ExcludedTable groups={run.excludedGroups} />
                    </div>
                  </div>
                )}
              </>
            )}

            {run && run.status !== "preview_ready" && !isWaiting && !isPlanning && (
              <div className="space-y-2">
                <Badge variant="outline">{t(deletePlanStatusLabelKey(run.status))}</Badge>
                {run.errorMessage && (
                  <p className="whitespace-pre-wrap break-all text-sm text-destructive">{run.errorMessage}</p>
                )}
              </div>
            )}
          </div>
        )}

        {step === "confirm" && run && (
          <div className="space-y-4 py-2">
            <TotalsGrid run={run} />
            <div className="rounded-md border border-yellow-500/20 bg-yellow-500/10 p-3 text-sm text-yellow-600 dark:text-yellow-400">
              <p className="font-medium">{t("preferences.deletePlanConfirm.title")}</p>
              <p>{t("preferences.deletePlanConfirm.description")}</p>
              <p className="mt-1">{t("preferences.deletePlanConfirm.otherInstances")}</p>
            </div>
          </div>
        )}

        <DialogFooter className="mt-4 sm:justify-between">
          <div className="flex gap-2">
            {step !== "target" && (
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

            {step === "confirm" && (
              <Button type="button" variant="destructive" disabled>
                {t("preferences.deletePlanConfirm.confirm")}
              </Button>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
