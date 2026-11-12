

import { useCallback, useEffect, useMemo, useRef, useState } from "../runtime";
import type { ReactNode } from "../runtime";
import { IconAdjustments } from "../icons";
import { Alert, AlertDescription } from "../host-ui";
import { Button } from "../host-ui";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "../host-ui";
import {
  AzureDevOpsFilters,
  type FiltersProps,
  type AzureDevOpsBrowseMode,
  type AzureDevOpsFiltersState,
} from "./filters";
import { pullRequestSearchQuery, repositoryForProject, workItemSearchWiql } from "./search-query";
import { createSearchSequence, settleScopedSearch } from "./search-sequence";
import { clearRequestCaches } from "./board-discovery";
import { AzureDevOpsFeedbackDialog } from "./feedback-dialog";
import {
  AzureDevOpsPullRequestResults,
  AzureDevOpsWorkItemResults,
} from "./results";
import {
  AzureDevOpsScopeBar,
  type AzureDevOpsScopeSelection,
} from "./scope-bar";
import { AzureDevOpsSaveViewDialog } from "./save-view-dialog";
import { filtersFromSavedView, workItemViewScope } from "./saved-view-filters";
import { AzureDevOpsBoard } from "./board";
import { AzureDevOpsSprints } from "./sprints";
import { teamPickerOptions, teamScopeConditions, wiqlWithConditions } from "./sprints-model";
import { useSprintOptions } from "./sprint-options";
import { useAzureIdentity, usePullRequestPeople } from "./use-pull-request-people";
import { teamSettingsCache, useTeamList, useTeamSettings } from "./use-teams";
import { AzureDevOpsTaskLauncher } from "./task-launcher";
import type { AzureDevOpsLaunchPayload } from "./task-launch";
import { AzureDevOpsWorkItemDetail } from "./work-item-detail";
import { AzureDevOpsPullRequestPagination } from "./pull-request-pagination";
import { AzureDevOpsModeTabs } from "./mode-tabs";
import {
  matchingPresetId,
  presetsForKind,
  type AzureDevOpsQueryPresets,
} from "./presets";
import {
  useAzureDevOpsConnection,
  useAzureDevOpsPullRequestFeedback,
  useAzureDevOpsPullRequestSearch,
  useAzureDevOpsWorkItemSearch,
} from "./use-browse";
import {
  useAzureDevOpsProjects,
  useAzureDevOpsRepositories,
} from "./use-projects";
import { useAzureDevOpsSavedViews } from "./use-saved-views";
import { useAzureDevOpsPagePreferences } from "./use-preferences";
import { useAzureDevOpsWorkspaceSettings } from "./use-workspace-actions";
import type {
  AzureDevOpsActionPreset,
  AzureDevOpsPullRequest,
  AzureDevOpsSavedView,
  AzureDevOpsWorkItem,
} from "./types";
import { useT } from "../i18n";
import { currentHost } from "../runtime";

const PAGE_SIZE = 25;
const EMPTY_ITERATIONS: never[] = [];
const WORK_ITEMS_MODE: AzureDevOpsBrowseMode = "work-items";
const PULL_REQUESTS_MODE: AzureDevOpsBrowseMode = "pull-requests";
const BOARD_MODE: AzureDevOpsBrowseMode = "board";
const SPRINTS_MODE: AzureDevOpsBrowseMode = "sprints";

/** Board and Sprints show taskboards instead of search filters and results. */
const isTaskboardMode = (mode: AzureDevOpsBrowseMode) => mode === BOARD_MODE || mode === SPRINTS_MODE;
const DEFAULT_WIQL =
  "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project ORDER BY [System.ChangedDate] DESC";

const DEFAULT_FILTERS: AzureDevOpsFiltersState = {
  projectId: "",
  repositoryId: "",
  wiql: DEFAULT_WIQL,
  text: "",
  top: 50,
  status: "active",
  creator: "",
  reviewer: "",
  teamId: "",
  iterationId: "",
};

type PageProps = { workspaceId?: string };

function NotConfigured({ workspaceId }: { workspaceId?: string }) {
  const t = useT();
  const href = workspaceId
    ? `/settings/workspaces/${encodeURIComponent(workspaceId)}/integrations/azure-devops`
    : "/settings/integrations/azure-devops";
  return (
    <div className="max-w-2xl p-6">
      <Alert>
        <AlertDescription>
          {t("notConnected")}{" "}
          <a
            href={href}
            className="cursor-pointer font-medium underline"
            data-testid="azure-devops-open-settings"
            onClick={(event) => {
              event.preventDefault();
              currentHost().navigate(href);
            }}
          >
            {t("openSettings")}
          </a>
        </AlertDescription>
      </Alert>
    </div>
  );
}

// Keys, not text: resolving at module scope would freeze these at the boot locale.
const RESULT_LABEL_KEYS: Record<AzureDevOpsBrowseMode, string> = {
  [WORK_ITEMS_MODE]: "workItems",
  [PULL_REQUESTS_MODE]: "pullRequests",
  [BOARD_MODE]: "board",
  [SPRINTS_MODE]: "sprints",
};

function ResultHeader({ mode, count }: { mode: AzureDevOpsBrowseMode; count: number }) {
  const t = useT();
  return (
    <div className="flex min-h-12 items-center justify-between border-b px-4">
      <h2 className="text-sm font-semibold">{t(RESULT_LABEL_KEYS[mode])}</h2>
      <span className="text-xs text-muted-foreground">
        {t("resultsCount", { count })}
      </span>
    </div>
  );
}

function useBrowseFilters(defaultProjectId?: string) {
  const [filters, setFilters] = useState(DEFAULT_FILTERS);
  useEffect(() => {
    if (defaultProjectId) {
      setFilters((current) =>
        current.projectId ? current : { ...current, projectId: defaultProjectId },
      );
    }
  }, [defaultProjectId]);
  const update = useCallback(
    <K extends keyof AzureDevOpsFiltersState>(key: K, value: AzureDevOpsFiltersState[K]) =>
      setFilters((current) => ({
        ...current,
        [key]: value,
        ...(key === "projectId" ? { repositoryId: "", teamId: "", iterationId: "" } : {}),
        ...(key === "teamId" ? { iterationId: "" } : {}),
      })),
    [],
  );
  const replace = useCallback((next: AzureDevOpsFiltersState) => setFilters(next), []);
  return { filters, update, replace };
}

function modeForKind(kind: AzureDevOpsScopeSelection["kind"]): AzureDevOpsBrowseMode {
  return kind === "work_item" ? WORK_ITEMS_MODE : PULL_REQUESTS_MODE;
}

function kindForMode(mode: AzureDevOpsBrowseMode): AzureDevOpsScopeSelection["kind"] {
  return mode === WORK_ITEMS_MODE ? "work_item" : "pull_request";
}

function filtersForScope(
  current: AzureDevOpsFiltersState,
  selection: AzureDevOpsScopeSelection,
  views: AzureDevOpsSavedView[],
  queryPresets: AzureDevOpsQueryPresets,
): AzureDevOpsFiltersState | undefined {
  if (selection.source === "saved") {
    const view = views.find((candidate) => candidate.id === selection.id);
    return view ? filtersFromSavedView(current, view) : undefined;
  }
  const preset = presetsForKind(selection.kind, queryPresets).find(
    (candidate) => candidate.value === selection.id,
  );
  return preset ? { ...current, text: "", ...preset.filters } : undefined;
}

/**
 * The selection the scope bar shows: a custom search whose filters match a
 * shortcut shows that shortcut selected.
 */
function shownSelection(
  selection: AzureDevOpsScopeSelection,
  mode: AzureDevOpsBrowseMode,
  filters: AzureDevOpsFiltersState,
  queryPresets: AzureDevOpsQueryPresets,
): AzureDevOpsScopeSelection {
  const kind = mode === WORK_ITEMS_MODE || mode === PULL_REQUESTS_MODE ? kindForMode(mode) : selection.kind;
  if (kind === selection.kind && (selection.source !== "preset" || selection.id)) return selection;
  return { kind, source: "preset", id: matchingPresetId(presetsForKind(kind, queryPresets), filters) };
}

function useDefaultAzureRepository(
  projectId: string,
  repositoryId: string,
  repositories: Array<{ id: string; projectId: string }>,
  update: (key: "repositoryId", value: string) => void,
) {
  useEffect(() => {
    const next = repositoryForProject(projectId, repositoryId, repositories);
    if (next !== undefined) update("repositoryId", next);
  }, [projectId, repositories, repositoryId, update]);
}

type RunAzureSearch = (
  nextSkip?: number,
  override?: { mode: AzureDevOpsBrowseMode; filters: AzureDevOpsFiltersState },
) => void;

function useInitialAzureSearch({
  connection,
  filters,
  mode,
  runSearch,
  ready,
}: {
  connection: ReturnType<typeof useAzureDevOpsConnection>;
  filters: AzureDevOpsFiltersState;
  mode: AzureDevOpsBrowseMode;
  runSearch: RunAzureSearch;
  ready: boolean;
}) {
  const searchedWorkspace = useRef("");
  const connectedWorkspaceId = connection.data?.hasSecret ? connection.data.workspaceId : undefined;
  useEffect(() => {
    if (
      !ready ||
      !connectedWorkspaceId ||
      !filters.projectId ||
      isTaskboardMode(mode) ||
      searchedWorkspace.current === connectedWorkspaceId
    ) {
      return;
    }
    searchedWorkspace.current = connectedWorkspaceId;
    runSearch(0);
  }, [connectedWorkspaceId, filters.projectId, mode, ready, runSearch]);
}

function useAzureScopeControls({
  filters,
  replace,
  runSearch,
  views,
  setMode,
  setSkip,
  queryPresets,
  queryPresetsReady,
}: {
  filters: AzureDevOpsFiltersState;
  replace: (next: AzureDevOpsFiltersState) => void;
  runSearch: RunAzureSearch;
  views: AzureDevOpsSavedView[];
  setMode: (mode: AzureDevOpsBrowseMode) => void;
  setSkip: (skip: number) => void;
  queryPresets: AzureDevOpsQueryPresets;
  queryPresetsReady: boolean;
}) {
  const [selection, setSelection] = useState<AzureDevOpsScopeSelection>({
    kind: "work_item",
    source: "preset",
    id: "recent",
  });
  const selectScope = useCallback(
    (next: AzureDevOpsScopeSelection) => {
      setSelection(next);
      const nextMode = modeForKind(next.kind);
      setMode(nextMode);
      setSkip(0);
      const nextFilters = filtersForScope(filters, next, views, queryPresets);
      if (nextFilters) {
        replace(nextFilters);
        runSearch(0, { mode: nextMode, filters: nextFilters });
      }
    },
    [filters, queryPresets, replace, runSearch, setMode, setSkip, views],
  );
  useEffect(() => {
    if (!queryPresetsReady || selection.source !== "preset" || !selection.id) return;
    const available = presetsForKind(selection.kind, queryPresets);
    if (available.some((preset) => preset.value === selection.id) || !available[0]) return;
    selectScope({ ...selection, id: available[0].value });
  }, [queryPresets, queryPresetsReady, selectScope, selection]);
  const markCustom = useCallback(
    () => setSelection((current) => ({ ...current, source: "preset", id: "" })),
    [],
  );
  return { selection, setSelection, selectScope, markCustom };
}

function useAzureSavedViewControls({
  mode,
  filters,
  savedViews,
  setSelection,
}: {
  mode: AzureDevOpsBrowseMode;
  filters: AzureDevOpsFiltersState;
  savedViews: ReturnType<typeof useAzureDevOpsSavedViews>;
  setSelection: (selection: AzureDevOpsScopeSelection) => void;
}) {
  const [saveViewOpen, setSaveViewOpen] = useState(false);
  const canSaveCurrent =
    !isTaskboardMode(mode) &&
    Boolean(
      filters.projectId &&
        (mode === WORK_ITEMS_MODE
          ? workItemSearchWiql(filters.text, filters.wiql).trim()
          : filters.repositoryId),
    );
  const saveCurrentView = useCallback(
    async (label: string) => {
      const view: AzureDevOpsSavedView = {
        id: `az_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
        kind: kindForMode(mode),
        label,
        projectId: filters.projectId,
        repositoryId: mode === PULL_REQUESTS_MODE ? filters.repositoryId : undefined,
        wiql: mode === WORK_ITEMS_MODE ? workItemSearchWiql(filters.text, filters.wiql) : undefined,
        top: mode === WORK_ITEMS_MODE ? filters.top : undefined,
        status: mode === PULL_REQUESTS_MODE ? filters.status : undefined,
        creator: mode === PULL_REQUESTS_MODE ? filters.creator : undefined,
        reviewer: mode === PULL_REQUESTS_MODE ? filters.reviewer : undefined,
        ...(mode === WORK_ITEMS_MODE ? workItemViewScope(filters) : {}),
        createdAt: new Date().toISOString(),
      };
      await savedViews.replace((current) => [...current, view]);
      setSelection({ kind: view.kind, source: "saved", id: view.id });
    },
    [filters, mode, savedViews, setSelection],
  );
  return { saveViewOpen, setSaveViewOpen, canSaveCurrent, saveCurrentView };
}

/**
 * The work-item WIQL for the filters: the search box or WIQL query, limited
 * to the team's areas and sprint when a team is selected.
 */
async function scopedWorkItemWiql(workspaceId: string | undefined, filters: AzureDevOpsFiltersState): Promise<string> {
  const wiql = workItemSearchWiql(filters.text, filters.wiql);
  if (!workspaceId || !filters.teamId) return wiql;
  const settings = await teamSettingsCache.get(workspaceId, filters.projectId, filters.teamId);
  return wiqlWithConditions(wiql, teamScopeConditions(settings, filters.iterationId));
}

function useAzureSearch({
  workspaceId,
  mode,
  filters,
  workItems,
  pullRequests,
  setSkip,
  setMobileFiltersOpen,
}: {
  workspaceId: string | undefined;
  mode: AzureDevOpsBrowseMode;
  filters: AzureDevOpsFiltersState;
  workItems: ReturnType<typeof useAzureDevOpsWorkItemSearch>;
  pullRequests: ReturnType<typeof useAzureDevOpsPullRequestSearch>;
  setSkip: (skip: number) => void;
  setMobileFiltersOpen: (open: boolean) => void;
}) {
  const [sequence] = useState(createSearchSequence);
  useEffect(() => () => sequence.retire(), [sequence]);
  return useCallback(
    (
      nextSkip: number = 0,
      override?: { mode: AzureDevOpsBrowseMode; filters: AzureDevOpsFiltersState },
    ) => {
      setMobileFiltersOpen(false);
      const searchMode = override?.mode ?? mode;
      const searchFilters = override?.filters ?? filters;
      const isCurrent = sequence.begin();
      if (searchMode === WORK_ITEMS_MODE) {
        void settleScopedSearch(
          scopedWorkItemWiql(workspaceId, searchFilters),
          isCurrent,
          (wiql) => workItems.search({ project: searchFilters.projectId, wiql, top: searchFilters.top }),
          (error: unknown) =>
            currentHost().toast.error(error instanceof Error ? error.message : String(error)),
        );
        return;
      }
      if (isTaskboardMode(searchMode)) return;
      setSkip(nextSkip);
      void pullRequests.search(pullRequestSearchQuery(searchFilters, nextSkip, PAGE_SIZE));
    },
    [filters, mode, pullRequests, sequence, setMobileFiltersOpen, setSkip, workItems, workspaceId],
  );
}

/** Forgets shared reads when the connected organization changes. */
function useClearCachesOnOrganizationChange(organizationUrl: string | undefined) {
  const previous = useRef(organizationUrl);
  useEffect(() => {
    if (organizationUrl && previous.current && previous.current !== organizationUrl) clearRequestCaches();
    if (organizationUrl) previous.current = organizationUrl;
  }, [organizationUrl]);
}

// eslint-disable-next-line max-lines-per-function -- this hook coordinates the page's persisted filters and browse modes.
function useAzureDevOpsPageState(workspaceId?: string) {
  const [mode, setMode] = useState<AzureDevOpsBrowseMode>(BOARD_MODE);
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false);
  const [detailItem, setDetailItem] = useState<AzureDevOpsWorkItem | null>(null);
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const [skip, setSkip] = useState(0);
  const connection = useAzureDevOpsConnection(workspaceId);
  useClearCachesOnOrganizationChange(connection.data?.organizationUrl);
  const projectList = useAzureDevOpsProjects(workspaceId ?? "", !!connection.data?.hasSecret);
  const { filters, update, replace } = useBrowseFilters(
    connection.data?.defaultProjectId || projectList.data[0]?.id,
  );
  const repositoryList = useAzureDevOpsRepositories(workspaceId ?? "", filters.projectId);
  const workItems = useAzureDevOpsWorkItemSearch(workspaceId);
  const pullRequests = useAzureDevOpsPullRequestSearch(workspaceId);
  const feedback = useAzureDevOpsPullRequestFeedback(workspaceId);
  const savedViews = useAzureDevOpsSavedViews(workspaceId);
  const workspaceSettings = useAzureDevOpsWorkspaceSettings(workspaceId);
  const queryPresets = useMemo<AzureDevOpsQueryPresets>(
    () => ({
      workItems: workspaceSettings.settings.workItemQueries,
      pullRequests: workspaceSettings.settings.pullRequestQueries,
    }),
    [workspaceSettings.settings.pullRequestQueries, workspaceSettings.settings.workItemQueries],
  );

  useDefaultAzureRepository(filters.projectId, filters.repositoryId, repositoryList.data, update);

  const runSearch = useAzureSearch({
    workspaceId,
    mode,
    filters,
    workItems,
    pullRequests,
    setSkip,
    setMobileFiltersOpen,
  });

  const openFeedback = (pullRequest: AzureDevOpsPullRequest) =>
    openAzureDevOpsFeedback(feedback, setFeedbackOpen, pullRequest);

  const scope = useAzureScopeControls({
    filters,
    replace,
    runSearch,
    views: savedViews.views,
    setMode,
    setSkip,
    queryPresets,
    queryPresetsReady: !workspaceSettings.loading,
  });
  const savedViewControls = useAzureSavedViewControls({
    mode,
    filters,
    savedViews,
    setSelection: scope.setSelection,
  });

  const pagePreferences = useAzureDevOpsPagePreferences(workspaceId, {
    mode,
    setMode,
    filters,
    replaceFilters: replace,
    selection: scope.selection,
    setSelection: scope.setSelection,
    defaultFilters: DEFAULT_FILTERS,
  });

  useInitialAzureSearch({
    connection,
    filters,
    mode,
    runSearch,
    ready: pagePreferences.hydrated,
  });

  const updateFromUI = useCallback(
    <K extends keyof AzureDevOpsFiltersState>(key: K, value: AzureDevOpsFiltersState[K]) => {
      scope.markCustom();
      update(key, value);
    },
    [scope, update],
  );

  return {
    mode,
    setMode,
    selection: scope.selection,
    selectScope: scope.selectScope,
    saveViewOpen: savedViewControls.saveViewOpen,
    setSaveViewOpen: savedViewControls.setSaveViewOpen,
    mobileFiltersOpen,
    setMobileFiltersOpen,
    detailItem,
    setDetailItem,
    feedbackOpen,
    setFeedbackOpen,
    skip,
    connection,
    projectList,
    filters,
    update: updateFromUI,
    repositoryList,
    workItems,
    pullRequests,
    feedback,
    runSearch,
    openFeedback,
    savedViews,
    canSaveCurrent: savedViewControls.canSaveCurrent,
    saveCurrentView: savedViewControls.saveCurrentView,
    preferencesLoaded: pagePreferences.hydrated,
    boardPreference: pagePreferences.board,
    setBoardPreference: pagePreferences.setBoard,
    teamByProject: pagePreferences.teamByProject,
    rememberTeam: pagePreferences.rememberTeam,
    expandChildren: pagePreferences.expandChildren,
    rememberExpandChildren: pagePreferences.rememberExpandChildren,
    sprintsPreference: pagePreferences.sprints,
    setSprintsPreference: pagePreferences.setSprints,
    workItemActions: workspaceSettings.settings.workItemActions,
    pullRequestActions: workspaceSettings.settings.pullRequestActions,
    queryPresets,
  };
}

function openAzureDevOpsFeedback(
  feedback: ReturnType<typeof useAzureDevOpsPullRequestFeedback>,
  setFeedbackOpen: (open: boolean) => void,
  pullRequest: AzureDevOpsPullRequest,
) {
  setFeedbackOpen(true);
  void feedback.load(pullRequest);
}

type PageState = ReturnType<typeof useAzureDevOpsPageState>;

type Launch = (payload: AzureDevOpsLaunchPayload) => void;

function BrowseResults({ state, launch }: { state: PageState; launch: Launch }) {
  if (isTaskboardMode(state.mode)) return null;
  return (
    <section className="flex min-w-0 flex-1 flex-col overflow-hidden">
      <ResultHeader
        mode={state.mode}
        count={
          state.mode === WORK_ITEMS_MODE
            ? state.workItems.data.length
            : state.pullRequests.data.length
        }
      />
      <div className="min-h-0 flex-1 overflow-y-auto">
        {state.mode === WORK_ITEMS_MODE ? (
          <AzureDevOpsWorkItemResults
            items={state.workItems.data}
            loading={state.workItems.loading}
            error={state.workItems.error}
            onOpenDetail={state.setDetailItem}
            quickActions={state.workItemActions}
            onStartTask={(item) => launch({ kind: "work-item", item })}
            onQuickAction={(item, action) => launch({ kind: "work-item", item, action })}
          />
        ) : (
          <AzureDevOpsPullRequestResults
            items={state.pullRequests.data}
            loading={state.pullRequests.loading}
            error={state.pullRequests.error}
            onFeedback={state.openFeedback}
            quickActions={state.pullRequestActions}
            onStartTask={(pullRequest) => launch({ kind: "pull-request", pullRequest })}
            onQuickAction={(pullRequest, action) => launch({ kind: "pull-request", pullRequest, action })}
          />
        )}
      </div>
      {state.mode === PULL_REQUESTS_MODE && (
        <AzureDevOpsPullRequestPagination
          skip={state.skip}
          count={state.pullRequests.data.length}
          loading={state.pullRequests.loading}
          pageSize={PAGE_SIZE}
          onPage={state.runSearch}
        />
      )}
    </section>
  );
}

function MobileFilters({
  state,
  filterProps,
}: {
  state: PageState;
  filterProps: Omit<FiltersProps, "idSuffix">;
}) {
  const t = useT();
  return (
    <Sheet open={state.mobileFiltersOpen} onOpenChange={state.setMobileFiltersOpen}>
      <SheetContent side="left" className="w-80 max-w-[90vw] overflow-y-auto">
        <SheetHeader className="mb-5 text-left">
          <SheetTitle>{t("azureDevopsFilters")}</SheetTitle>
        </SheetHeader>
        <AzureDevOpsFilters {...filterProps} idSuffix="-mobile" />
      </SheetContent>
    </Sheet>
  );
}

// eslint-disable-next-line max-lines-per-function -- the page content owns the responsive browse composition.
function AzureDevOpsPageContent({ workspaceId }: PageProps) {
  const t = useT();
  const state = useAzureDevOpsPageState(workspaceId);
  const [launchPayload, setLaunchPayload] = useState<AzureDevOpsLaunchPayload | null>(null);
  const launchWorkItem = (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) =>
    setLaunchPayload({ kind: "work-item", item, ...(action ? { action } : {}) });
  const filterTeams = useTeamList(workspaceId, state.mode === WORK_ITEMS_MODE ? state.filters.projectId : "");
  const filterTeamSettings = useTeamSettings(workspaceId, state.filters.projectId, state.filters.teamId);
  const teamFilterOptions = useMemo(
    () => teamPickerOptions(filterTeams.data?.teams ?? [], filterTeams.data?.mineIds ?? [], t("myTeams")),
    [filterTeams.data, t],
  );
  const sprintFilterOptions = useSprintOptions(filterTeamSettings.data?.iterations ?? EMPTY_ITERATIONS);
  const pullRequestPeople = usePullRequestPeople(
    workspaceId,
    state.mode === PULL_REQUESTS_MODE && state.filters.repositoryId ? state.filters.projectId : "",
    state.filters.repositoryId,
  );
  const identity = useAzureIdentity(state.mode === PULL_REQUESTS_MODE ? workspaceId : undefined);

  if (!state.preferencesLoaded) return null;
  if (state.connection.loading) return null;
  if (!state.connection.data?.hasSecret) return <NotConfigured workspaceId={workspaceId} />;

  const searchLoading =
    state.mode === WORK_ITEMS_MODE ? state.workItems.loading : state.pullRequests.loading;
  const filterProps = {
    mode: state.mode,
    filters: state.filters,
    projects: state.projectList.data,
    repositories: state.repositoryList.data,
    loading: searchLoading || filterTeamSettings.loading,
    onChange: ((key, value) => {
      state.update(key, value);
      if (key === "teamId" && typeof value === "string") state.rememberTeam(state.filters.projectId, value);
    }) as FiltersProps["onChange"],
    onSearch: () => state.runSearch(0),
    teams: teamFilterOptions,
    sprints: sprintFilterOptions,
    people: pullRequestPeople.data,
    me: identity.data,
  };

  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="azure-devops-page">
      <div className="flex items-center justify-between gap-2 border-b px-4 py-2">
        <p className="min-w-0 truncate text-xs text-muted-foreground">
          {t("boardsAndRepos", { organizationUrl: state.connection.data.organizationUrl })}
        </p>
        <Button
          type="button"
          variant="outline"
          size="icon"
          onClick={() => state.setMobileFiltersOpen(true)}
          className="cursor-pointer md:hidden"
          aria-label={t("openAzureDevopsFilters")}
          data-testid="azure-devops-mobile-filter-button"
        >
          <IconAdjustments className="h-4 w-4" />
        </Button>
      </div>
      <AzureDevOpsScopeBar
        selected={shownSelection(state.selection, state.mode, state.filters, state.queryPresets)}
        onSelect={state.selectScope}
        savedViews={state.savedViews.views}
        onDeleteSaved={(id) => {
          void state.savedViews.remove(id).catch((error: unknown) => {
            currentHost().toast.error(t("failedToDeleteSavedQuery"), {
              description: error instanceof Error ? error.message : undefined,
            });
          });
        }}
        canSaveCurrent={state.canSaveCurrent}
        onSaveCurrent={() => state.setSaveViewOpen(true)}
        queryPresets={state.queryPresets}
      />
      <AzureDevOpsModeTabs mode={state.mode} onModeChange={state.setMode} />
      {state.mode === BOARD_MODE && (
        <AzureDevOpsBoard
          workspaceId={workspaceId}
          projectId={state.filters.projectId}
          projects={state.projectList.data}
          onProjectChange={(projectId) => state.update("projectId", projectId)}
          initialPreference={state.boardPreference}
          onPreferenceChange={state.setBoardPreference}
          quickActions={state.workItemActions}
          onStartTask={launchWorkItem}
          rememberedTeamId={state.teamByProject[state.filters.projectId]}
          onTeamChosen={(teamId) => state.rememberTeam(state.filters.projectId, teamId)}
          expandChildrenByBoard={state.expandChildren}
          onExpandChildrenChange={state.rememberExpandChildren}
        />
      )}
      {state.mode === SPRINTS_MODE && (
        <AzureDevOpsSprints
          workspaceId={workspaceId}
          projectId={state.filters.projectId}
          projects={state.projectList.data}
          onProjectChange={(projectId) => state.update("projectId", projectId)}
          rememberedTeamId={state.teamByProject[state.filters.projectId]}
          onTeamChosen={(teamId) => state.rememberTeam(state.filters.projectId, teamId)}
          preference={state.sprintsPreference}
          onPreferenceChange={state.setSprintsPreference}
          quickActions={state.workItemActions}
          onStartTask={launchWorkItem}
          onOpenDetail={state.setDetailItem}
        />
      )}
      {!isTaskboardMode(state.mode) && (
        <div className="hidden border-b px-4 py-3 md:block">
          <AzureDevOpsFilters {...filterProps} idSuffix="" compact />
        </div>
      )}
      <BrowseResults state={state} launch={setLaunchPayload} />
      {!isTaskboardMode(state.mode) && <MobileFilters state={state} filterProps={filterProps} />}
      <AzureDevOpsFeedbackDialog
        open={state.feedbackOpen}
        loading={state.feedback.loading}
        error={state.feedback.error}
        feedback={state.feedback.data}
        onOpenChange={(open) => {
          state.setFeedbackOpen(open);
          if (!open) state.feedback.clear();
        }}
      />
      <AzureDevOpsWorkItemDetail
        open={!!state.detailItem}
        onOpenChange={(open) => !open && state.setDetailItem(null)}
        workspaceId={workspaceId}
        projectId={state.filters.projectId}
        initialItem={state.detailItem}
        quickActions={state.workItemActions}
        onStartTask={(item, action) => {
          state.setDetailItem(null);
          launchWorkItem(item, action);
        }}
      />
      <AzureDevOpsTaskLauncher
        workspaceId={workspaceId}
        organizationUrl={state.connection.data.organizationUrl}
        payload={launchPayload}
        onClose={() => setLaunchPayload(null)}
      />
      <AzureDevOpsSaveViewDialog
        open={state.saveViewOpen}
        kind={state.selection.kind}
        onOpenChange={state.setSaveViewOpen}
        onSave={state.saveCurrentView}
      />
    </div>
  );
}

/** The /azure-devops route for the active workspace. */
export function AzureDevOpsPage() {
  // Each visit reads identity, teams, and sprint settings again: the
  // connection may have been signed in to another account meanwhile.
  useState(clearRequestCaches);
  const [workspaceId, setWorkspaceId] = useState(() => currentHost().context.getActiveWorkspaceId());
  useEffect(() => currentHost().context.subscribeActiveWorkspace(setWorkspaceId), []);
  return <AzureDevOpsPageContent key={workspaceId ?? ""} workspaceId={workspaceId} />;
}
