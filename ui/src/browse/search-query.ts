import type { AzureDevOpsBrowseMode, AzureDevOpsFiltersState } from "./filters";

/** The pull_requests.list query for the current filters. */
export function pullRequestSearchQuery(filters: AzureDevOpsFiltersState, skip: number, top: number) {
  return {
    project: filters.projectId,
    repository: filters.repositoryId,
    // "all" is sent as is: Azure DevOps returns only active pull requests
    // when the status is omitted.
    status: filters.status,
    creator: filters.creator || undefined,
    reviewer: filters.reviewer || undefined,
    skip,
    top,
  };
}

/**
 * The repository to select for a project, or undefined to keep the current
 * one. The list can still hold the previous project's repositories right
 * after a project change, so only repositories of this project count.
 */
export function repositoryForProject(
  projectId: string,
  repositoryId: string,
  repositories: Array<{ id: string; projectId: string }>,
): string | undefined {
  const own = repositories.filter((repository) => repository.projectId === projectId);
  if (own.length === 0 || own.some((repository) => repository.id === repositoryId)) return undefined;
  return own[0].id;
}

const WORK_ITEM_SEARCH_BASE =
  "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND ";

/**
 * The WIQL for the work-item search box: a number finds that work item,
 * other text finds titles containing every word, in any order. An empty box
 * keeps the WIQL query.
 */
export function workItemSearchWiql(text: string, wiql: string): string {
  const query = text.trim();
  if (!query) return wiql;
  const id = /^#?(\d+)$/.exec(query)?.[1];
  if (id) return `${WORK_ITEM_SEARCH_BASE}[System.Id] = ${Number(id)}`;
  const words = query
    .split(/\s+/)
    .map((word) => `[System.Title] CONTAINS '${word.replaceAll("'", "''")}'`)
    .join(" AND ");
  return `${WORK_ITEM_SEARCH_BASE}${words} ORDER BY [System.ChangedDate] DESC`;
}

/** Whether the current filters can run a search in this mode. */
export function canSearch(mode: AzureDevOpsBrowseMode, filters: AzureDevOpsFiltersState): boolean {
  if (mode === "board" || mode === "sprints" || !filters.projectId) return false;
  if (mode === "work-items") return Boolean(filters.text.trim() || filters.wiql.trim());
  return Boolean(filters.repositoryId);
}

/**
 * The value after a picker selection. Projects, teams, boards, and
 * repositories always need a value, so picking the selected option again
 * (which the host combobox reports as "") keeps it.
 */
export function pickedValue(current: string, next: string): string {
  return next || current;
}

/**
 * Picker filter that keeps options whose name contains the typed text,
 * ignoring case. Fuzzy matching would keep most of a few hundred team names
 * for a short query.
 */
export function pickerFilter(_value: string, search: string, keywords: string[] = []): number {
  const query = search.trim().toLowerCase();
  if (!query) return 1;
  return keywords.some((keyword) => keyword.toLowerCase().includes(query)) ? 1 : 0;
}
