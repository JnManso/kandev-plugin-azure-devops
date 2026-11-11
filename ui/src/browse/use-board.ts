
import { useCallback, useEffect, useRef, useState } from "../runtime";
import { useAzureDevOpsMutationOwner } from "./use-mutation-owner";
import { pickOption, rememberedBoardStart } from "./board-discovery";
import { teamPickerOptions } from "./sprints-model";
import { teamsCache } from "./use-teams";
import type { AzureDevOpsBoardPreference } from "./use-preferences";
import {
  getAzureDevOpsBoardSnapshot,
  listAzureDevOpsBoards,
  updateAzureDevOpsBoardWorkItem,
} from "./rest-compat";
import type {
  AzureDevOpsBoardSnapshot,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsBoardWorkItemUpdate,
  AzureDevOpsBoardReference,
  AzureDevOpsTeam,
} from "./types";

type BoardWorkItemChanges = Omit<AzureDevOpsBoardWorkItemUpdate, "revision">;

function errorMessage(cause: unknown, fallback: string): string {
  return cause instanceof Error ? cause.message : fallback;
}

function replaceBoardItem(
  snapshot: AzureDevOpsBoardSnapshot | null,
  id: number,
  item: AzureDevOpsBoardWorkItem,
): AzureDevOpsBoardSnapshot | null {
  if (!snapshot) return snapshot;
  return {
    ...snapshot,
    items: snapshot.items.map((candidate) =>
      candidate.id === id && candidate.revision <= item.revision ? item : candidate,
    ),
  };
}

type BoardPreference = AzureDevOpsBoardPreference | undefined;

/**
 * Starts on the team and board remembered for the project while the team
 * list loads, then keeps them only when the project still lists them. The
 * team remembered for the project by any team selector wins over the Board's
 * own preference; otherwise the user's first team is preselected.
 */
// eslint-disable-next-line max-lines-per-function -- one owner for the team and board discovery state.
function useBoardDiscovery(
  workspaceId: string | undefined,
  projectId: string,
  preference: BoardPreference,
  rememberedTeamId: string | undefined,
) {
  const preferenceRef = useRef(preference);
  preferenceRef.current = preference;
  const rememberedTeamRef = useRef(rememberedTeamId);
  rememberedTeamRef.current = rememberedTeamId;
  const [teams, setTeams] = useState<AzureDevOpsTeam[]>([]);
  const [mineIds, setMineIds] = useState<string[]>([]);
  const [boards, setBoards] = useState<AzureDevOpsBoardReference[]>([]);
  const [teamsLoading, setTeamsLoading] = useState(false);
  const [boardsLoading, setBoardsLoading] = useState(false);
  const [teamId, setTeamIdState] = useState("");
  const [boardId, setBoardId] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const boardStart = rememberedBoardStart(projectId, preferenceRef.current);
    const start =
      boardStart && (!rememberedTeamRef.current || rememberedTeamRef.current === boardStart.team.id)
        ? boardStart
        : null;
    setError(null);
    setMineIds([]);
    setTeams(start ? [{ ...start.team, projectId }] : []);
    setBoards(start ? [start.board] : []);
    setTeamIdState(start?.team.id ?? "");
    setBoardId(start?.board.id ?? "");
    if (!workspaceId || !projectId) return;
    let cancelled = false;
    setTeamsLoading(true);
    teamsCache
      .get(workspaceId, projectId)
      .then((list) => {
        if (cancelled) return;
        setTeams(list.teams);
        setMineIds(list.mineIds);
        const ordered = teamPickerOptions(list.teams, list.mineIds, "");
        const remembered = rememberedTeamRef.current ?? preferenceRef.current?.teamId;
        setTeamIdState((current) => pickOption(ordered, current, remembered));
      })
      .catch((cause) => !cancelled && setError(errorMessage(cause, "Unable to load teams")))
      .finally(() => !cancelled && setTeamsLoading(false));
    return () => {
      cancelled = true;
    };
  }, [projectId, workspaceId]);

  useEffect(() => {
    if (!workspaceId || !projectId || !teamId) {
      setBoardsLoading(false);
      return;
    }
    let cancelled = false;
    setBoardsLoading(true);
    listAzureDevOpsBoards(workspaceId, projectId, teamId)
      .then((result) => {
        if (cancelled) return;
        setBoards(result.boards);
        setBoardId((current) => pickOption(result.boards, current, preferenceRef.current?.boardId));
      })
      .catch((cause) => !cancelled && setError(errorMessage(cause, "Unable to load boards")))
      .finally(() => !cancelled && setBoardsLoading(false));
    return () => {
      cancelled = true;
    };
  }, [projectId, teamId, workspaceId]);

  const setTeamId = useCallback(
    (next: string) => {
      if (next === teamId) return;
      setBoards([]);
      setBoardId("");
      setTeamIdState(next);
    },
    [teamId],
  );

  return {
    teams,
    mineIds,
    boards,
    teamId,
    setTeamId,
    boardId,
    setBoardId,
    discovering: teamsLoading || boardsLoading,
    error,
  };
}

function useBoardSnapshot(
  workspaceId: string | undefined,
  projectId: string,
  teamId: string,
  boardId: string,
) {
  const [snapshot, setSnapshot] = useState<AzureDevOpsBoardSnapshot | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    // A failed read must not leave the previous board's cards on screen.
    setSnapshot(null);
    if (!workspaceId || !projectId || !teamId || !boardId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    getAzureDevOpsBoardSnapshot(workspaceId, projectId, teamId, boardId)
      .then((next) => !cancelled && setSnapshot(next))
      .catch((cause) => !cancelled && setError(errorMessage(cause, "Unable to load board")))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [boardId, projectId, teamId, workspaceId]);

  return { snapshot, loading, error, setSnapshot, setError };
}

export function useAzureDevOpsBoard(
  workspaceId: string | undefined,
  projectId: string,
  preference?: AzureDevOpsBoardPreference,
  rememberedTeamId?: string,
) {
  const discovery = useBoardDiscovery(workspaceId, projectId, preference, rememberedTeamId);
  const { teamId, boardId } = discovery;
  const beginMutation = useAzureDevOpsMutationOwner(
    JSON.stringify([workspaceId, projectId, teamId, boardId]),
  );
  const { snapshot, loading, error, setSnapshot, setError } = useBoardSnapshot(
    workspaceId,
    projectId,
    teamId,
    boardId,
  );

  const updateItem = useCallback(
    async (
      item: AzureDevOpsBoardWorkItem,
      values: BoardWorkItemChanges,
    ): Promise<AzureDevOpsBoardWorkItem | undefined> => {
      const isCurrent = beginMutation();
      if (!workspaceId || !teamId || !boardId || !isCurrent()) return undefined;
      setError(null);
      const optimistic = { ...item, ...values };
      setSnapshot((current) => replaceBoardItem(current, item.id, optimistic));
      try {
        const updated = await updateAzureDevOpsBoardWorkItem(
          workspaceId,
          projectId,
          teamId,
          boardId,
          item.id,
          { revision: item.revision, ...values },
        );
        if (!isCurrent()) return undefined;
        setSnapshot((current) =>
          isCurrent() ? replaceBoardItem(current, item.id, updated) : current,
        );
        return updated;
      } catch (cause) {
        if (!isCurrent()) return undefined;
        setSnapshot((current) => {
          if (!current || !isCurrent()) return current;
          return {
            ...current,
            items: current.items.map((candidate) => (candidate === optimistic ? item : candidate)),
          };
        });
        setError(errorMessage(cause, "Unable to update work item"));
        throw cause;
      }
    },
    [beginMutation, boardId, projectId, setError, setSnapshot, teamId, workspaceId],
  );

  const moveItem = useCallback(
    async (
      item: AzureDevOpsBoardWorkItem,
      columnId: string,
      columnDone = item.columnDone,
    ): Promise<AzureDevOpsBoardWorkItem | undefined> => {
      if (item.columnId !== columnId || item.columnDone !== columnDone) {
        return updateItem(item, { columnId, columnDone });
      }
      return item;
    },
    [updateItem],
  );

  const updateAssignee = useCallback(
    (item: AzureDevOpsBoardWorkItem, assigneeAction: "assign_current_user" | "unassign") =>
      updateItem(item, { assigneeAction }),
    [updateItem],
  );

  const mergeItem = useCallback(
    (item: AzureDevOpsBoardWorkItem) => {
      const isCurrent = beginMutation();
      if (!isCurrent()) return;
      setSnapshot((current) => (isCurrent() ? replaceBoardItem(current, item.id, item) : current));
    },
    [beginMutation, setSnapshot],
  );

  return {
    teams: discovery.teams,
    mineIds: discovery.mineIds,
    boards: discovery.boards,
    teamId,
    setTeamId: discovery.setTeamId,
    boardId,
    setBoardId: discovery.setBoardId,
    snapshot,
    loading,
    discovering: discovery.discovering,
    error: discovery.error ?? error,
    moveItem,
    updateAssignee,
    mergeItem,
  };
}
