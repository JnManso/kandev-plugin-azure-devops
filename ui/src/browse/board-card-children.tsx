import { IconChevronDown, IconChevronRight } from "../icons";
import { Badge } from "../host-ui";
import { useT } from "../i18n";
import { childSummary } from "./board-children";
import { WorkItemTaskActions, containedEvents } from "./task-actions";
import type { AzureDevOpsActionPreset, AzureDevOpsChildCard, AzureDevOpsWorkItem } from "./types";

type StartTask = (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;

/** What every child line needs: the children read so far and their expansion. */
export type ChildTreeView = {
  childrenOf: (id: number) => AzureDevOpsChildCard[] | undefined;
  isLineExpanded: (id: number) => boolean;
  toggleLine: (id: number) => void;
  onOpen: (item: AzureDevOpsChildCard) => void;
  quickActions: AzureDevOpsActionPreset[];
  onStartTask?: StartTask;
};

/** The done/total count of an item's children, which expands or collapses them. */
export function CardChildrenToggle({
  parentId,
  items,
  expanded,
  onToggle,
}: {
  parentId: number;
  items: AzureDevOpsChildCard[];
  expanded: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const summary = childSummary(items);
  const label = summary.type
    ? t("childCountTyped", { type: summary.type, done: summary.done, total: summary.total })
    : t("childCount", { done: summary.done, total: summary.total });
  return (
    <button
      type="button"
      {...containedEvents}
      onClick={(event: { stopPropagation(): void }) => {
        event.stopPropagation();
        onToggle();
      }}
      aria-expanded={expanded}
      aria-label={t(expanded ? "hideChildren" : "showChildren", { id: parentId })}
      className="inline-flex min-w-0 cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
      data-testid={`azure-board-children-toggle-${parentId}`}
    >
      {expanded ? (
        <IconChevronDown className="h-3.5 w-3.5 shrink-0" />
      ) : (
        <IconChevronRight className="h-3.5 w-3.5 shrink-0" />
      )}
      <span className="truncate">{label}</span>
    </button>
  );
}

function ChildLine({ item, tree }: { item: AzureDevOpsChildCard; tree: ChildTreeView }) {
  const t = useT();
  const children = tree.childrenOf(item.id);
  const expanded = tree.isLineExpanded(item.id) && !!children?.length;
  return (
    <li className="space-y-1.5">
      <div
        role="button"
        tabIndex={0}
        {...containedEvents}
        onClick={(event: { stopPropagation(): void }) => {
          event.stopPropagation();
          tree.onOpen(item);
        }}
        onKeyDown={(event: KeyboardEvent) => {
          event.stopPropagation();
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            tree.onOpen(item);
          }
        }}
        className="flex cursor-pointer items-start gap-2 rounded-md border bg-background/60 px-2 py-1.5 hover:border-primary/50"
        data-testid={`azure-board-child-${item.id}`}
      >
        <div className="min-w-0 flex-1 space-y-1">
          <div className="line-clamp-2 text-xs font-medium text-foreground">
            {item.title || t("untitledWorkItem")}
          </div>
          <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
            <span className="font-mono">#{item.id}</span>
            {item.type && <Badge variant="secondary">{item.type}</Badge>}
            <span>{item.state}</span>
            <span className="truncate">{item.assignedTo || t("unassigned")}</span>
          </div>
          {!!children?.length && (
            <CardChildrenToggle
              parentId={item.id}
              items={children}
              expanded={expanded}
              onToggle={() => tree.toggleLine(item.id)}
            />
          )}
        </div>
        {tree.onStartTask && (
          <WorkItemTaskActions item={item} quickActions={tree.quickActions} onStart={tree.onStartTask} />
        )}
      </div>
      {expanded && children && <CardChildrenList parentId={item.id} items={children} tree={tree} nested />}
    </li>
  );
}

/**
 * An item's children, one line each, opening the child's detail; a child
 * with children of its own expands the same way below its line.
 */
export function CardChildrenList({
  parentId,
  items,
  tree,
  nested = false,
}: {
  parentId: number;
  items: AzureDevOpsChildCard[];
  tree: ChildTreeView;
  nested?: boolean;
}) {
  return (
    <ul
      {...containedEvents}
      className={nested ? "ml-1 space-y-1.5 border-l pl-2" : "space-y-1.5"}
      data-testid={`azure-board-children-${parentId}`}
    >
      {items.map((item) => (
        <ChildLine key={item.id} item={item} tree={tree} />
      ))}
    </ul>
  );
}
