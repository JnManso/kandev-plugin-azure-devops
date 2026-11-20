import { currentHost, useCallback, useEffect, useMemo, useRef, useState } from "../runtime";
import { createBrowseApi } from "./api";
import { applyChildRead, cardChildrenComplete, nextChildReadIds, replaceChildItem, type ChildReadState } from "./board-children";
import type { AzureDevOpsChildCard, AzureDevOpsWorkItem } from "./types";

type ChildTree = ChildReadState & { scope: string };

const emptyTree = (scope: string): ChildTree => ({
  scope,
  byParent: new Map<number, AzureDevOpsChildCard[]>(),
  failed: false,
  truncated: false,
  cardFailed: false,
  cardTruncated: false,
});

/**
 * The children of the board's cards, read after the cards render in batches
 * the action accepts, and of each item shown under an expanded card or child,
 * read when it is shown. A new set of cards starts over; moving a card keeps
 * what was read.
 */
export function useBoardChildren(
  workspaceId: string | undefined,
  projectId: string,
  items: Array<{ id: number }> | undefined,
  expandedIds: number[],
) {
  const cardIds = useMemo(() => (items ?? []).map((item) => item.id), [items]);
  const cardSet = useMemo(() => new Set(cardIds), [cardIds]);
  const scope = `${workspaceId ?? ""}|${projectId}|${cardIds.join(",")}`;
  const [tree, setTree] = useState<ChildTree>(() => emptyTree(scope));
  const requested = useRef({ scope, ids: new Set<number>() });
  const current = tree.scope === scope ? tree : emptyTree(scope);
  if (requested.current.scope !== scope) requested.current = { scope, ids: new Set() };

  const missingKey = nextChildReadIds(cardIds, expandedIds, current.byParent, requested.current.ids).join(",");

  useEffect(() => {
    if (!workspaceId || !projectId || !missingKey) return;
    const owner = requested.current;
    const ids = missingKey
      .split(",")
      .map(Number)
      .filter((id) => !owner.ids.has(id));
    if (ids.length === 0) return;
    for (const id of ids) owner.ids.add(id);
    const apply = (result: Parameters<typeof applyChildRead>[2]) =>
      setTree((previous) => {
        if (requested.current !== owner) return previous;
        const base = previous.scope === scope ? previous : emptyTree(scope);
        return { ...applyChildRead(base, ids, result, cardSet), scope };
      });
    createBrowseApi(currentHost(), workspaceId)
      .boardChildren(projectId, ids)
      .then(apply)
      .catch(() => apply(null));
  }, [cardSet, missingKey, projectId, scope, workspaceId]);

  const mergeChild = useCallback(
    (item: AzureDevOpsWorkItem) =>
      setTree((previous) =>
        previous.scope === scope ? { ...previous, byParent: replaceChildItem(previous.byParent, item) } : previous,
      ),
    [scope],
  );

  return {
    byParent: current.byParent,
    /** Whether every card's own children were read in full (no failed or truncated card read). */
    complete: cardChildrenComplete(cardIds, current.byParent, current.cardFailed, current.cardTruncated),
    failed: current.failed,
    truncated: current.truncated,
    mergeChild,
  };
}
