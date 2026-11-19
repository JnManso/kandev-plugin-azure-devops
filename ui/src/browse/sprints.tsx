import { useEffect, useMemo, useState } from "../runtime";
import { useResponsiveBreakpoint } from "../runtime";
import { IconChevronDown, IconChevronRight } from "../icons";
import { Alert, AlertDescription, Badge, Button, Card, CardContent, CardHeader, CardTitle, Spinner } from "../host-ui";
import { cn } from "../cn";
import { useT } from "../i18n";
import { SearchablePicker } from "./searchable-picker";
import { useSprintOptions } from "./sprint-options";
import { ANY_OPTION, UNASSIGNED_PERSON, filterRowsByPerson, sprintDateRange, tasksInColumn, teamPickerOptions } from "./sprints-model";
import { WorkItemTaskActions } from "./task-actions";
import type {
  AzureDevOpsActionPreset,
  AzureDevOpsProject,
  AzureDevOpsSprintBoard,
  AzureDevOpsSprintRow,
  AzureDevOpsTaskboardColumn,
  AzureDevOpsWorkItem,
} from "./types";
import type { AzureDevOpsSprintsPreference } from "./use-preferences";
import { useAzureDevOpsSprints } from "./use-sprints";

type StartTask = (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;

type Props = {
  workspaceId?: string;
  projectId: string;
  projects: AzureDevOpsProject[];
  onProjectChange: (projectId: string) => void;
  rememberedTeamId?: string;
  onTeamChosen: (teamId: string) => void;
  preference: AzureDevOpsSprintsPreference;
  onPreferenceChange: (preference: AzureDevOpsSprintsPreference) => void;
  quickActions: AzureDevOpsActionPreset[];
  onStartTask: StartTask;
  onOpenDetail: (item: AzureDevOpsWorkItem) => void;
};

const UNPARENTED_ROW = "unparented";

const rowKey = (row: AzureDevOpsSprintRow) => (row.parent ? String(row.parent.id) : UNPARENTED_ROW);

function SprintCard({
  item,
  quickActions,
  onStartTask,
  onOpenDetail,
}: {
  item: AzureDevOpsWorkItem;
  quickActions: AzureDevOpsActionPreset[];
  onStartTask: StartTask;
  onOpenDetail: (item: AzureDevOpsWorkItem) => void;
}) {
  const t = useT();
  return (
    <Card
      role="button"
      tabIndex={0}
      onClick={() => onOpenDetail(item)}
      onKeyDown={(event: KeyboardEvent) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpenDetail(item);
        }
      }}
      className="cursor-pointer gap-2 py-3 transition hover:border-primary/50"
      data-testid={`azure-sprint-card-${item.id}`}
    >
      <CardHeader className="px-3 py-0">
        <CardTitle className="line-clamp-2 text-sm font-medium">{item.title || t("untitledWorkItem")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 px-3 text-xs text-muted-foreground">
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="outline">#{item.id}</Badge>
          {item.type && <Badge variant="secondary">{item.type}</Badge>}
          <span className="truncate">{item.assignedTo || t("unassigned")}</span>
        </div>
        <div className="flex justify-end">
          <WorkItemTaskActions item={item} quickActions={quickActions} onStart={onStartTask} />
        </div>
      </CardContent>
    </Card>
  );
}

type CardProps = Pick<Parameters<typeof SprintCard>[0], "quickActions" | "onStartTask" | "onOpenDetail">;

function RowHeader({
  row,
  collapsed,
  onToggle,
}: {
  row: AzureDevOpsSprintRow;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const title = row.parent ? row.parent.title || t("untitledWorkItem") : t("unparented");
  return (
    <button
      type="button"
      className="flex w-full cursor-pointer items-center gap-2 rounded-md px-1 py-1.5 text-left text-sm font-medium hover:bg-muted"
      onClick={onToggle}
      aria-expanded={!collapsed}
      aria-label={t(collapsed ? "expandRow" : "collapseRow", { title })}
      data-testid={`azure-sprint-row-toggle-${rowKey(row)}`}
    >
      {collapsed ? <IconChevronRight className="h-4 w-4 shrink-0" /> : <IconChevronDown className="h-4 w-4 shrink-0" />}
      {row.parent && <span className="font-mono text-xs text-muted-foreground">#{row.parent.id}</span>}
      <span className="min-w-0 truncate">{title}</span>
      <Badge variant="outline" className="ml-auto">
        {row.tasks.length}
      </Badge>
    </button>
  );
}


function DesktopTaskboard({
  board,
  rows,
  collapsed,
  onToggle,
  cardProps,
}: {
  board: AzureDevOpsSprintBoard;
  rows: AzureDevOpsSprintRow[];
  collapsed: Set<string>;
  onToggle: (key: string) => void;
  cardProps: CardProps;
}) {
  const t = useT();
  const grid = { gridTemplateColumns: `minmax(14rem, 1.2fr) repeat(${board.columns.length}, minmax(11rem, 1fr))` };
  return (
    <div className="min-h-0 flex-1 overflow-auto" data-testid="azure-sprint-taskboard">
      <div className="grid gap-2 border-b pb-2 text-xs font-semibold text-muted-foreground" style={grid}>
        <div>{t("backlogItem")}</div>
        {board.columns.map((column) => (
          <div key={column.id} data-testid={`azure-sprint-column-${column.id}`}>
            {column.name}
          </div>
        ))}
      </div>
      {rows.map((row) => {
        const key = rowKey(row);
        return (
          <div key={key} className="border-b py-2" data-testid={`azure-sprint-row-${key}`}>
            <RowHeader row={row} collapsed={collapsed.has(key)} onToggle={() => onToggle(key)} />
            {!collapsed.has(key) && (
              <div className="mt-2 grid gap-2" style={grid}>
                <div>{row.parent && <SprintCard item={row.parent} {...cardProps} />}</div>
                {board.columns.map((column) => (
                  <div key={column.id} className="space-y-2">
                    {tasksInColumn(row, board.columns, column.id).map((task) => (
                      <SprintCard key={task.id} item={task} {...cardProps} />
                    ))}
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function PhoneTaskboard({
  board,
  rows,
  collapsed,
  onToggle,
  cardProps,
}: {
  board: AzureDevOpsSprintBoard;
  rows: AzureDevOpsSprintRow[];
  collapsed: Set<string>;
  onToggle: (key: string) => void;
  cardProps: CardProps;
}) {
  return (
    <div className="min-h-0 flex-1 space-y-2 overflow-y-auto" data-testid="azure-sprint-taskboard">
      {rows.map((row) => {
        const key = rowKey(row);
        return (
          <div key={key} className="rounded-md border p-2" data-testid={`azure-sprint-row-${key}`}>
            <RowHeader row={row} collapsed={collapsed.has(key)} onToggle={() => onToggle(key)} />
            {!collapsed.has(key) && (
              <div className="mt-2 space-y-3">
                {row.parent && <SprintCard item={row.parent} {...cardProps} />}
                {board.columns.map((column) => {
                  const tasks = tasksInColumn(row, board.columns, column.id);
                  if (tasks.length === 0) return null;
                  return (
                    <section key={column.id} className="space-y-2" data-testid={`azure-sprint-group-${key}-${column.id}`}>
                      <h4 className="text-xs font-semibold text-muted-foreground">
                        {column.name} · {tasks.length}
                      </h4>
                      {tasks.map((task) => (
                        <SprintCard key={task.id} item={task} {...cardProps} />
                      ))}
                    </section>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function useCollapsedRows(rows: AzureDevOpsSprintRow[]) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const keys = useMemo(() => rows.map(rowKey), [rows]);
  const allCollapsed = keys.length > 0 && keys.every((key) => collapsed.has(key));
  const toggle = (key: string) =>
    setCollapsed((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  const toggleAll = () => setCollapsed(allCollapsed ? new Set() : new Set(keys));
  return { collapsed, allCollapsed, toggle, toggleAll };
}

function SprintSummary({ board }: { board: AzureDevOpsSprintBoard }) {
  const t = useT();
  const range = sprintDateRange(board.iteration.start, board.iteration.finish);
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm" data-testid="azure-sprint-summary">
      <span className="font-semibold">{board.iteration.name}</span>
      {range && <span className="text-muted-foreground">{range}</span>}
      <Badge variant="secondary" data-testid="azure-sprint-days-left">
        {t("workingDaysLeft", { count: board.workingDaysLeft })}
      </Badge>
      {board.truncated && <span className="text-xs text-muted-foreground">{t("sprintTruncated")}</span>}
    </div>
  );
}

function SprintMessage({ children, testId }: { children: string; testId: string }) {
  return (
    <div className="p-6 text-sm text-muted-foreground" data-testid={testId}>
      {children}
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function -- the Sprints view composes selectors, states, and both layouts.
export function AzureDevOpsSprints({
  workspaceId,
  projectId,
  projects,
  onProjectChange,
  rememberedTeamId,
  onTeamChosen,
  preference,
  onPreferenceChange,
  quickActions,
  onStartTask,
  onOpenDetail,
}: Props) {
  const t = useT();
  const { isMobile } = useResponsiveBreakpoint();
  const sprints = useAzureDevOpsSprints(workspaceId, projectId, rememberedTeamId, preference);
  const teamOptions = useMemo(
    () => teamPickerOptions(sprints.teams, sprints.mineIds, t("myTeams")),
    [sprints.mineIds, sprints.teams, t],
  );
  const sprintOptions = useSprintOptions(sprints.iterations);
  const people = sprints.board?.people ?? [];
  const personOptions = useMemo(
    () => [
      { id: ANY_OPTION, name: t("allPeople") },
      { id: UNASSIGNED_PERSON, name: t("unassigned") },
      ...people.map((name) => ({ id: name, name })),
    ],
    [people, t],
  );
  const rows = useMemo(
    () => filterRowsByPerson(sprints.board?.rows ?? [], sprints.person),
    [sprints.board, sprints.person],
  );
  const collapse = useCollapsedRows(rows);

  useEffect(() => {
    if (!sprints.iterationId) return;
    onPreferenceChange({ iterationId: sprints.iterationId, person: sprints.person });
  }, [onPreferenceChange, sprints.iterationId, sprints.person]);

  const picker = cn("w-full", !isMobile && "w-52");
  const loading = sprints.teamsLoading || sprints.settingsLoading || sprints.boardLoading;
  const cardProps = { quickActions, onStartTask, onOpenDetail };
  return (
    <section className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden p-4" data-testid="azure-devops-sprints">
      <div className={cn("flex gap-2", isMobile ? "flex-col" : "flex-wrap")} data-testid="azure-sprint-selectors">
        <SearchablePicker
          value={projectId}
          options={projects}
          onChange={onProjectChange}
          placeholder={t("project")}
          testId="azure-sprint-project-select"
          className={picker}
        />
        <SearchablePicker
          value={sprints.teamId}
          options={teamOptions}
          onChange={(teamId) => {
            sprints.setTeamId(teamId);
            onTeamChosen(teamId);
          }}
          placeholder={t("team")}
          testId="azure-sprint-team-select"
          className={picker}
          disabled={teamOptions.length === 0}
        />
        <SearchablePicker
          value={sprints.iterationId}
          options={sprintOptions}
          onChange={sprints.setIterationId}
          placeholder={t("sprint")}
          testId="azure-sprint-select"
          className={picker}
          disabled={sprintOptions.length === 0}
        />
        <SearchablePicker
          value={sprints.person}
          options={personOptions}
          onChange={sprints.setPerson}
          placeholder={t("person")}
          testId="azure-sprint-person-select"
          className={picker}
          disabled={!sprints.board}
        />
      </div>
      {sprints.board && (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <SprintSummary board={sprints.board} />
          {rows.length > 0 && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="cursor-pointer"
              onClick={collapse.toggleAll}
              data-testid="azure-sprint-collapse-all"
            >
              {t(collapse.allCollapsed ? "expandAll" : "collapseAll")}
            </Button>
          )}
        </div>
      )}
      {sprints.error && (
        <Alert variant="destructive">
          <AlertDescription>{sprints.error}</AlertDescription>
        </Alert>
      )}
      {loading && (
        <div
          className="flex items-center gap-2 p-6 text-sm text-muted-foreground"
          role="status"
          data-testid="azure-sprint-loading"
        >
          <Spinner className="h-4 w-4" />
          {t("loadingSprint")}
        </div>
      )}
      {!loading && !sprints.error && sprints.teamId && sprints.iterations.length === 0 && (
        <SprintMessage testId="azure-sprint-no-sprints">{t("noSprints")}</SprintMessage>
      )}
      {!loading && sprints.board && sprints.board.rows.length === 0 && (
        <SprintMessage testId="azure-sprint-empty">{t("noSprintWorkItems")}</SprintMessage>
      )}
      {!loading && sprints.board && sprints.board.rows.length > 0 && rows.length === 0 && (
        <SprintMessage testId="azure-sprint-no-person-work">{t("noSprintWorkForPerson")}</SprintMessage>
      )}
      {!loading &&
        sprints.board &&
        rows.length > 0 &&
        (isMobile ? (
          <PhoneTaskboard board={sprints.board} rows={rows} cardProps={cardProps} collapsed={collapse.collapsed} onToggle={collapse.toggle} />
        ) : (
          <DesktopTaskboard board={sprints.board} rows={rows} cardProps={cardProps} collapsed={collapse.collapsed} onToggle={collapse.toggle} />
        ))}
    </section>
  );
}
