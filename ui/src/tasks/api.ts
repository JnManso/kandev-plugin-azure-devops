import type { PluginHostApi, ReviewTaskStatus } from "@kdlbs/plugin-sdk";

export type PullRequestLink = {
  projectId: string;
  projectName?: string;
  repositoryId: string;
  repositoryName?: string;
  pullRequestId: number;
  title?: string;
  url?: string;
  sourceBranch?: string;
  targetBranch?: string;
  linkedAt?: string;
};

export type PullRequestLinkStatus = PullRequestLink & {
  status?: string;
  isDraft?: boolean;
  taskStatus?: ReviewTaskStatus;
  error?: string;
};

export type WorkItemLink = {
  projectId: string;
  workItemId: number;
  title?: string;
  url?: string;
  state?: string;
  type?: string;
  linkedAt?: string;
};

export type TaskLinks = { pullRequests: PullRequestLinkStatus[]; workItems: WorkItemLink[] };

export type WorkspaceLinks = {
  pullRequests: Record<string, PullRequestLink[]>;
  workItems: Record<string, WorkItemLink[]>;
  /** Titles of the linked tasks the plugin could read. */
  tasks: Record<string, { title: string }>;
};

export type CreatePullRequestResult = {
  url?: string;
  number?: number;
  linked?: boolean;
  associationError?: string;
};

type TaskSelector = { workspaceId?: string; taskId: string };

/** Typed wrappers over the plugin's task link actions. */
export function createTaskLinksApi(host: PluginHostApi) {
  const onTask = <T>(key: string, selector: TaskSelector, body: unknown = {}, signal?: AbortSignal) =>
    host.api.invokeAction<T>(
      key,
      { ...(selector.workspaceId ? { workspaceId: selector.workspaceId } : {}), taskId: selector.taskId, body },
      { signal },
    );
  return {
    async get(selector: TaskSelector, signal?: AbortSignal): Promise<TaskLinks> {
      const response = await onTask<Partial<TaskLinks>>("task_links.get", selector, {}, signal);
      return { pullRequests: response.pullRequests ?? [], workItems: response.workItems ?? [] };
    },
    async workspace(workspaceId: string, signal?: AbortSignal): Promise<WorkspaceLinks> {
      const response = await host.api.invokeAction<Partial<WorkspaceLinks>>(
        "task_links.workspace",
        { workspaceId, body: {} },
        { signal },
      );
      return { pullRequests: response.pullRequests ?? {}, workItems: response.workItems ?? {}, tasks: response.tasks ?? {} };
    },
    linkPullRequest: (selector: TaskSelector, body: Record<string, unknown>, signal?: AbortSignal) =>
      onTask<{ pullRequest?: PullRequestLink }>("pull_requests.link", selector, body, signal),
    unlinkPullRequest: (selector: TaskSelector, repositoryId: string, pullRequestId: number, signal?: AbortSignal) =>
      onTask("pull_requests.unlink", selector, { repositoryId, pullRequestId }, signal),
    linkWorkItem: (selector: TaskSelector, body: Record<string, unknown>, signal?: AbortSignal) =>
      onTask<{ workItem?: WorkItemLink }>("work_items.link", selector, body, signal),
    unlinkWorkItem: (selector: TaskSelector, workItemId: number, signal?: AbortSignal) =>
      onTask("work_items.unlink", selector, { workItemId }, signal),
    createPullRequest: (
      selector: TaskSelector & { sessionId: string; repositoryId: string },
      body: { title: string; description: string; baseBranch?: string; draft: boolean },
      signal?: AbortSignal,
    ) =>
      host.api.invokeAction<CreatePullRequestResult>(
        "pull_requests.create",
        {
          workspaceId: selector.workspaceId,
          taskId: selector.taskId,
          sessionId: selector.sessionId,
          repositoryId: selector.repositoryId,
          body,
        },
        { signal },
      ),
  };
}

export type TaskLinksApi = ReturnType<typeof createTaskLinksApi>;
