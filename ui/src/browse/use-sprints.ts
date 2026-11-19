import { currentHost, useEffect, useRef, useState } from "../runtime";
import { createBrowseApi } from "./api";
import { pickOption } from "./board-discovery";
import { ANY_OPTION, UNASSIGNED_PERSON, defaultIterationId, teamPickerOptions } from "./sprints-model";
import type { AzureDevOpsSprintBoard } from "./types";
import type { AzureDevOpsSprintsPreference } from "./use-preferences";
import { useTeamList, useTeamSettings } from "./use-teams";

type BoardState = { data: AzureDevOpsSprintBoard | null; loading: boolean; error: string | null };

function useSprintBoard(workspaceId: string | undefined, projectId: string, teamId: string, iterationId: string) {
  const [state, setState] = useState<BoardState>({ data: null, loading: false, error: null });
  useEffect(() => {
    if (!workspaceId || !projectId || !teamId || !iterationId) {
      setState({ data: null, loading: false, error: null });
      return;
    }
    const controller = new AbortController();
    setState({ data: null, loading: true, error: null });
    createBrowseApi(currentHost(), workspaceId)
      .sprintBoard(projectId, teamId, iterationId, controller.signal)
      .then((data) => !controller.signal.aborted && setState({ data, loading: false, error: null }))
      .catch((cause: unknown) => {
        if (controller.signal.aborted) return;
        setState({ data: null, loading: false, error: cause instanceof Error ? cause.message : "Unable to load the sprint" });
      });
    return () => controller.abort();
  }, [iterationId, projectId, teamId, workspaceId]);
  return state;
}

/**
 * The Sprints selection: the team remembered for the project (else the user's
 * first team), the remembered sprint (else the current one), and the person
 * filter, with the selected sprint's taskboard.
 */
export function useAzureDevOpsSprints(
  workspaceId: string | undefined,
  projectId: string,
  rememberedTeamId: string | undefined,
  preference: AzureDevOpsSprintsPreference,
) {
  const rememberedRef = useRef({ teamId: rememberedTeamId, ...preference });
  rememberedRef.current = { teamId: rememberedTeamId, ...preference };
  const teamList = useTeamList(workspaceId, projectId);
  const [teamId, setTeamId] = useState("");
  useEffect(() => {
    const list = teamList.data;
    if (!list) {
      setTeamId("");
      return;
    }
    const ordered = teamPickerOptions(list.teams, list.mineIds, "");
    setTeamId((current) => pickOption(ordered, current, rememberedRef.current.teamId));
  }, [teamList.data]);

  const settings = useTeamSettings(workspaceId, projectId, teamId);
  const [iterationId, setIterationId] = useState("");
  useEffect(() => {
    const iterations = settings.data?.iterations;
    if (!iterations) {
      setIterationId("");
      return;
    }
    setIterationId((current) =>
      iterations.some((iteration) => iteration.id === current)
        ? current
        : defaultIterationId(iterations, rememberedRef.current.iterationId),
    );
  }, [settings.data]);

  const board = useSprintBoard(workspaceId, projectId, teamId, iterationId);
  const [person, setPerson] = useState(preference.person || ANY_OPTION);
  useEffect(() => {
    const people = board.data?.people;
    if (!people) return;
    setPerson((current) =>
      current === ANY_OPTION || current === UNASSIGNED_PERSON || people.includes(current) ? current : ANY_OPTION,
    );
  }, [board.data]);

  return {
    teams: teamList.data?.teams ?? [],
    mineIds: teamList.data?.mineIds ?? [],
    teamsLoading: teamList.loading,
    teamId,
    setTeamId,
    iterations: settings.data?.iterations ?? [],
    settingsLoading: settings.loading,
    iterationId,
    setIterationId,
    board: board.data,
    boardLoading: board.loading,
    person,
    setPerson,
    error: teamList.error ?? settings.error ?? board.error,
  };
}
