/**
 * The built-in integration's REST helper signatures, served by the plugin's
 * browse actions, so the ported hooks keep their call sites.
 */
import { currentHost } from "../runtime";
import { createConnectionApi } from "../connection-model";
import { createBrowseApi } from "./api";
import { createTaskLinksApi } from "../tasks/api";
import type {
  AzureDevOpsBoardReference,
  AzureDevOpsConfig,
  AzureDevOpsBoardSnapshot,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsBoardWorkItemUpdate,
  AzureDevOpsProject,
  AzureDevOpsPullRequestFeedback,
  AzureDevOpsPullRequestPage,
  AzureDevOpsRepository,
  AzureDevOpsSavedView,
  AzureDevOpsTaskWorkItem,
  AzureDevOpsTeam,
  AzureDevOpsWorkItem,
  AzureDevOpsWorkItemAssignmentUpdate,
  AzureDevOpsWorkItemCommentPage,
  AzureDevOpsWorkItemDetail,
  AzureDevOpsWorkItemSearchResult,
  AzureDevOpsWorkspaceSettings,
} from "./types";

export type ApiRequestOptions = { cache?: string; init?: { signal?: AbortSignal | null } };

const api = (workspaceId: string) => createBrowseApi(currentHost(), workspaceId);
const signalOf = (options?: ApiRequestOptions) => options?.init?.signal ?? undefined;

export async function getAzureDevOpsConfig(workspaceId: string, options?: ApiRequestOptions): Promise<AzureDevOpsConfig | null> {
  const connection = await createConnectionApi(currentHost(), workspaceId).get(signalOf(options));
  if (!connection) return null;
  return {
    workspaceId,
    organizationUrl: connection.organizationUrl ?? "",
    defaultProjectId: connection.defaultProjectId,
    defaultProjectName: connection.defaultProjectName,
    hasSecret: connection.hasSecret && !connection.reauthRequired,
  };
}

export async function listAzureDevOpsProjects(workspaceId: string, options?: ApiRequestOptions): Promise<{ projects: AzureDevOpsProject[] }> {
  return { projects: await api(workspaceId).projects(signalOf(options)) };
}

export async function listAzureDevOpsRepositories(
  workspaceId: string,
  project: string,
  options?: ApiRequestOptions,
): Promise<{ repositories: AzureDevOpsRepository[] }> {
  return { repositories: await api(workspaceId).repositories(project, signalOf(options)) };
}

export async function getAzureDevOpsSavedViews(workspaceId: string, options?: ApiRequestOptions): Promise<{ views: AzureDevOpsSavedView[] }> {
  return { views: await api(workspaceId).views(signalOf(options)) };
}

export async function setAzureDevOpsSavedViews(workspaceId: string, views: AzureDevOpsSavedView[]): Promise<{ views: AzureDevOpsSavedView[] }> {
  return { views: await api(workspaceId).saveViews(views) };
}

export function getAzureDevOpsWorkspaceSettings(workspaceId: string, options?: ApiRequestOptions): Promise<AzureDevOpsWorkspaceSettings> {
  return api(workspaceId).settings(signalOf(options));
}

export function updateAzureDevOpsWorkspaceSettings(
  workspaceId: string,
  payload: Partial<Record<keyof AzureDevOpsWorkspaceSettings, unknown>>,
): Promise<AzureDevOpsWorkspaceSettings> {
  return api(workspaceId).patchSettings(payload);
}

export function searchAzureDevOpsWorkItems(
  workspaceId: string,
  payload: { project: string; wiql: string; top?: number },
  options?: ApiRequestOptions,
): Promise<AzureDevOpsWorkItemSearchResult> {
  return api(workspaceId).searchWorkItems({ projectId: payload.project, wiql: payload.wiql, top: payload.top ?? 50 }, signalOf(options));
}

export function getAzureDevOpsWorkItemDetail(
  workspaceId: string,
  project: string,
  id: number,
  options?: ApiRequestOptions,
): Promise<AzureDevOpsWorkItemDetail> {
  return api(workspaceId).workItem(project, id, signalOf(options));
}

export function listAzureDevOpsWorkItemComments(
  workspaceId: string,
  project: string,
  id: number,
  continuationToken?: string,
  options?: ApiRequestOptions,
): Promise<AzureDevOpsWorkItemCommentPage> {
  return api(workspaceId).comments(project, id, continuationToken ?? "", signalOf(options));
}

export function updateAzureDevOpsWorkItemAssignment(
  workspaceId: string,
  project: string,
  id: number,
  payload: AzureDevOpsWorkItemAssignmentUpdate,
  _options?: ApiRequestOptions,
): Promise<AzureDevOpsWorkItem> {
  return api(workspaceId).updateWorkItem(project, id, payload);
}

export async function listAzureDevOpsTeams(workspaceId: string, project: string, options?: ApiRequestOptions): Promise<{ teams: AzureDevOpsTeam[]; mineIds: string[] }> {
  return api(workspaceId).teams(project, signalOf(options));
}

export async function listAzureDevOpsBoards(
  workspaceId: string,
  project: string,
  team: string,
  options?: ApiRequestOptions,
): Promise<{ boards: AzureDevOpsBoardReference[] }> {
  return { boards: await api(workspaceId).boards(project, team, signalOf(options)) };
}

export function getAzureDevOpsBoardSnapshot(
  workspaceId: string,
  project: string,
  team: string,
  board: string,
  options?: ApiRequestOptions,
): Promise<AzureDevOpsBoardSnapshot> {
  return api(workspaceId).board(project, team, board, signalOf(options));
}

export function updateAzureDevOpsBoardWorkItem(
  workspaceId: string,
  project: string,
  team: string,
  board: string,
  id: number,
  payload: AzureDevOpsBoardWorkItemUpdate,
): Promise<AzureDevOpsBoardWorkItem> {
  return api(workspaceId).updateBoardItem({ projectId: project, teamId: team, boardId: board }, id, payload);
}

export type AzureDevOpsPullRequestFilters = {
  project: string;
  repository: string;
  status?: string;
  creator?: string;
  reviewer?: string;
  skip?: number;
  top?: number;
};

/** Encodes the backend's opaque page cursor for an offset. */
export function skipCursor(skip: number): string {
  if (skip <= 0) return "";
  return btoa(`skip:${skip}`).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export async function listAzureDevOpsPullRequests(
  workspaceId: string,
  filters: AzureDevOpsPullRequestFilters,
  options?: ApiRequestOptions,
): Promise<AzureDevOpsPullRequestPage> {
  const skip = filters.skip ?? 0;
  const top = filters.top ?? 25;
  const page = await api(workspaceId).pullRequests(
    {
      projectId: filters.project,
      repositoryId: filters.repository,
      status: filters.status,
      creator: filters.creator,
      reviewer: filters.reviewer,
      cursor: skipCursor(skip),
      top,
    },
    signalOf(options),
  );
  return { items: page.items, count: page.items.length, skip, top };
}

export function getAzureDevOpsPullRequestFeedback(
  workspaceId: string,
  projectId: string,
  repositoryId: string,
  pullRequestId: number,
  options?: ApiRequestOptions,
): Promise<AzureDevOpsPullRequestFeedback> {
  return api(workspaceId).feedback(projectId, repositoryId, pullRequestId, signalOf(options));
}

/** Kandev tasks linked to work items, grouped by task; title is the task title. */
export async function listWorkspaceAzureDevOpsTaskWorkItems(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<{ taskWorkItems: Record<string, AzureDevOpsTaskWorkItem[]> }> {
  const links = await createTaskLinksApi(currentHost()).workspace(workspaceId, signalOf(options));
  const taskWorkItems: Record<string, AzureDevOpsTaskWorkItem[]> = {};
  for (const [taskId, items] of Object.entries(links.workItems)) {
    const taskTitle = links.tasks[taskId]?.title;
    taskWorkItems[taskId] = items.map((item) => ({
      id: `${taskId}:${item.workItemId}`,
      taskId,
      workspaceId,
      projectId: item.projectId,
      workItemId: item.workItemId,
      workItemUrl: item.url ?? "",
      title: taskTitle || item.title || "",
      state: item.state ?? "",
      type: item.type ?? "",
      createdAt: item.linkedAt ?? "",
      updatedAt: item.linkedAt ?? "",
    }));
  }
  return { taskWorkItems };
}
