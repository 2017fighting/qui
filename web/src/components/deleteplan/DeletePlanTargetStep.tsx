import { isSortableKeyField, moveKey } from "@/components/deleteplan/deletePlanKeys"
import { FieldCombobox } from "@/components/query-builder/FieldCombobox"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { unitLabel, type ByteUnit } from "@/lib/unit-format"
import type { KeyRule } from "@/types"
import { ArrowDown, ArrowUp, ChevronDown, ChevronUp } from "lucide-react"
import type { Dispatch, SetStateAction } from "react"
import { useTranslation } from "react-i18next"

const TARGET_UNITS: ByteUnit[] = ["MiB", "GiB", "TiB"]

function isTargetUnit(value: string): value is ByteUnit {
  return TARGET_UNITS.some(unit => unit === value)
}

export interface DeletePlanTargetStepProps {
  instanceId: number
  instanceName?: string
  targetValue: string
  setTargetValue: Dispatch<SetStateAction<string>>
  targetUnit: ByteUnit
  setTargetUnit: Dispatch<SetStateAction<ByteUnit>>
  targetInvalid: boolean
  conflict: boolean
  chain: KeyRule[]
  setChain: Dispatch<SetStateAction<KeyRule[]>>
  updateChainKey: (index: number, patch: Partial<KeyRule>) => void
  keyDisabledFields: { field: string; reason: string }[]
  removeDeadGroups: boolean
  setRemoveDeadGroups: Dispatch<SetStateAction<boolean>>
}

export function DeletePlanTargetStep({
  instanceId,
  instanceName,
  targetValue,
  setTargetValue,
  targetUnit,
  setTargetUnit,
  targetInvalid,
  conflict,
  chain,
  setChain,
  updateChainKey,
  keyDisabledFields,
  removeDeadGroups,
  setRemoveDeadGroups,
}: DeletePlanTargetStepProps) {
  const { t } = useTranslation("instances")

  return (
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
          <Select value={targetUnit} onValueChange={(value) => {
            if (isTargetUnit(value)) setTargetUnit(value)
          }}>
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
              onChange={(value) => {
                if (!isSortableKeyField(value)) return
                updateChainKey(index, { field: value })
              }}
              disabledFields={keyDisabledFields}
            />
            <div className="flex items-center gap-1">
              <Button
                type="button"
                variant={key.direction === "ASC" ? "secondary" : "ghost"}
                size="sm"
                onClick={() => updateChainKey(index, { direction: "ASC" })}
              >
                <ArrowUp className="mr-1 h-3.5 w-3.5" />
                {t("preferences.deletePlanWizard.directionAsc")}
              </Button>
              <Button
                type="button"
                variant={key.direction === "DESC" ? "secondary" : "ghost"}
                size="sm"
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
  )
}
