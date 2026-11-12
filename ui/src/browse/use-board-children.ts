import { currentHost, useCallback, useEffect, useMemo, useRef, useState } from "../runtime";
import { createBrowseApi } from "./api";
import { MAX_CHILD_PARENTS, childParentIds, mergeChildren, missingChildIds, replaceChildItem } from "./board-children";
import type { AzureDevOpsChildCard, AzureDevOpsWorkItem } from "./types";

type ChildTree = {
  scope: string;
  byParent: Map<number, AzureDevOpsChildCard[]>;
  failed: boolean;
  truncated: boolean;
};

const emptyTree = (scope: string): ChildTree => ({ scope, byParent: new Map(), failed: false, truncated: false });

/**
 * The children of the board's cards, read after the cards render, and of
 * each item shown under an expanded card or child, read when it is shown.
 * Every level is one request. A new set of cards starts over; moving a card
 * keeps what was read.
 */
export function useBoardChildren(
  workspaceId: string | undefined,
  projectId: string,
  items: Array<{ id: number }> | undefined,
  expandedIds: number[],
) {
  const parents = useMemo(() => childParentIds(items ?? []), [items]);
  const scope = `${workspaceId ?? ""}|${projectId}|${parents.ids.join(",")}`;
  const [tree, setTree] = useState<ChildTree>(() => emptyTree(scope));
  const requested = useRef({ scope, ids: new Set<number>() });
  const current = tree.scope === scope ? tree : emptyTree(scope);
  if (requested.current.scope !== scope) requested.current = { scope, ids: new Set() };

  const missing =
    current.byParent.size === 0 && !current.failed
      ? parents.ids.filter((id) => !requested.current.ids.has(id))
      : missingChildIds(expandedIds, current.byParent, requested.current.ids);
  const missingKey = missing.slice(0, MAX_CHILD_PARENTS).join(",");

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
    failed: current.failed,
    truncated: parents.truncated || current.truncated,
    mergeChild,
  };
}
