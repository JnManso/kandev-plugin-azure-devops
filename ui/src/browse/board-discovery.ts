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

/**
 * Shares one request per key (for example workspace and project) for the page
 * session; a failed request is forgotten so the next one retries.
 */
export function createRequestCache<V>(fetchValue: (...keys: string[]) => Promise<V>) {
  const entries = new Map<string, Promise<V>>();
  return {
    get(...keys: string[]): Promise<V> {
      const key = JSON.stringify(keys);
      let entry = entries.get(key);
      if (!entry) {
        entry = fetchValue(...keys).catch((cause: unknown) => {
          entries.delete(key);
          throw cause;
        });
        entries.set(key, entry);
      }
      return entry;
    },
  };
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
