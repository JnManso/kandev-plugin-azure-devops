
import { IntegrationScopeBar } from "../host-ui";

export type ScopeSelection<K extends string> = { kind: K; source: "preset" | "saved"; id: string };
import type { AzureDevOpsSavedView } from "./types";
import { useT } from "../i18n";
import {
  presetsForKind,
  type AzureDevOpsPresetKind,
  type AzureDevOpsQueryPresets,
} from "./presets";

export type AzureDevOpsScopeSelection = ScopeSelection<AzureDevOpsPresetKind>;

export function AzureDevOpsScopeBar({
  selected,
  onSelect,
  savedViews,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
  queryPresets,
}: {
  selected: AzureDevOpsScopeSelection;
  onSelect: (selection: AzureDevOpsScopeSelection) => void;
  savedViews: AzureDevOpsSavedView[];
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  queryPresets: AzureDevOpsQueryPresets;
}) {
  const t = useT();
  const kinds = [
    { value: "work_item" as const, label: t("workItems") },
    { value: "pull_request" as const, label: t("pullRequests") },
  ];
  return (
    <IntegrationScopeBar
      testId="azure-devops-presets-scope-bar"
      savedMenuTestId="azure-devops-saved-views-menu"
      kinds={kinds}
      selected={selected}
      onSelect={onSelect}
      presetsByKind={(kind: AzureDevOpsPresetKind) => presetsForKind(kind, queryPresets, t)}
      savedPresets={savedViews}
      onDeleteSaved={onDeleteSaved}
      canSaveCurrent={canSaveCurrent}
      onSaveCurrent={onSaveCurrent}
      className="border-b"
    />
  );
}
