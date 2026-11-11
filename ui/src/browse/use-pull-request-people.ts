import { currentHost } from "../runtime";
import { createBrowseApi } from "./api";
import { createRequestCache } from "./board-discovery";
import type { AzureDevOpsIdentity, AzureDevOpsPullRequestPeople } from "./types";
import { useCached } from "./use-teams";

const api = (workspaceId: string) => createBrowseApi(currentHost(), workspaceId);

/** One people read per workspace, project, and repository for the page session. */
const peopleCache = createRequestCache<AzureDevOpsPullRequestPeople>((workspaceId, projectId, repositoryId) =>
  api(workspaceId).pullRequestPeople(projectId, repositoryId),
);

/** One identity read per workspace for the page session. */
const identityCache = createRequestCache<AzureDevOpsIdentity>((workspaceId) => api(workspaceId).identity());

/**
 * The people the creator and reviewer filters can name: those of the
 * repository's pull requests, or of the whole project without a repository.
 */
export function usePullRequestPeople(workspaceId: string | undefined, projectId: string, repositoryId: string) {
  const ready = Boolean(workspaceId && projectId);
  return useCached(
    ready ? () => peopleCache.get(workspaceId ?? "", projectId, repositoryId) : null,
    JSON.stringify([workspaceId, projectId, repositoryId]),
    "Unable to load the pull requests' people",
  );
}

/** The signed-in Azure DevOps identity, which the Me option stands for. */
export function useAzureIdentity(workspaceId: string | undefined) {
  return useCached(
    workspaceId ? () => identityCache.get(workspaceId) : null,
    JSON.stringify([workspaceId]),
    "Unable to load the signed-in identity",
  );
}
