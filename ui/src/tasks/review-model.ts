import type { ReviewSummary, ReviewTaskAssociation, ReviewTaskStatus } from "@kdlbs/plugin-sdk";
import type { AzureDevOpsPullRequest, AzureDevOpsPullRequestFeedback } from "../browse/types";
import type { PullRequestLinkStatus, WorkspaceLinks } from "./api";

export const PROVIDER_ID = "azure_devops";

/** Stable review identity: Azure repository GUID and pull request number. */
export function reviewKey(repositoryId: string, pullRequestId: number): string {
  return `${repositoryId.toLowerCase()}!${pullRequestId}`;
}

const TASK_STATES = new Set(["open", "merged", "closed", "draft"]);
const PIPELINE_STATES = new Set(["success", "failure", "pending", "neutral"]);

function validTaskStatus(status: ReviewTaskStatus | undefined): ReviewTaskStatus | undefined {
  if (!status || !TASK_STATES.has(status.state) || !PIPELINE_STATES.has(status.pipelineState)) return undefined;
  return { ...status, checks: (status.checks ?? []).filter((check) => PIPELINE_STATES.has(check.state)) };
}

export function pullRequestState(status: string | undefined, draft?: boolean): string {
  switch ((status ?? "").toLowerCase()) {
    case "completed":
      return "merged";
    case "abandoned":
      return "closed";
    default:
      return draft ? "draft" : "open";
  }
}

/** Maps a linked pull request to the host review summary. */
export function toReviewSummary(link: PullRequestLinkStatus, fallbackTitle: string): ReviewSummary | null {
  if (!link.repositoryId || !link.projectId || !(link.pullRequestId > 0)) return null;
  const taskStatus = validTaskStatus(link.taskStatus);
  return {
    providerId: PROVIDER_ID,
    reviewKey: reviewKey(link.repositoryId, link.pullRequestId),
    title: link.title?.trim() || fallbackTitle,
    url: link.url ?? "",
    connectionScope: link.projectId,
    repositoryId: link.repositoryId,
    changeRequestNumber: link.pullRequestId,
    state: taskStatus?.state ?? pullRequestState(link.status, link.isDraft),
    ...(taskStatus ? { taskStatus } : {}),
  };
}

/** Flattens workspace pull request links for linked-task indicators. */
export function toAssociations(links: WorkspaceLinks): ReviewTaskAssociation[] {
  return Object.entries(links.pullRequests).flatMap(([taskId, pullRequests]) =>
    pullRequests
      .filter((link) => link.repositoryId && link.projectId && link.pullRequestId > 0)
      .map((link) => ({
        providerId: PROVIDER_ID,
        taskId,
        reviewKey: reviewKey(link.repositoryId, link.pullRequestId),
        connectionScope: link.projectId,
        repositoryId: link.repositoryId,
        changeRequestNumber: link.pullRequestId,
      })),
  );
}

type Person = { name: string };

function shortBranch(branch: string): string {
  return (branch ?? "").replace(/^refs\/heads\//, "");
}

function reviewerState(vote: number): string {
  if (vote >= 5) return "APPROVED";
  if (vote === -10) return "CHANGES_REQUESTED";
  return "PENDING";
}

function checkState(status: string): string {
  switch (status.toLowerCase()) {
    case "approved":
      return "success";
    case "rejected":
    case "broken":
      return "failure";
    case "queued":
    case "running":
      return "pending";
    default:
      return "neutral";
  }
}

function author(pr: AzureDevOpsPullRequest, fallback: string): Person {
  return { name: pr.author?.displayName?.trim() || fallback };
}

/** Maps Azure pull request feedback to the host ChangeRequestDetail model. */
export function toChangeRequestDetail(feedback: AzureDevOpsPullRequestFeedback, unknownAuthor: string) {
  const pr = feedback.pullRequest;
  const voted = feedback.reviewers.filter((reviewer) => reviewer.vote !== 0);
  const state = pullRequestState(pr.status, pr.isDraft);
  return {
    providerId: PROVIDER_ID,
    reviewKey: reviewKey(pr.repositoryId, pr.id),
    number: pr.id,
    title: pr.title,
    url: pr.webUrl,
    state: state === "draft" ? "open" : state,
    draft: pr.isDraft,
    author: author(pr, unknownAuthor),
    ...(pr.creationDate ? { createdAt: pr.creationDate } : {}),
    ...(state === "merged" && pr.closedDate ? { mergedAt: pr.closedDate } : {}),
    ...(state === "closed" && pr.closedDate ? { closedAt: pr.closedDate } : {}),
    sourceBranch: shortBranch(pr.sourceBranch),
    targetBranch: shortBranch(pr.targetBranch),
    additions: 0,
    deletions: 0,
    ...(pr.description ? { description: pr.description } : {}),
    reviews: voted.map((reviewer) => ({
      id: reviewer.id,
      author: { name: reviewer.displayName || unknownAuthor },
      state: reviewerState(reviewer.vote),
    })),
    requestedReviewers: feedback.reviewers
      .filter((reviewer) => reviewer.vote === 0)
      .map((reviewer) => ({ name: reviewer.displayName || unknownAuthor })),
    checks: feedback.policies.map((policy) => ({
      id: policy.id,
      name: policy.name || policy.id,
      state: checkState(policy.status),
    })),
    comments: feedback.threads.flatMap((thread) => {
      const resolved = !["active", "pending"].includes((thread.status ?? "").toLowerCase());
      return (thread.comments ?? [])
        .filter((comment) => comment.commentType !== "system" && comment.content?.trim())
        .map((comment) => ({
          id: `${thread.id}-${comment.id}`,
          author: { name: comment.author?.displayName || unknownAuthor },
          body: comment.content,
          ...(comment.publishedAt ? { createdAt: comment.publishedAt } : {}),
          resolved,
        }));
    }),
  };
}
