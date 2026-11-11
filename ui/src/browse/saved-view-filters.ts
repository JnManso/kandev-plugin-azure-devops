import type { AzureDevOpsFiltersState } from "./filters";
import type { AzureDevOpsSavedView } from "./types";

/** The team and sprint filters a saved work-item view records. */
export function workItemViewScope(filters: AzureDevOpsFiltersState): Pick<AzureDevOpsSavedView, "teamId" | "iterationId"> {
  return {
    ...(filters.teamId ? { teamId: filters.teamId } : {}),
    ...(filters.iterationId ? { iterationId: filters.iterationId } : {}),
  };
}

/** The filters a saved view runs with; its team and sprint replace the current ones. */
export function filtersFromSavedView(
  current: AzureDevOpsFiltersState,
  view: AzureDevOpsSavedView,
): AzureDevOpsFiltersState {
  return {
    ...current,
    projectId: view.projectId,
    repositoryId: view.repositoryId ?? "",
    wiql: view.wiql ?? current.wiql,
    text: "",
    top: view.top ?? 50,
    status: view.status ?? "active",
    creator: view.creator ?? "",
    reviewer: view.reviewer ?? "",
    teamId: view.teamId ?? "",
    iterationId: view.iterationId ?? "",
  };
}
