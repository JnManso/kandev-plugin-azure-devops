import type { TaskCreationContext } from "@kdlbs/plugin-sdk";
import { htmlToText } from "./text";
import type { AzureDevOpsActionPreset, AzureDevOpsPullRequest, AzureDevOpsWorkItem } from "./types";

export type AzureDevOpsLaunchPayload =
  | { kind: "work-item"; item: AzureDevOpsWorkItem; action?: AzureDevOpsActionPreset }
  | { kind: "pull-request"; pullRequest: AzureDevOpsPullRequest; action?: AzureDevOpsActionPreset };

const MAX_TITLE_LENGTH = 200;

function truncateTitle(title: string): string {
  return title.length <= MAX_TITLE_LENGTH ? title : `${title.slice(0, MAX_TITLE_LENGTH - 1)}…`;
}

function actionDescription(
  action: AzureDevOpsActionPreset | undefined,
  url: string,
  title: string,
  fallback: string,
): string {
  if (!action?.promptTemplate.trim()) return fallback;
  return action.promptTemplate.replaceAll("{{url}}", url).replaceAll("{{title}}", title);
}

/**
 * Title and description of a task started from a work item or pull request.
 * These are task data written in English, like the built-in integration did.
 */
export function launchText(payload: AzureDevOpsLaunchPayload, emptyDescription: string) {
  if (payload.kind === "work-item") {
    const item = payload.item;
    const url = item.webUrl ?? item.apiUrl ?? "";
    // i18n-exempt: persisted task description, not UI copy.
    const description = [`Azure DevOps work item: ${item.id}`, `URL: ${url}`, "", item.title, "", htmlToText(item.description ?? "") || emptyDescription].join("\n");
    return {
      title: truncateTitle(payload.action ? `${payload.action.label}: ${item.title}` : `${item.type} ${item.id}: ${item.title}`),
      description: actionDescription(payload.action, url, item.title, description),
    };
  }
  const pr = payload.pullRequest;
  // i18n-exempt: persisted task description, not UI copy.
  const description = [`Azure DevOps pull request: ${pr.webUrl}`, "", pr.title, "", htmlToText(pr.description ?? "") || emptyDescription].join("\n");
  return {
    title: truncateTitle(payload.action ? `${payload.action.label}: ${pr.title}` : `Review PR ${pr.id}: ${pr.title}`),
    description: actionDescription(payload.action, pr.webUrl, pr.title, description),
  };
}

export function organizationFromUrl(organizationUrl: string): string {
  try {
    return new URL(organizationUrl).pathname.split("/").filter(Boolean)[0] ?? "";
  } catch {
    return "";
  }
}

export type LaunchRepository = { repositoryId?: string; remoteUrl?: string; branch?: string; checkoutBranch?: string };

/**
 * Repository preset for a pull request task: the workspace repository when it
 * is already in Kandev, otherwise its Azure Repos URL so the dialog imports it.
 */
export function launchRepository(
  payload: AzureDevOpsLaunchPayload,
  resolveRepositoryId: (providerScope: string, providerRepositoryId: string) => string | undefined,
  organization: string,
): LaunchRepository {
  if (payload.kind !== "pull-request") return {};
  const pr = payload.pullRequest;
  const sourceBranch = pr.sourceBranch.replace(/^refs\/heads\//, "");
  const branches = sourceBranch ? { branch: sourceBranch, checkoutBranch: sourceBranch } : {};
  const repositoryId = organization ? resolveRepositoryId(`${organization}/${pr.projectName}`, pr.repositoryId) : undefined;
  if (repositoryId) return { repositoryId, ...branches };
  const remoteUrl = pr.webUrl.replace(/\/pullrequest\/\d+$/, "");
  return remoteUrl ? { remoteUrl, ...branches } : {};
}

/** The dialog needs a workflow with at least one step. */
export function hasLaunchWorkflow(context: TaskCreationContext | null): context is TaskCreationContext {
  return !!context?.workflowId && !!context.defaultStepId && context.steps.length > 0;
}
