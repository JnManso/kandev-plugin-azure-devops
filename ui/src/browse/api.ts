import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import type {
  AzureDevOpsBoardChildren,
  AzureDevOpsBoardReference,
  AzureDevOpsBoardSnapshot,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsBoardWorkItemUpdate,
  AzureDevOpsIdentity,
  AzureDevOpsProject,
  AzureDevOpsPullRequest,
  AzureDevOpsPullRequestFeedback,
  AzureDevOpsPullRequestListPage,
  AzureDevOpsPullRequestPeople,
  AzureDevOpsRepository,
  AzureDevOpsSavedView,
  AzureDevOpsSprintBoard,
  AzureDevOpsTeamList,
  AzureDevOpsTeamSettings,
  AzureDevOpsWorkItem,
  AzureDevOpsWorkItemAssignmentUpdate,
  AzureDevOpsWorkItemCommentPage,
  AzureDevOpsWorkItemDetail,
  AzureDevOpsWorkItemSearchResult,
  AzureDevOpsWorkspaceSettings,
} from "./types";

export type PullRequestQuery = {
  projectId: string;
  repositoryId: string;
  status?: string;
  creator?: string;
  reviewer?: string;
  cursor?: string;
  top?: number;
};

/** Typed wrappers over the plugin's workspace-scoped browse actions. */
export function createBrowseApi(host: PluginHostApi, workspaceId: string) {
  const invoke = <T>(key: string, body: unknown = {}, signal?: AbortSignal) =>
    host.api.invokeAction<T>(key, { workspaceId, body }, { signal });
  return {
    projects: async (signal?: AbortSignal) =>
      (await invoke<{ projects?: AzureDevOpsProject[] }>("projects.list", {}, signal)).projects ?? [],
    repositories: async (projectId: string, signal?: AbortSignal) =>
      (await invoke<{ repositories?: AzureDevOpsRepository[] }>("projects.repositories", { projectId }, signal))
        .repositories ?? [],
    teams: async (projectId: string, signal?: AbortSignal): Promise<AzureDevOpsTeamList> => {
      const list = await invoke<Partial<AzureDevOpsTeamList>>("teams.list", { projectId }, signal);
      return { teams: list.teams ?? [], mineIds: list.mineIds ?? [] };
    },
    teamSettings: (projectId: string, teamId: string, signal?: AbortSignal) =>
      invoke<AzureDevOpsTeamSettings>("teams.settings", { projectId, teamId }, signal),
    sprintBoard: (projectId: string, teamId: string, iterationId: string, signal?: AbortSignal) =>
      invoke<AzureDevOpsSprintBoard>("sprints.board", { projectId, teamId, iterationId }, signal),
    boards: async (projectId: string, teamId: string, signal?: AbortSignal) =>
      (await invoke<{ boards?: AzureDevOpsBoardReference[] }>("boards.list", { projectId, teamId }, signal)).boards ?? [],
    board: (projectId: string, teamId: string, boardId: string, signal?: AbortSignal) =>
      invoke<AzureDevOpsBoardSnapshot>("boards.get", { projectId, teamId, boardId }, signal),
    boardChildren: async (projectId: string, ids: number[], signal?: AbortSignal): Promise<AzureDevOpsBoardChildren> => {
      const result = await invoke<Partial<AzureDevOpsBoardChildren>>("boards.children", { projectId, ids }, signal);
      return { children: result.children ?? [], truncated: result.truncated ?? false };
    },
    pullRequestPeople: async (
      projectId: string,
      repositoryId: string,
      signal?: AbortSignal,
    ): Promise<AzureDevOpsPullRequestPeople> => {
      const people = await invoke<Partial<AzureDevOpsPullRequestPeople>>(
        "pull_requests.people",
        { projectId, repositoryId },
        signal,
      );
      return { creators: people.creators ?? [], reviewers: people.reviewers ?? [] };
    },
    updateBoardItem: async (
      location: { projectId: string; teamId: string; boardId: string },
      id: number,
      update: AzureDevOpsBoardWorkItemUpdate,
    ) => (await invoke<{ item: AzureDevOpsBoardWorkItem }>("boards.update_item", { ...location, id, ...update })).item,
    searchWorkItems: (query: { projectId: string; wiql: string; top: number }, signal?: AbortSignal) =>
      invoke<AzureDevOpsWorkItemSearchResult>("work_items.search", query, signal),
    workItem: async (projectId: string, id: number, signal?: AbortSignal) =>
      (await invoke<{ item: AzureDevOpsWorkItemDetail }>("work_items.get", { projectId, id }, signal)).item,
    comments: (projectId: string, id: number, continuationToken = "", signal?: AbortSignal) =>
      invoke<AzureDevOpsWorkItemCommentPage>("work_items.comments", { projectId, id, continuationToken }, signal),
    updateWorkItem: async (projectId: string, id: number, update: AzureDevOpsWorkItemAssignmentUpdate) =>
      (await invoke<{ item: AzureDevOpsWorkItem }>("work_items.update", { projectId, id, ...update })).item,
    pullRequests: (query: PullRequestQuery, signal?: AbortSignal) =>
      invoke<AzureDevOpsPullRequestListPage>("pull_requests.list", query, signal),
    pullRequest: async (projectId: string, repositoryId: string, id: number, signal?: AbortSignal) =>
      (await invoke<{ pullRequest: AzureDevOpsPullRequest }>("pull_requests.get", { projectId, repositoryId, id }, signal))
        .pullRequest,
    feedback: (projectId: string, repositoryId: string, id: number, signal?: AbortSignal) =>
      invoke<AzureDevOpsPullRequestFeedback>("pull_requests.feedback", { projectId, repositoryId, id }, signal),
    identity: async (signal?: AbortSignal) =>
      (await invoke<{ identity: AzureDevOpsIdentity }>("identity.get", {}, signal)).identity,
    views: async (signal?: AbortSignal) =>
      (await invoke<{ views?: AzureDevOpsSavedView[] }>("views.get", {}, signal)).views ?? [],
    saveViews: async (views: AzureDevOpsSavedView[]) =>
      (await invoke<{ views?: AzureDevOpsSavedView[] }>("views.save", { views })).views ?? [],
    settings: (signal?: AbortSignal) => invoke<AzureDevOpsWorkspaceSettings>("workspace_settings.get", {}, signal),
    patchSettings: (patch: Partial<Record<keyof AzureDevOpsWorkspaceSettings, unknown>>) =>
      invoke<AzureDevOpsWorkspaceSettings>("workspace_settings.patch", patch),
  };
}

export type BrowseApi = ReturnType<typeof createBrowseApi>;
