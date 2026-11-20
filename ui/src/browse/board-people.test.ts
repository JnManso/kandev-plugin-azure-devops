import { describe, expect, it } from "vitest";
import { boardPeople, boardPersonFilter, filterCardsByPerson } from "./board-people";
import { ANY_OPTION, UNASSIGNED_PERSON } from "./sprints-model";
import type { AzureDevOpsChildCard } from "./types";

const card = (id: number, assignedTo?: string) => ({ id, assignedTo });
const child = (id: number, assignedTo?: string): AzureDevOpsChildCard => ({
  id,
  revision: 1,
  title: `Child ${id}`,
  state: "Active",
  type: "Task",
  assignedTo,
});

describe("boardPeople", () => {
  it("lists every assignee of a card or a direct child once, by name", () => {
    const byParent = new Map([
      [1, [child(11, "Rui"), child(12, " Ana "), child(13)]],
      [2, [child(21, "bruno")]],
      [11, [child(111, "Grandchild Owner")]],
    ]);
    expect(boardPeople([card(1, "Ana"), card(2), card(3, "Carla")], byParent)).toEqual(["Ana", "bruno", "Carla", "Rui"]);
  });

  it("orders names that differ only in case like the sprint people", () => {
    expect(boardPeople([card(1, "ana"), card(2, "Ana")], new Map())).toEqual(["Ana", "ana"]);
  });
});

describe("filterCardsByPerson", () => {
  const cards = [card(1, "Ana"), card(2), card(3, "Rui"), card(4, "Ana")];
  const byParent = new Map([
    [2, [child(21, "Rui")]],
    [3, [child(31, "Rui"), child(32)]],
    [4, [child(41, "Ana")]],
    [21, [child(211, "Bruno")]],
  ]);

  it("keeps every card for anyone", () => {
    expect(filterCardsByPerson(cards, byParent, ANY_OPTION)).toBe(cards);
    expect(filterCardsByPerson(cards, byParent, "")).toBe(cards);
  });

  it("keeps the cards assigned to the person or with a direct child assigned to them", () => {
    expect(filterCardsByPerson(cards, byParent, "Ana").map((item) => item.id)).toEqual([1, 4]);
    expect(filterCardsByPerson(cards, byParent, "Rui").map((item) => item.id)).toEqual([2, 3]);
  });

  it("ignores grandchildren", () => {
    expect(filterCardsByPerson(cards, byParent, "Bruno")).toEqual([]);
  });

  it("keeps unassigned cards and cards with an unassigned direct child", () => {
    expect(filterCardsByPerson(cards, byParent, UNASSIGNED_PERSON).map((item) => item.id)).toEqual([2, 3]);
  });
});

describe("boardPersonFilter", () => {
  it("starts with all people without a remembered person", () => {
    expect(boardPersonFilter(undefined, ["Ana"], true)).toEqual({ person: ANY_OPTION, people: ["Ana"] });
  });

  it("keeps a remembered person the board names", () => {
    expect(boardPersonFilter("Rui", ["Ana", "Rui"], true)).toEqual({ person: "Rui", people: ["Ana", "Rui"] });
    expect(boardPersonFilter(UNASSIGNED_PERSON, [], true).person).toBe(UNASSIGNED_PERSON);
  });

  it("keeps a remembered person, as an option, until the children are complete", () => {
    expect(boardPersonFilter("Rui", ["Ana"], false)).toEqual({ person: "Rui", people: ["Ana", "Rui"] });
  });

  it("shows all people once the complete children do not name the remembered person", () => {
    expect(boardPersonFilter("Rui", ["Ana"], true)).toEqual({ person: ANY_OPTION, people: ["Ana"] });
  });
});
