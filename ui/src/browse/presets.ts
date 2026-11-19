import {
  IconActivity,
  IconCalendarClock,
  IconChecks,
  IconGitPullRequest,
  IconInbox,
  IconSearch,
  IconUser,
} from "../icons";
import type { AzureDevOpsQueryPreset } from "./types";
import type { AzureDevOpsFiltersState } from "./filters";
import {
  DEFAULT_AZURE_PULL_REQUEST_QUERIES,
  DEFAULT_AZURE_WORK_ITEM_QUERIES,
  azureQueryDisplayLabel,
} from "./workspace-defaults";

export type AzureDevOpsPresetKind = "work_item" | "pull_request";

export type AzureDevOpsPreset = {
  value: string;
  label: string;
  icon: typeof IconInbox;
  group: "inbox" | "created";
  filters: Partial<AzureDevOpsFiltersState>;
};

const ICONS = {
  recent: IconCalendarClock,
  assigned: IconInbox,
  active: IconActivity,
  "review-requested": IconInbox,
  completed: IconChecks,
  created: IconUser,
};

function stringFilter(filters: Record<string, unknown>, key: string, fallback = ""): string {
  return typeof filters[key] === "string" ? filters[key] : fallback;
}

function filtersForQuery(
  kind: AzureDevOpsPresetKind,
  filters: Record<string, unknown>,
): Partial<AzureDevOpsFiltersState> {
  if (kind === "work_item") {
    const top = typeof filters.top === "number" && filters.top > 0 ? filters.top : 50;
    return { wiql: stringFilter(filters, "wiql"), top };
  }
  return {
    status: stringFilter(filters, "status", "active"),
    creator: stringFilter(filters, "creator"),
    reviewer: stringFilter(filters, "reviewer"),
  };
}

function toBrowsePreset(
  kind: AzureDevOpsPresetKind,
  preset: AzureDevOpsQueryPreset,
): AzureDevOpsPreset {
  return {
    value: preset.id,
    label: preset.label,
    icon:
      ICONS[preset.id as keyof typeof ICONS] ??
      (kind === "pull_request" ? IconGitPullRequest : IconSearch),
    group: preset.group === "created" ? "created" : "inbox",
    filters: filtersForQuery(kind, preset.filters),
  };
}

export type AzureDevOpsQueryPresets = {
  workItems: AzureDevOpsQueryPreset[];
  pullRequests: AzureDevOpsQueryPreset[];
};

export function presetsForKind(
  kind: AzureDevOpsPresetKind,
  configured?: AzureDevOpsQueryPresets,
  translate?: (key: string) => string,
): AzureDevOpsPreset[] {
  const defaults =
    kind === "work_item" ? DEFAULT_AZURE_WORK_ITEM_QUERIES : DEFAULT_AZURE_PULL_REQUEST_QUERIES;
  const candidates = kind === "work_item" ? configured?.workItems : configured?.pullRequests;
  return (candidates?.length ? candidates : defaults).map((preset) => {
    const result = toBrowsePreset(kind, preset);
    return translate
      ? { ...result, label: azureQueryDisplayLabel(kind, preset, translate) }
      : result;
  });
}

export const AZURE_WORK_ITEM_PRESETS = presetsForKind("work_item");
export const AZURE_PULL_REQUEST_PRESETS = presetsForKind("pull_request");

/**
 * The shortcut whose filters the current filters carry, so the scope bar
 * shows it selected; "" when the search text or any filter differs.
 */
export function matchingPresetId(
  presets: AzureDevOpsPreset[],
  filters: Partial<AzureDevOpsFiltersState>,
): string {
  if (filters.text?.trim()) return "";
  const match = presets.find((preset) =>
    Object.entries(preset.filters).every(
      ([key, value]) => filters[key as keyof AzureDevOpsFiltersState] === value,
    ),
  );
  return match?.value ?? "";
}

/** How many pull-request filters under Advanced are set. */
export function advancedFilterCount(filters: { creator: string; reviewer: string }): number {
  return [filters.creator, filters.reviewer].filter((value) => value.trim()).length;
}
