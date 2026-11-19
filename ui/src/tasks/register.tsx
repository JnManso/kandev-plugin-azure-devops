import type {
  PluginHostApi,
  PluginIcon,
  PluginRegistry,
  ReviewSummary,
  ReviewTaskAssociation,
  TaskContext,
} from "@kdlbs/plugin-sdk";
import { pluginTranslate } from "../i18n";
import { createSnapshotStore } from "../source-control/review-store";
import { createTaskLinksApi, type TaskLinksApi } from "./api";
import { AzureDevOpsReviewPanel } from "./review-panel";
import { PROVIDER_ID, toAssociations, toReviewSummary } from "./review-model";

export type TaskIntegrations = {
  api: TaskLinksApi;
  /** Refreshes a task's review snapshot and its workspace associations. */
  refreshTask(workspaceId: string, taskId: string, signal?: AbortSignal): Promise<void>;
  destroy(): void;
};

type LinkKind = "pullRequest" | "workItem";

/** Localized link failure from the action's HTTP status. */
export function linkErrorKey(error: unknown): "linkReferenceNotFound" | "linkReferenceInvalid" | "linkConnectionRequired" | null {
  const status = (error as { status?: unknown } | null)?.status;
  switch (status) {
    case 404:
      return "linkReferenceNotFound";
    case 400:
      return "linkReferenceInvalid";
    case 409:
      return "linkConnectionRequired";
    default:
      return null;
  }
}

/**
 * Registers the review provider for linked pull requests and the Link submenu
 * entries for pull requests and work items.
 */
export function registerTaskIntegrations(registry: PluginRegistry, host: PluginHostApi, icon: PluginIcon): TaskIntegrations {
  const t = pluginTranslate(host);
  const api = createTaskLinksApi(host);
  const reviews = createSnapshotStore<string, ReviewSummary>();
  const associations = createSnapshotStore<string, ReviewTaskAssociation>();
  const overlays = new Set<{ close(): void }>();

  async function refreshReviews(taskId: string, signal: AbortSignal, workspaceId?: string) {
    const token = reviews.beginRefresh(taskId);
    const links = await api.get({ taskId, ...(workspaceId ? { workspaceId } : {}) }, signal);
    const summaries = links.pullRequests.flatMap((link) => {
      const summary = toReviewSummary(link, t("pullRequestFallbackTitle", { values: { id: link.pullRequestId } }));
      return summary ? [summary] : [];
    });
    reviews.commit(taskId, token, summaries);
  }

  async function refreshAssociations(workspaceId: string, signal: AbortSignal) {
    const token = associations.beginRefresh(workspaceId);
    associations.commit(workspaceId, token, toAssociations(await api.workspace(workspaceId, signal)));
  }

  async function refreshTask(workspaceId: string, taskId: string, signal: AbortSignal = new AbortController().signal) {
    await Promise.all([refreshReviews(taskId, signal, workspaceId), refreshAssociations(workspaceId, signal)]).catch(
      () => undefined,
    );
  }

  function openLinkDialog(kind: LinkKind, context: TaskContext) {
    const isPullRequest = kind === "pullRequest";
    const dialog = host.openTaskLinkDialog({
      title: t(isPullRequest ? "linkPullRequestTitle" : "linkWorkItemTitle"),
      description: t(isPullRequest ? "linkPullRequestDescription" : "linkWorkItemDescription"),
      inputLabel: t(isPullRequest ? "pullRequestKindTitle" : "workItemKindTitle"),
      placeholder: isPullRequest ? "Project/Repository!42" : "Project#42",
      emptyError: t(isPullRequest ? "linkPullRequestEmpty" : "linkWorkItemEmpty"),
      failureMessage: t(isPullRequest ? "failedToLinkPullRequest" : "failedToLinkWorkItem"),
      successMessage: t(isPullRequest ? "linkPullRequestSuccess" : "linkWorkItemSuccess"),
      inputTestId: `azure-devops-link-${kind}-reference`,
      errorTestId: `azure-devops-link-${kind}-error`,
      submitTestId: `azure-devops-link-${kind}-submit`,
      async onSubmit(reference, signal) {
        const value = reference.trim();
        if (!value) throw new Error(t(isPullRequest ? "linkPullRequestEmpty" : "linkWorkItemEmpty"));
        const selector = { workspaceId: context.workspaceId, taskId: context.taskId };
        try {
          if (isPullRequest) await api.linkPullRequest(selector, { reference: value }, signal);
          else await api.linkWorkItem(selector, { reference: value }, signal);
        } catch (error) {
          const key = linkErrorKey(error);
          throw key ? new Error(t(key)) : error;
        }
        await refreshTask(context.workspaceId, context.taskId, signal);
      },
    });
    overlays.add(dialog);
  }

  registry.registerTaskAction({
    id: "azure-devops-link-pull-request",
    get label() {
      return t("linkPullRequestMenu");
    },
    icon,
    placement: "link",
    singleTaskOnly: true,
    run: async (context) => openLinkDialog("pullRequest", context),
  });
  registry.registerTaskAction({
    id: "azure-devops-link-work-item",
    get label() {
      return t("linkWorkItemMenu");
    },
    icon,
    placement: "link",
    singleTaskOnly: true,
    run: async (context) => openLinkDialog("workItem", context),
  });
  registry.registerReviewProvider({
    id: PROVIDER_ID,
    get label() {
      return t("azureDevOps");
    },
    icon,
    get changeRequestNoun() {
      return t("pullRequestKindLower");
    },
    order: 40,
    getSnapshot: (taskId) => reviews.get(taskId),
    subscribe: (taskId, listener) => reviews.subscribe(taskId, listener),
    refresh: (taskId, signal) => refreshReviews(taskId, signal),
    getAssociationSnapshot: (workspaceId) => associations.get(workspaceId),
    subscribeAssociations: (workspaceId, listener) => associations.subscribe(workspaceId, listener),
    refreshAssociations,
    async unlink({ workspaceId, taskId, repositoryId, changeRequestNumber, signal }) {
      const number = Number(changeRequestNumber);
      if (!repositoryId || !Number.isInteger(number) || number <= 0) return;
      await api.unlinkPullRequest({ workspaceId, taskId }, repositoryId, number, signal);
      await refreshTask(workspaceId, taskId, signal);
    },
    ReviewPanel: (props) => (
      <AzureDevOpsReviewPanel
        presentation={props.presentation}
        workspaceId={props.workspaceId}
        connectionScope={props.connectionScope}
        repositoryId={props.repositoryId}
        changeRequestNumber={props.changeRequestNumber}
      />
    ),
  });

  return {
    api,
    refreshTask,
    destroy() {
      overlays.forEach((overlay) => overlay.close());
      overlays.clear();
      reviews.clear();
      associations.clear();
    },
  };
}
