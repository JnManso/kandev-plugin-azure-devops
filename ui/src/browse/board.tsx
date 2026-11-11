
import { useEffect, useMemo, useRef, useState } from "../runtime";
import type { ReactNode } from "../runtime";
import { IconChevronLeft, IconChevronRight, IconGripVertical, IconList } from "../icons";
import { Alert, AlertDescription, Spinner } from "../host-ui";
import { Badge } from "../host-ui";
import { Button } from "../host-ui";
import { Card, CardContent, CardHeader, CardTitle } from "../host-ui";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "../host-ui";
import { SearchablePicker, type PickerOption } from "./searchable-picker";
import { teamPickerOptions } from "./sprints-model";
import { WorkItemTaskActions } from "./task-actions";
import { CardChildrenList, CardChildrenToggle, type ChildTreeView } from "./board-card-children";
import { everyCardExpanded, isCardExpanded } from "./board-children";
import { useBoardChildren } from "./use-board-children";
import { useAzureDevOpsBoard } from "./use-board";
import type { AzureDevOpsBoardPreference } from "./use-preferences";
import { useResponsiveBreakpoint } from "../runtime";
import type {
  AzureDevOpsActionPreset,
  AzureDevOpsBoard,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsChildCard,
  AzureDevOpsProject,
  AzureDevOpsWorkItem,
} from "./types";
import { groupAzureDevOpsBoardItems } from "./board-view";
import { boardPreferenceFor } from "./board-discovery";
import { AzureDevOpsWorkItemDetail } from "./work-item-detail";
import { useT } from "../i18n";

type Props = {
  workspaceId?: string;
  projectId: string;
  projects: AzureDevOpsProject[];
  onProjectChange: (projectId: string) => void;
  initialPreference?: AzureDevOpsBoardPreference;
  onPreferenceChange: (preference: AzureDevOpsBoardPreference) => void;
  quickActions?: AzureDevOpsActionPreset[];
  onStartTask?: (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;
  /** The team last chosen in this project by any team selector. */
  rememberedTeamId?: string;
  onTeamChosen?: (teamId: string) => void;
  /** Whether each board, by boardChildrenKey, opens with its cards expanded. */
  expandChildrenByBoard?: Record<string, boolean>;
  onExpandChildrenChange?: (key: string, expand: boolean) => void;
};

/** The key a board's expand-all choice is stored under. */
export function boardChildrenKey(projectId: string, teamId: string, boardId: string): string {
  return `${projectId}/${teamId}/${boardId}`;
}

type CardChildren = {
  items?: AzureDevOpsChildCard[];
  expanded: boolean;
  onToggle: () => void;
  tree: ChildTreeView;
};

// eslint-disable-next-line max-lines-per-function -- one card owns its open, drag, task, and children controls.
function BoardCard({
  item,
  onOpen,
  onDragStart,
  quickActions,
  onStartTask,
  childCards,
}: {
  item: AzureDevOpsBoardWorkItem;
  onOpen: () => void;
  onDragStart: () => void;
  quickActions: AzureDevOpsActionPreset[];
  onStartTask?: (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;
  childCards: CardChildren;
}) {
  const t = useT();
  const pointerStart = useRef<{ x: number; y: number } | null>(null);
  const handlePointerDown = (event: PointerEvent) => {
    pointerStart.current = { x: event.clientX, y: event.clientY };
  };
  const handlePointerUp = (event: PointerEvent) => {
    const start = pointerStart.current;
    pointerStart.current = null;
    if (!start || Math.hypot(event.clientX - start.x, event.clientY - start.y) < 6) onOpen();
  };
  return (
    <Card
      draggable
      role="button"
      tabIndex={0}
      onDragStart={onDragStart}
      onPointerDown={handlePointerDown}
      onPointerUp={handlePointerUp}
      onClick={onOpen}
      onKeyDown={(event: KeyboardEvent) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
      className="cursor-pointer gap-2 py-3 transition hover:border-primary/50"
      data-testid={`azure-board-card-${item.id}`}
    >
      <CardHeader className="px-3 py-0">
        <CardTitle className="flex items-start gap-2 text-sm font-medium">
          <IconGripVertical className="mt-0.5 hidden h-4 w-4 shrink-0 text-muted-foreground md:block" />
          <span className="line-clamp-2">{item.title || t("untitledWorkItem")}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 px-3 text-xs text-muted-foreground">
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="outline">#{item.id}</Badge>
          {item.type && <Badge variant="secondary">{item.type}</Badge>}
          {item.assignedTo && <span className="truncate">{item.assignedTo}</span>}
        </div>
        {!!item.tags?.length && <div className="truncate">{item.tags.join(" · ")}</div>}
        {(onStartTask || !!childCards.items?.length) && (
          <div className="flex items-center justify-between gap-2">
            {childCards.items?.length ? (
              <CardChildrenToggle
                parentId={item.id}
                items={childCards.items}
                expanded={childCards.expanded}
                onToggle={childCards.onToggle}
              />
            ) : (
              <span />
            )}
            {onStartTask && <WorkItemTaskActions item={item} quickActions={quickActions} onStart={onStartTask} />}
          </div>
        )}
        {childCards.expanded && !!childCards.items?.length && (
          <CardChildrenList parentId={item.id} items={childCards.items} tree={childCards.tree} />
        )}
      </CardContent>
    </Card>
  );
}

function MobileColumnNavigator({
  columns,
  selectedColumn,
  counts,
  onChange,
}: {
  columns: AzureDevOpsBoard["columns"];
  selectedColumn: string;
  counts: Map<string, AzureDevOpsBoardWorkItem[]>;
  onChange: (columnId: string) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const index = Math.max(
    0,
    columns.findIndex((column) => column.id === selectedColumn),
  );
  const column = columns[index];
  const select = (columnId: string) => {
    onChange(columnId);
    setOpen(false);
  };
  return (
    <div className="flex items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="cursor-pointer"
        aria-label={t("previousBoardColumn")}
        disabled={index === 0}
        onClick={() => onChange(columns[index - 1].id)}
      >
        <IconChevronLeft className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="outline"
        className="flex-1 cursor-pointer justify-between"
        data-testid="azure-board-column-picker"
        onClick={() => setOpen(true)}
      >
        <span>{column?.name ?? t("boardColumn")}</span>
        <span>{counts.get(column?.id ?? "")?.length ?? 0}</span>
        <IconList className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="cursor-pointer"
        aria-label={t("nextBoardColumn")}
        disabled={index >= columns.length - 1}
        onClick={() => onChange(columns[index + 1].id)}
      >
        <IconChevronRight className="h-4 w-4" />
      </Button>
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerContent className="max-h-[min(32rem,calc(100dvh-16px-env(safe-area-inset-bottom,0px)))]">
          <DrawerHeader>
            <DrawerTitle>{t("boardColumns")}</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 overflow-y-auto p-4 pb-[env(safe-area-inset-bottom,0px)]">
            {columns.map((candidate) => (
              <Button
                key={candidate.id}
                type="button"
                variant={candidate.id === selectedColumn ? "default" : "outline"}
                className="mb-2 min-h-11 w-full cursor-pointer justify-between"
                data-testid={`azure-board-column-option-${candidate.id}`}
                onClick={() => select(candidate.id)}
              >
                <span>{candidate.name}</span>
                <Badge variant="secondary">{counts.get(candidate.id)?.length ?? 0}</Badge>
              </Button>
            ))}
          </div>
        </DrawerContent>
      </Drawer>
    </div>
  );
}

function BoardSelectors({
  projectId,
  projects,
  onProjectChange,
  teamId,
  teams,
  onTeamChange,
  boardId,
  boards,
  onBoardChange,
}: {
  projectId: string;
  projects: AzureDevOpsProject[];
  onProjectChange: (value: string) => void;
  teamId: string;
  teams: PickerOption[];
  onTeamChange: (value: string) => void;
  boardId: string;
  boards: Array<{ id: string; name: string }>;
  onBoardChange: (value: string) => void;
}) {
  const t = useT();
  return (
    <div className="flex flex-wrap gap-2">
      <SearchablePicker
        value={projectId}
        options={projects}
        onChange={onProjectChange}
        placeholder={t("project")}
        testId="azure-board-project-select"
        className="w-52"
      />
      <SearchablePicker
        value={teamId}
        options={teams}
        onChange={onTeamChange}
        placeholder={t("team")}
        testId="azure-board-team-select"
        className="w-52"
        disabled={!teams.length}
      />
      <SearchablePicker
        value={boardId}
        options={boards}
        onChange={onBoardChange}
        placeholder={t("board")}
        testId="azure-board-select"
        className="w-52"
        disabled={!boards.length}
      />
    </div>
  );
}

function preferredColumn(
  columns: AzureDevOpsBoard["columns"],
  selectedColumn: string,
  initialPreference: AzureDevOpsBoardPreference | undefined,
): string {
  if (columns.some((column) => column.id === selectedColumn)) return selectedColumn;
  if (columns.some((column) => column.id === initialPreference?.focusedColumnId)) {
    return initialPreference?.focusedColumnId ?? "";
  }
  return columns[0]?.id ?? "";
}

function useBoardPreference(
  projectId: string,
  board: ReturnType<typeof useAzureDevOpsBoard>,
  columns: AzureDevOpsBoard["columns"],
  initialPreference: AzureDevOpsBoardPreference | undefined,
  onPreferenceChange: (preference: AzureDevOpsBoardPreference) => void,
) {
  const [selectedColumn, setSelectedColumn] = useState("");
  const effectiveColumn = preferredColumn(columns, selectedColumn, initialPreference);

  useEffect(() => {
    if (effectiveColumn !== selectedColumn) setSelectedColumn(effectiveColumn);
  }, [effectiveColumn, selectedColumn]);

  useEffect(() => {
    if (!board.teamId || !board.boardId || !effectiveColumn) return;
    onPreferenceChange(
      boardPreferenceFor({
        projectId,
        teams: board.teams,
        teamId: board.teamId,
        boards: board.boards,
        boardId: board.boardId,
        focusedColumnId: effectiveColumn,
      }),
    );
  }, [board.boardId, board.boards, board.teamId, board.teams, effectiveColumn, onPreferenceChange, projectId]);

  return { effectiveColumn, setSelectedColumn };
}

function useBoardGroups(snapshot: ReturnType<typeof useAzureDevOpsBoard>["snapshot"]) {
  return useMemo(
    () =>
      snapshot
        ? groupAzureDevOpsBoardItems(snapshot.board, snapshot.items)
        : new Map<string, AzureDevOpsBoardWorkItem[]>(),
    [snapshot],
  );
}

function useBoardPresentation(
  projectId: string,
  board: ReturnType<typeof useAzureDevOpsBoard>,
  initialPreference: AzureDevOpsBoardPreference | undefined,
  onPreferenceChange: (preference: AzureDevOpsBoardPreference) => void,
) {
  const groups = useBoardGroups(board.snapshot);
  const columns = board.snapshot?.board.columns ?? [];
  const columnPreference = useBoardPreference(
    projectId,
    board,
    columns,
    initialPreference,
    onPreferenceChange,
  );
  return { groups, columns, columnPreference };
}

type Toggles = { key: string; cards: Record<number, boolean>; lines: Record<number, boolean> };

/**
 * The board's card children, their expansion (cards follow expand all unless
 * toggled; child lines expand one by one), and the child detail.
 */
// eslint-disable-next-line max-lines-per-function -- one owner for the child tree state of a board.
function useChildPresentation(
  workspaceId: string | undefined,
  projectId: string,
  board: ReturnType<typeof useAzureDevOpsBoard>,
  options: {
    expandChildrenByBoard?: Record<string, boolean>;
    onExpandChildrenChange?: (key: string, expand: boolean) => void;
    quickActions: AzureDevOpsActionPreset[];
    onStartTask?: (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;
  },
) {
  const key = boardChildrenKey(projectId, board.teamId, board.boardId);
  const expandAll = Boolean(options.expandChildrenByBoard?.[key]);
  const [toggles, setToggles] = useState<Toggles>({ key, cards: {}, lines: {} });
  const current = toggles.key === key ? toggles : { key, cards: {}, lines: {} };
  const cardIds = useMemo(() => (board.snapshot?.items ?? []).map((item) => item.id), [board.snapshot]);
  const expandedIds = [
    ...cardIds.filter((id) => isCardExpanded(id, expandAll, current.cards)),
    ...Object.keys(current.lines)
      .map(Number)
      .filter((id) => current.lines[id]),
  ];
  const children = useBoardChildren(workspaceId, projectId, board.snapshot?.items, expandedIds);
  const parentIds = cardIds.filter((id) => !!children.byParent.get(id)?.length);
  const allExpanded = everyCardExpanded(parentIds, expandAll, current.cards);
  const [detail, setDetail] = useState<AzureDevOpsChildCard | null>(null);
  const tree: ChildTreeView = {
    childrenOf: (id) => children.byParent.get(id),
    isLineExpanded: (id) => Boolean(current.lines[id]),
    toggleLine: (id) => setToggles({ ...current, lines: { ...current.lines, [id]: !current.lines[id] } }),
    onOpen: setDetail,
    quickActions: options.quickActions,
    onStartTask: options.onStartTask,
  };
  const childCardsFor = (item: AzureDevOpsBoardWorkItem): CardChildren => {
    const expanded = isCardExpanded(item.id, expandAll, current.cards);
    return {
      items: children.byParent.get(item.id),
      expanded,
      onToggle: () => setToggles({ ...current, cards: { ...current.cards, [item.id]: !expanded } }),
      tree,
    };
  };
  return {
    failed: children.failed,
    truncated: children.truncated,
    hasChildren: parentIds.length > 0,
    allExpanded,
    toggleAll: () => {
      options.onExpandChildrenChange?.(key, !allExpanded);
      setToggles({ ...current, cards: {} });
    },
    childCardsFor,
    detail,
    setDetail,
  };
}

function ChildNotices({ failed, truncated }: { failed: boolean; truncated: boolean }) {
  const t = useT();
  if (!failed && !truncated) return null;
  return (
    <p className="px-1 text-xs text-muted-foreground" role="status" data-testid="azure-board-children-notice">
      {t(failed ? "childrenUnavailable" : "childrenTruncated")}
    </p>
  );
}

function BoardEmptyState({
  projects,
  onProjectChange,
}: {
  projects: AzureDevOpsProject[];
  onProjectChange: (value: string) => void;
}) {
  const t = useT();
  return (
    <section className="flex flex-col gap-3 p-4" data-testid="azure-devops-board-empty">
      <SearchablePicker
        value=""
        options={projects}
        onChange={onProjectChange}
        placeholder={t("project")}
        testId="azure-board-project-select"
        className="w-52"
      />
      <p className="text-sm text-muted-foreground">{t("selectAProjectToViewIts")}</p>
    </section>
  );
}

// eslint-disable-next-line max-lines-per-function -- board rendering coordinates drag/drop, responsive columns, and detail state.
export function AzureDevOpsBoard({
  workspaceId,
  projectId,
  projects,
  onProjectChange,
  initialPreference,
  onPreferenceChange,
  quickActions = [],
  onStartTask,
  rememberedTeamId,
  onTeamChosen,
  expandChildrenByBoard,
  onExpandChildrenChange,
}: Props) {
  const t = useT();
  const { isMobile } = useResponsiveBreakpoint();
  const board = useAzureDevOpsBoard(workspaceId, projectId, initialPreference, rememberedTeamId);
  const teamOptions = useMemo(
    () => teamPickerOptions(board.teams, board.mineIds, t("myTeams")),
    [board.mineIds, board.teams, t],
  );
  const [editing, setEditing] = useState<AzureDevOpsBoardWorkItem | null>(null);
  const [dragged, setDragged] = useState<AzureDevOpsBoardWorkItem | null>(null);
  const presentation = useBoardPresentation(projectId, board, initialPreference, onPreferenceChange);
  const selectedItem = board.snapshot?.items.find((item) => item.id === editing?.id) ?? editing;
  const childPresentation = useChildPresentation(workspaceId, projectId, board, {
    expandChildrenByBoard,
    onExpandChildrenChange,
    quickActions,
    onStartTask,
  });
  if (!projectId) return <BoardEmptyState projects={projects} onProjectChange={onProjectChange} />;
  return (
    <section
      className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden p-4"
      data-testid="azure-devops-board"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
      <BoardSelectors
        projectId={projectId}
        projects={projects}
        onProjectChange={onProjectChange}
        teamId={board.teamId}
        teams={teamOptions}
        onTeamChange={(teamId) => {
          board.setTeamId(teamId);
          onTeamChosen?.(teamId);
        }}
        boardId={board.boardId}
        boards={board.boards}
        onBoardChange={board.setBoardId}
      />
        {childPresentation.hasChildren && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={childPresentation.toggleAll}
            data-testid="azure-board-expand-children"
          >
            {t(childPresentation.allExpanded ? "collapseAll" : "expandAll")}
          </Button>
        )}
      </div>
      {board.error && (
        <Alert variant="destructive">
          <AlertDescription>{board.error}</AlertDescription>
        </Alert>
      )}
      {(board.loading || (board.discovering && !board.snapshot)) && (
        <div
          className="flex items-center gap-2 p-6 text-sm text-muted-foreground"
          role="status"
          data-testid="azure-devops-board-loading"
        >
          <Spinner className="h-4 w-4" />
          {t("loadingBoard")}
        </div>
      )}
      {!board.loading && board.snapshot && (
        <>
          <ChildNotices failed={childPresentation.failed} truncated={childPresentation.truncated} />
          {isMobile && (
            <MobileColumnNavigator
              columns={presentation.columns}
              selectedColumn={presentation.columnPreference.effectiveColumn}
              counts={presentation.groups}
              onChange={presentation.columnPreference.setSelectedColumn}
            />
          )}
          <div
            className={
              isMobile
                ? "min-h-0 flex-1 overflow-y-auto"
                : "grid min-h-0 flex-1 auto-cols-fr grid-flow-col gap-3 overflow-x-auto"
            }
          >
            {presentation.columns
              .filter(
                (column) =>
                  !isMobile || column.id === presentation.columnPreference.effectiveColumn,
              )
              .map((column) => (
                <div
                  key={column.id}
                  data-testid={`azure-board-column-${column.id}`}
                  className="flex min-w-72 flex-col gap-2 rounded-lg bg-muted/40 p-2"
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={() => {
                    if (dragged) void board.moveItem(dragged, column.id);
                    setDragged(null);
                  }}
                >
                  <div className="flex items-center justify-between px-1 text-sm font-semibold">
                    <span>{column.name}</span>
                    <Badge variant="secondary">
                      {presentation.groups.get(column.id)?.length ?? 0}
                    </Badge>
                  </div>
                  <div className="space-y-2">
                    {(presentation.groups.get(column.id) ?? []).map((item) => (
                      <BoardCard
                        key={item.id}
                        item={item}
                        onOpen={() => setEditing(item)}
                        onDragStart={() => setDragged(item)}
                        quickActions={quickActions}
                        onStartTask={onStartTask}
                        childCards={childPresentation.childCardsFor(item)}
                      />
                    ))}
                  </div>
                </div>
              ))}
          </div>
          <AzureDevOpsWorkItemDetail
            open={!!editing}
            workspaceId={workspaceId}
            projectId={projectId}
            initialItem={selectedItem}
            onOpenChange={(open) => !open && setEditing(null)}
            boardContext={
              selectedItem
                ? {
                    board: board.snapshot.board,
                    item: selectedItem,
                    onMove: board.moveItem,
                    onItemUpdated: board.mergeItem,
                    onMoveCompleted: (item) => {
                      if (isMobile) {
                        presentation.columnPreference.setSelectedColumn(item.columnId);
                      }
                    },
                  }
                : undefined
            }
            quickActions={quickActions}
            onStartTask={onStartTask}
          />
          <AzureDevOpsWorkItemDetail
            open={!!childPresentation.detail}
            workspaceId={workspaceId}
            projectId={projectId}
            initialItem={childPresentation.detail}
            onOpenChange={(open) => !open && childPresentation.setDetail(null)}
            quickActions={quickActions}
            onStartTask={onStartTask}
          />
        </>
      )}
    </section>
  );
}
