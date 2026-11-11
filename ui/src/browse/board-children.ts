import type { AzureDevOpsBoardChildren, AzureDevOpsChildCard } from "./types";

/** The most cards one children read covers (the action's bound). */
export const MAX_CHILD_PARENTS = 1000;

const COMPLETED = "completed";

export type ChildSummary = { done: number; total: number; type?: string };

/** Done and total children, and their work-item type when they share one. */
export function childSummary(items: AzureDevOpsChildCard[]): ChildSummary {
  const done = items.filter((item) => item.stateCategory?.toLowerCase() === COMPLETED).length;
  const types = new Set(items.map((item) => item.type.toLowerCase()));
  return { done, total: items.length, type: types.size === 1 ? items[0].type : undefined };
}

/** The card IDs to read children for, at most MAX_CHILD_PARENTS. */
export function childParentIds(items: Array<{ id: number }>): { ids: number[]; truncated: boolean } {
  return {
    ids: items.slice(0, MAX_CHILD_PARENTS).map((item) => item.id),
    truncated: items.length > MAX_CHILD_PARENTS,
  };
}

export function childrenByParent(result: AzureDevOpsBoardChildren | null): Map<number, AzureDevOpsChildCard[]> {
  return new Map((result?.children ?? []).map((group) => [group.parentId, group.items]));
}

/** Card expansion: the card's own toggle, else the board's expand-all choice. */
export function isCardExpanded(id: number, expandAll: boolean, toggled: Record<number, boolean>): boolean {
  return toggled[id] ?? expandAll;
}

export function everyCardExpanded(ids: number[], expandAll: boolean, toggled: Record<number, boolean>): boolean {
  return ids.length > 0 && ids.every((id) => isCardExpanded(id, expandAll, toggled));
}

/**
 * The items shown under expanded nodes whose own children were not asked
 * for yet, in display order.
 */
export function missingChildIds(
  expandedIds: number[],
  byParent: Map<number, AzureDevOpsChildCard[]>,
  requested: Set<number>,
): number[] {
  const missing: number[] = [];
  const seen = new Set<number>();
  for (const id of expandedIds) {
    for (const item of byParent.get(id) ?? []) {
      if (!requested.has(item.id) && !seen.has(item.id)) {
        seen.add(item.id);
        missing.push(item.id);
      }
    }
  }
  return missing;
}

/**
 * A copy of the children index with a level's response merged in; requested
 * items missing from the response have no children.
 */
export function mergeChildren(
  byParent: Map<number, AzureDevOpsChildCard[]>,
  requestedIds: number[],
  result: AzureDevOpsBoardChildren,
): Map<number, AzureDevOpsChildCard[]> {
  const merged = new Map(byParent);
  for (const id of requestedIds) if (!merged.has(id)) merged.set(id, []);
  for (const group of result.children) merged.set(group.parentId, group.items);
  return merged;
}
