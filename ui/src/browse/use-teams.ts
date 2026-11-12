import { useEffect, useRef, useState } from "../runtime";
import { currentHost } from "../runtime";
import { createBrowseApi } from "./api";
import { createRequestCache } from "./board-discovery";
import type { AzureDevOpsTeamList, AzureDevOpsTeamSettings } from "./types";

const api = (workspaceId: string) => createBrowseApi(currentHost(), workspaceId);

/** One team listing per workspace and project for the page session. */
export const teamsCache = createRequestCache<AzureDevOpsTeamList>((workspaceId, projectId) =>
  api(workspaceId).teams(projectId),
);

/** One settings read per workspace, project, and team for the page session. */
export const teamSettingsCache = createRequestCache<AzureDevOpsTeamSettings>((workspaceId, projectId, teamId) =>
  api(workspaceId).teamSettings(projectId, teamId),
);

function errorMessage(cause: unknown, fallback: string): string {
  return cause instanceof Error ? cause.message : fallback;
}

type Loaded<T> = { data: T | undefined; loading: boolean; error: string | null };

/** A cached read keyed by `key`, or nothing while `load` is null. */
export function useCached<T>(load: (() => Promise<T>) | null, key: string, fallbackError: string): Loaded<T> {
  const [state, setState] = useState<Loaded<T>>({ data: undefined, loading: false, error: null });
  const loadRef = useRef(load);
  loadRef.current = load;
  useEffect(() => {
    const load = loadRef.current;
    if (!load) {
      setState({ data: undefined, loading: false, error: null });
      return;
    }
    let cancelled = false;
    setState({ data: undefined, loading: true, error: null });
    load()
      .then((data) => !cancelled && setState({ data, loading: false, error: null }))
      .catch((cause) => !cancelled && setState({ data: undefined, loading: false, error: errorMessage(cause, fallbackError) }));
    return () => {
      cancelled = true;
    };
  }, [key, fallbackError]);
  return state;
}

/** The project's teams and which of them are the connected identity's. */
export function useTeamList(workspaceId: string | undefined, projectId: string) {
  const ready = Boolean(workspaceId && projectId);
  return useCached(
    ready ? () => teamsCache.get(workspaceId ?? "", projectId) : null,
    JSON.stringify([workspaceId, projectId]),
    "Unable to load teams",
  );
}

/** The team's areas and sprints. */
export function useTeamSettings(workspaceId: string | undefined, projectId: string, teamId: string) {
  const ready = Boolean(workspaceId && projectId && teamId);
  return useCached(
    ready ? () => teamSettingsCache.get(workspaceId ?? "", projectId, teamId) : null,
    JSON.stringify([workspaceId, projectId, teamId]),
    "Unable to load the team's sprints",
  );
}
