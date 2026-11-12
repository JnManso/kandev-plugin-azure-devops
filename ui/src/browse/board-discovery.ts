import type { AzureDevOpsBoardPreference } from "./use-preferences";

type Option = { id: string; name: string };

/**
 * The team and board the user last opened in this project, when the
 * preference names both with their labels, so the Board can load them before
 * the project's team list arrives.
 */
export function rememberedBoardStart(
  projectId: string,
  preference: AzureDevOpsBoardPreference | undefined,
): { team: Option; board: Option } | null {
  if (!preference || !projectId || preference.projectId !== projectId) return null;
  const { teamId, teamName, boardId, boardName } = preference;
  if (!teamId || !teamName || !boardId || !boardName) return null;
  return { team: { id: teamId, name: teamName }, board: { id: boardId, name: boardName } };
}

/** The current option when listed, else the remembered one, else the first. */
export function pickOption(options: Option[], current: string, remembered: string | undefined): string {
  if (options.some((option) => option.id === current)) return current;
  if (remembered && options.some((option) => option.id === remembered)) return remembered;
  return options[0]?.id ?? "";
}

/** How long a successful shared request is reused. */
export const REQUEST_CACHE_TTL_MS = 5 * 60 * 1000;

type RequestCache = { clear(): void };

const requestCaches = new Set<RequestCache>();

/**
 * Shares one request per key (for example workspace and project) for a
 * bounded time; a failed request is forgotten so the next one retries. The
 * browse page clears every cache when it opens and when the connected
 * organization changes, so a new sign-in never reads the previous account's
 * identity, teams, or sprint settings.
 */
export function createRequestCache<V>(
  fetchValue: (...keys: string[]) => Promise<V>,
  options: { ttlMs?: number; now?: () => number } = {},
) {
  const ttlMs = options.ttlMs ?? REQUEST_CACHE_TTL_MS;
  const now = options.now ?? Date.now;
  const entries = new Map<string, { value: Promise<V>; expiresAt: number }>();
  const cache = {
    get(...keys: string[]): Promise<V> {
      const key = JSON.stringify(keys);
      const existing = entries.get(key);
      if (existing && existing.expiresAt > now()) return existing.value;
      const entry = {
        value: fetchValue(...keys).catch((cause: unknown) => {
          if (entries.get(key) === entry) entries.delete(key);
          throw cause;
        }),
        expiresAt: now() + ttlMs,
      };
      entries.set(key, entry);
      return entry.value;
    },
    clear() {
      entries.clear();
    },
  };
  requestCaches.add(cache);
  return cache;
}

/** Forgets every shared request, so the next reads go to Azure DevOps. */
export function clearRequestCaches(): void {
  for (const cache of requestCaches) cache.clear();
}

/** The Board preference for the open team and board, with their labels. */
export function boardPreferenceFor(open: {
  projectId: string;
  teams: Option[];
  teamId: string;
  boards: Option[];
  boardId: string;
  focusedColumnId: string;
}): AzureDevOpsBoardPreference {
  return {
    projectId: open.projectId,
    teamId: open.teamId,
    teamName: open.teams.find((team) => team.id === open.teamId)?.name,
    boardId: open.boardId,
    boardName: open.boards.find((board) => board.id === open.boardId)?.name,
    focusedColumnId: open.focusedColumnId,
  };
}
