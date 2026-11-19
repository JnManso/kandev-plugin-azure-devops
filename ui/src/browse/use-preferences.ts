
import { currentHost, useCallback, useEffect, useRef, useState } from "../runtime";
import type { Dispatch, SetStateAction } from "../runtime";
import type {
  AzureDevOpsBrowseMode,
  AzureDevOpsFiltersState,
} from "./filters";
import type { AzureDevOpsScopeSelection } from "./scope-bar";

export type AzureDevOpsBoardPreference = {
  projectId?: string;
  teamId?: string;
  teamName?: string;
  boardId?: string;
  boardName?: string;
  focusedColumnId?: string;
};

export type AzureDevOpsSprintsPreference = {
  iterationId?: string;
  person?: string;
};

export type AzureDevOpsBrowsePreference = {
  mode?: AzureDevOpsBrowseMode;
  selection?: AzureDevOpsScopeSelection;
  filters?: Partial<AzureDevOpsFiltersState>;
  board?: AzureDevOpsBoardPreference;
  /** The team last chosen in each project, by any team selector. */
  teamByProject?: Record<string, string>;
  sprints?: AzureDevOpsSprintsPreference;
  /** Whether each board opens with its cards' children expanded. */
  expandChildren?: Record<string, boolean>;
};

type PagePreferenceState = {
  mode: AzureDevOpsBrowseMode;
  setMode: Dispatch<SetStateAction<AzureDevOpsBrowseMode>>;
  filters: AzureDevOpsFiltersState;
  replaceFilters: (filters: AzureDevOpsFiltersState) => void;
  selection: AzureDevOpsScopeSelection;
  setSelection: Dispatch<SetStateAction<AzureDevOpsScopeSelection>>;
  defaultFilters: AzureDevOpsFiltersState;
};

/** Per-user browse preferences live in plugin user storage, per workspace. */
const PREFERENCES_KEY = "browse";

export function readAzureDevOpsBrowsePreference(value: unknown): AzureDevOpsBrowsePreference | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  return value as AzureDevOpsBrowsePreference;
}

export function useAzureDevOpsPreferences(workspaceId: string | undefined) {
  const [loaded, setLoaded] = useState(false);
  const [preference, setPreference] = useState<AzureDevOpsBrowsePreference | undefined>(undefined);
  const revision = useRef(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoaded(false);
    setPreference(undefined);
    if (!workspaceId) {
      setLoaded(true);
      return () => controller.abort();
    }
    currentHost()
      .storage.get("workspace", workspaceId, PREFERENCES_KEY, { signal: controller.signal })
      .then((entry) => setPreference(readAzureDevOpsBrowsePreference(entry?.value)))
      .catch(() => setPreference(undefined))
      .finally(() => {
        if (!controller.signal.aborted) setLoaded(true);
      });
    return () => controller.abort();
  }, [workspaceId]);

  const save = useCallback(
    (next: AzureDevOpsBrowsePreference) => {
      if (!workspaceId) return;
      const currentRevision = ++revision.current;
      setPreference(next);
      void currentHost()
        .storage.set("workspace", workspaceId, PREFERENCES_KEY, next)
        .catch(() => {
          if (currentRevision === revision.current) setPreference(next);
        });
    },
    [workspaceId],
  );

  return { loaded, preference, save };
}

export function useAzureDevOpsPagePreferences(
  workspaceId: string | undefined,
  state: PagePreferenceState,
) {
  const { loaded, preference, save } = useAzureDevOpsPreferences(workspaceId);
  const [hydrated, setHydrated] = useState(false);
  const [board, setBoard] = useState<AzureDevOpsBoardPreference>({});
  const [teamByProject, setTeamByProject] = useState<Record<string, string>>({});
  const [sprints, setSprints] = useState<AzureDevOpsSprintsPreference>({});
  const [expandChildren, setExpandChildren] = useState<Record<string, boolean>>({});
  const initialized = useRef(false);
  const persisted = useRef("");

  useEffect(() => {
    if (!loaded || initialized.current) return;
    if (preference?.mode) state.setMode(preference.mode);
    if (preference?.filters)
      state.replaceFilters({ ...state.defaultFilters, ...preference.filters });
    if (preference?.selection) state.setSelection(preference.selection);
    setBoard(preference?.board ?? {});
    setTeamByProject(preference?.teamByProject ?? {});
    setSprints(preference?.sprints ?? {});
    setExpandChildren(preference?.expandChildren ?? {});
    persisted.current = JSON.stringify(preference ?? {});
    initialized.current = true;
    setHydrated(true);
  }, [loaded, preference, state]);

  useEffect(() => {
    if (!hydrated) return;
    const next = {
      mode: state.mode,
      selection: state.selection,
      filters: state.filters,
      board,
      teamByProject,
      sprints,
      expandChildren,
    };
    const serialized = JSON.stringify(next);
    if (serialized === persisted.current) return;
    persisted.current = serialized;
    save(next);
  }, [board, expandChildren, hydrated, save, sprints, state, teamByProject]);

  const rememberTeam = useCallback((projectId: string, teamId: string) => {
    if (!projectId || !teamId) return;
    setTeamByProject((current) => (current[projectId] === teamId ? current : { ...current, [projectId]: teamId }));
  }, []);

  const rememberExpandChildren = useCallback((key: string, expand: boolean) => {
    setExpandChildren((current) => (current[key] === expand ? current : { ...current, [key]: expand }));
  }, []);

  return {
    hydrated,
    board,
    setBoard,
    teamByProject,
    rememberTeam,
    sprints,
    setSprints,
    expandChildren,
    rememberExpandChildren,
  };
}
