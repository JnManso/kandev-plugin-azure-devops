import { IconPlus } from "../icons";
import { Button } from "../host-ui";
import { useT } from "../i18n";
import { TaskActionsMenu } from "./results";
import type { AzureDevOpsActionPreset, AzureDevOpsWorkItem } from "./types";

type HandledEvent = { stopPropagation(): void; preventDefault(): void };

const stop = (event: HandledEvent) => event.stopPropagation();

/**
 * Handlers that keep pointer, keyboard, and drag events inside a control
 * placed on a card, so using it never opens or drags the card.
 */
export const containedEvents = {
  onClick: stop,
  onPointerDown: stop,
  onPointerUp: stop,
  onKeyDown: stop,
  onDragStart: (event: HandledEvent) => {
    event.preventDefault();
    event.stopPropagation();
  },
};

/**
 * The work-item "+ Task" control of a card: the quick-action menu, or a start
 * button without quick actions. Its events stay inside it, so using it never
 * opens or drags the card.
 */
export function WorkItemTaskActions<T extends AzureDevOpsWorkItem>({
  item,
  quickActions,
  onStart,
}: {
  item: T;
  quickActions: AzureDevOpsActionPreset[];
  onStart: (item: T, action?: AzureDevOpsActionPreset) => void;
}) {
  const t = useT();
  return (
    <span
      className="inline-flex"
      {...containedEvents}
      data-testid={`azure-card-task-actions-${item.id}`}
    >
      {quickActions.length > 0 ? (
        <TaskActionsMenu item={item} itemKind="workItem" actions={quickActions} onSelect={onStart} />
      ) : (
        <Button type="button" size="sm" variant="outline" className="cursor-pointer" onClick={() => onStart(item)}>
          <IconPlus className="h-4 w-4" />
          {t("startTask")}
        </Button>
      )}
    </span>
  );
}
