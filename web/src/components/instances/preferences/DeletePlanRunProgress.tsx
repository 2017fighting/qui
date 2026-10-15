/*
 * Copyright (c) 2025-2026, s0up and the autobrr contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Progress } from "@/components/ui/progress"
import { formatBytes } from "@/lib/utils"
import type { DeletePlanRunStatus } from "@/types"
import { Loader2 } from "lucide-react"
import { useTranslation } from "react-i18next"

// The fields the progress and result views read, so the card (a run summary) and
// the wizard (a full run) share one component.
export interface DeletePlanRunProgressRun {
  status: DeletePlanRunStatus
  removedBytes: number
  plannedBytes: number
  errorMessage: string
  warningMessage: string
}

// DeletePlanRunProgress shows a run that is deleting or finished. It renders
// nothing for a run that has not started deleting yet.
export function DeletePlanRunProgress({ run }: { run: DeletePlanRunProgressRun }) {
  const { t } = useTranslation("instances")

  if (run.status === "deleting") {
    const percent = run.plannedBytes > 0
      ? Math.min(100, (run.removedBytes / run.plannedBytes) * 100)
      : 0
    return (
      <div className="space-y-1.5">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>{t("preferences.deletePlanOverview.deleting")}</span>
        </div>
        <Progress value={percent} />
        <p className="text-xs text-muted-foreground">
          {t("preferences.deletePlanOverview.progress", {
            removed: formatBytes(run.removedBytes),
            planned: formatBytes(run.plannedBytes),
          })}
        </p>
      </div>
    )
  }

  if (run.status === "completed") {
    return (
      <div className="space-y-1">
        <p className="text-sm">
          {t("preferences.deletePlanOverview.completedResult", {
            removed: formatBytes(run.removedBytes),
          })}
        </p>
        {run.warningMessage && (
          <p className="text-xs text-yellow-600 break-words dark:text-yellow-400">{run.warningMessage}</p>
        )}
      </div>
    )
  }

  if (run.status === "failed") {
    return (
      <div className="space-y-1">
        <p className="text-sm text-destructive">{t("preferences.deletePlanOverview.failedResult")}</p>
        {run.errorMessage && (
          <p className="text-xs text-destructive break-words">{run.errorMessage}</p>
        )}
      </div>
    )
  }

  if (run.status === "canceled") {
    return <p className="text-sm text-muted-foreground">{t("preferences.deletePlanOverview.canceledResult")}</p>
  }

  return null
}
