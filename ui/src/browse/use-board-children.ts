import { currentHost, useCallback, useEffect, useMemo, useRef, useState } from "../runtime";
import { createBrowseApi } from "./api";
import { cardChildrenComplete, mergeChildren, nextChildReadIds, replaceChildItem } from "./board-children";
import type { AzureDevOpsChildCard, AzureDevOpsWorkItem } from "./types";

type ChildTree = {
  scope: string;
  byParent: Map<number, AzureDevOpsChildCard[]>;
  failed: boolean;
  truncated: boolean;
};

const emptyTree = (scope: string): ChildTree => ({ scope, byParent: new Map(), failed: false, truncated: false });

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
    createBrowseApi(currentHost(), workspaceId)
      .boardChildren(projectId, ids)
      .then((result) =>
        setTree((previous) => {
          const base = previous.scope === scope ? previous : emptyTree(scope);
          if (requested.current !== owner) return previous;
          return {
            ...base,
            byParent: mergeChildren(base.byParent, ids, result),
            truncated: base.truncated || Boolean(result.truncated),
          };
        }),
      )
      .catch(() =>
        setTree((previous) => {
          if (requested.current !== owner) return previous;
          const base = previous.scope === scope ? previous : emptyTree(scope);
          return { ...base, byParent: mergeChildren(base.byParent, ids, { children: [] }), failed: true };
        }),
      );
  }, [missingKey, projectId, scope, workspaceId]);

  const mergeChild = useCallback(
    (item: AzureDevOpsWorkItem) =>
      setTree((previous) =>
        previous.scope === scope ? { ...previous, byParent: replaceChildItem(previous.byParent, item) } : previous,
      ),
    [scope],
  );

  return {
    byParent: current.byParent,
    /** Whether every card's own children were read in full (no failed or truncated read). */
    complete: cardChildrenComplete(cardIds, current.byParent, current.failed, current.truncated),
    failed: current.failed,
    truncated: current.truncated,
    mergeChild,
  };
}
