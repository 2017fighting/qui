/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { deletePlanExcludeReasonKey } from "@/components/instances/preferences/deletePlanLabels"
import { getFieldLabel } from "@/components/query-builder/constants"
import { PathCell } from "@/components/ui/path-cell"
import { getStateLabel } from "@/lib/torrent-state-utils"
import { formatBytes } from "@/lib/utils"
import type { DeletePlanRow, DeletePlanRun, KeyRule } from "@/types"
import type { TFunction } from "i18next"
import { AlertTriangle, ChevronRight } from "lucide-react"
import { Fragment, useState } from "react"
import { useTranslation } from "react-i18next"

// Read-only rendering of a delete plan run. The wizard preview and the history
// detail both mount these, so the two views cannot disagree about a run.

export interface DeletePlanStat {
  label: string
  value: string
}

export function DeletePlanStatGrid({ stats }: { stats: DeletePlanStat[] }) {
  return (
    <div className="grid grid-cols-3 gap-3">
      {stats.map((stat) => (
        <div key={stat.label} className="rounded-lg border p-3">
          <p className="text-xs text-muted-foreground">{stat.label}</p>
          <p className="font-mono text-sm">{stat.value}</p>
        </div>
      ))}
    </div>
  )
}

export function DeletePlanTotalsGrid({ run }: { run: DeletePlanRun }) {
  const { t } = useTranslation("instances")

  return (
    <DeletePlanStatGrid
      stats={[
        { label: t("preferences.deletePlanPreview.target"), value: formatBytes(run.targetBytes) },
        { label: t("preferences.deletePlanPreview.planned"), value: formatBytes(run.plannedBytes) },
        { label: t("preferences.deletePlanPreview.overshoot"), value: formatBytes(run.overshootBytes) },
      ]}
    />
  )
}

export function DeletePlanTotals({ run }: { run: DeletePlanRun }) {
  const { t } = useTranslation("instances")

  return (
    <div className="space-y-3">
      <DeletePlanTotalsGrid run={run} />

      {!run.targetReached && (
        <div role="alert" className="flex items-start gap-2 rounded-md border border-yellow-500/20 bg-yellow-500/10 p-3 text-sm text-yellow-600 dark:text-yellow-400">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>{t("preferences.deletePlanPreview.targetNotReached")}</p>
        </div>
      )}
    </div>
  )
}

function keyLabel(key: KeyRule, t: TFunction, tAutomations: TFunction): string {
  const direction = key.direction === "ASC"
    ? t("preferences.deletePlanWizard.directionAsc")
    : t("preferences.deletePlanWizard.directionDesc")
  return `${getFieldLabel(key.field, tAutomations)} ${direction}`
}

export function DeletePlanChainSummary({ keys }: { keys: KeyRule[] }) {
  const { t } = useTranslation("instances")
  const { t: tAutomations } = useTranslation("automations")
  const enabled = keys.filter((key) => key.enabled)

  if (enabled.length === 0) {
    return null
  }

  return (
    <p className="text-xs text-muted-foreground">
      {t("preferences.deletePlanPreview.ranking")}:{" "}
      {enabled.map((key) => keyLabel(key, t, tAutomations)).join(" · ")}
    </p>
  )
}

// DeletePlanConfiguration shows the chain and the dead-groups switch the run
// recorded, including the keys that were switched off. The history detail reads
// it back; the wizard preview shows only the ranking that decided the order.
export function DeletePlanConfiguration({ keys, removeDeadGroups }: { keys: KeyRule[]; removeDeadGroups: boolean }) {
  const { t } = useTranslation("instances")
  const { t: tAutomations } = useTranslation("automations")

  const chain = keys.length === 0
    ? t("preferences.deletePlanHistory.noKeys")
    : keys
      .map((key) => {
        const label = keyLabel(key, t, tAutomations)
        return key.enabled ? label : t("preferences.deletePlanHistory.disabledKey", { key: label })
      })
      .join(" · ")

  return (
    <div className="space-y-1 rounded-lg border p-3 text-xs text-muted-foreground">
      <p>
        {t("preferences.deletePlanWizard.keysTitle")}:{" "}
        {chain}
      </p>
      <p>
        {t("preferences.deletePlanWizard.removeDeadGroups")}:{" "}
        {removeDeadGroups ? t("preferences.deletePlanHistory.enabled") : t("preferences.deletePlanHistory.disabled")}
      </p>
    </div>
  )
}

export function DeletePlanGroupsTable({ groups }: { groups: DeletePlanRow[] }) {
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

export function DeletePlanExcludedTable({ groups }: { groups: DeletePlanRow[] }) {
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
