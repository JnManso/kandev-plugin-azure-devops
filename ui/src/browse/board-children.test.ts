import { describe, expect, it } from "vitest";
import {
  MAX_CHILD_PARENTS,
  childParentIds,
  childSummary,
  childrenByParent,
  everyCardExpanded,
  isCardExpanded,
  mergeChildren,
  missingChildIds,
} from "./board-children";
import type { AzureDevOpsChildCard } from "./types";

const child = (id: number, type: string, stateCategory?: string): AzureDevOpsChildCard => ({
  id,
  revision: 1,
  title: `Child ${id}`,
  state: "Active",
  type,
  stateCategory,
});

describe("childSummary", () => {
  it("counts completed children and names their shared type", () => {
    expect(childSummary([child(1, "Task", "Completed"), child(2, "task", "InProgress"), child(3, "Task", "completed")])).toEqual({
      done: 2,
      total: 3,
      type: "Task",
    });
  });

  it("leaves the type out when the children have several types", () => {
    expect(childSummary([child(1, "User Story", "Resolved"), child(2, "Bug", "Proposed")])).toEqual({
      done: 0,
      total: 2,
      type: undefined,
    });
  });
});

describe("childParentIds", () => {
  it("bounds the cards whose children are read", () => {
    const items = Array.from({ length: MAX_CHILD_PARENTS + 2 }, (_, index) => ({ id: index + 1 }));
    const bounded = childParentIds(items);
    expect(bounded.ids).toHaveLength(MAX_CHILD_PARENTS);
    expect(bounded.truncated).toBe(true);
    expect(childParentIds([{ id: 7 }, { id: 3 }])).toEqual({ ids: [7, 3], truncated: false });
  });
});

describe("childrenByParent", () => {
  it("indexes the children by card", () => {
    const map = childrenByParent({ children: [{ parentId: 5, items: [child(1, "Task")] }] });
    expect(map.get(5)?.map((item) => item.id)).toEqual([1]);
    expect(map.get(6)).toBeUndefined();
    expect(childrenByParent(null).size).toBe(0);
  });
});

describe("card expansion", () => {
  it("follows the expand-all choice unless the card was toggled", () => {
    expect(isCardExpanded(1, true, {})).toBe(true);
    expect(isCardExpanded(1, true, { 1: false })).toBe(false);
    expect(isCardExpanded(2, false, { 2: true })).toBe(true);
  });

  it("reports whether every card with children is expanded", () => {
    expect(everyCardExpanded([1, 2], false, { 1: true, 2: true })).toBe(true);
    expect(everyCardExpanded([1, 2], true, { 2: false })).toBe(false);
    expect(everyCardExpanded([], true, {})).toBe(false);
  });
});

describe("child levels", () => {
  const byParent = new Map([
    [1, [child(11, "Feature"), child(12, "Feature")]],
    [11, [child(111, "User Story")]],
    [2, [child(21, "Feature")]],
  ]);

  it("asks for the children of the items shown under expanded nodes, once", () => {
    expect(missingChildIds([1, 11, 3], byParent, new Set([11]))).toEqual([12, 111]);
    expect(missingChildIds([], byParent, new Set())).toEqual([]);
  });

  it("records the requested items, with an empty list for those without children", () => {
    const merged = mergeChildren(byParent, [12, 111], { children: [{ parentId: 12, items: [child(121, "User Story")] }] });
    expect(merged.get(12)?.map((item) => item.id)).toEqual([121]);
    expect(merged.get(111)).toEqual([]);
    expect(merged.get(1)?.length).toBe(2);
    expect(byParent.has(12)).toBe(false);
  });
});
