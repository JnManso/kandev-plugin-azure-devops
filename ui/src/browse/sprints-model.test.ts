import { describe, expect, it } from "vitest";
import {
  ANY_OPTION,
  UNASSIGNED_PERSON,
  defaultIterationId,
  sprintDateRange,
  filterRowsByPerson,
  iterationTimeFrameKey,
  teamPickerOptions,
  tasksInColumn,
  teamScopeConditions,
  wiqlWithConditions,
} from "./sprints-model";
import type { AzureDevOpsSprintRow, AzureDevOpsTeamSettings } from "./types";

const team = (id: string, name: string) => ({ id, name, projectId: "p" });

describe("teamPickerOptions", () => {
  it("lists my teams first, marked, then the other teams in order", () => {
    const options = teamPickerOptions(
      [team("a", "Alpha Team"), team("b", "Offers Team"), team("c", "Platform Team"), team("d", "Billing")],
      ["c", "b"],
      "My teams",
    );
    expect(options).toEqual([
      { id: "b", name: "Offers Team", description: "My teams" },
      { id: "c", name: "Platform Team", description: "My teams" },
      { id: "a", name: "Alpha Team" },
      { id: "d", name: "Billing" },
    ]);
  });
});

const settings: AzureDevOpsTeamSettings = {
  areaField: "System.AreaPath",
  areas: [
    { path: "Contoso\\Offers", includeChildren: true },
    { path: "Contoso\\O'Neil", includeChildren: false },
  ],
  iterations: [{ id: "it-23", name: "Sprint 23", path: "Contoso\\Sprint 23" }],
};

describe("teamScopeConditions", () => {
  it("ORs the team's areas and adds the sprint path", () => {
    expect(teamScopeConditions(settings, "it-23")).toEqual([
      "([System.AreaPath] UNDER 'Contoso\\Offers' OR [System.AreaPath] = 'Contoso\\O''Neil')",
      "[System.IterationPath] = 'Contoso\\Sprint 23'",
    ]);
  });

  it("adds nothing without settings, and no sprint condition for an unknown sprint", () => {
    expect(teamScopeConditions(undefined, "it-23")).toEqual([]);
    expect(teamScopeConditions({ ...settings, areas: [] }, "gone")).toEqual([]);
  });
});

describe("wiqlWithConditions", () => {
  const conditions = ["[A] = 1", "[B] = 2"];

  it("ANDs the conditions with the existing WHERE clause, before ORDER BY", () => {
    expect(
      wiqlWithConditions(
        "SELECT [System.Id] FROM WorkItems WHERE [X] = 1 OR [Y] = 2 ORDER BY [System.ChangedDate] DESC",
        conditions,
      ),
    ).toBe(
      "SELECT [System.Id] FROM WorkItems WHERE ([X] = 1 OR [Y] = 2) AND [A] = 1 AND [B] = 2 ORDER BY [System.ChangedDate] DESC",
    );
  });

  it("adds a WHERE clause when there is none, in any letter case", () => {
    expect(wiqlWithConditions("select [System.Id] from WorkItems order by [System.Id]", conditions)).toBe(
      "select [System.Id] from WorkItems WHERE [A] = 1 AND [B] = 2 order by [System.Id]",
    );
    expect(wiqlWithConditions("SELECT [System.Id] FROM WorkItems", conditions)).toBe(
      "SELECT [System.Id] FROM WorkItems WHERE [A] = 1 AND [B] = 2",
    );
  });

  it("keeps the query unchanged without conditions", () => {
    expect(wiqlWithConditions("SELECT [System.Id] FROM WorkItems", [])).toBe("SELECT [System.Id] FROM WorkItems");
  });
});

describe("defaultIterationId", () => {
  const iterations = [
    { id: "s21", name: "S21", path: "", timeFrame: "past", finish: "2026-10-23T00:00:00Z" },
    { id: "s22", name: "S22", path: "", timeFrame: "past", finish: "2026-11-06T00:00:00Z" },
    { id: "s23", name: "S23", path: "", timeFrame: "current", finish: "2026-11-20T00:00:00Z" },
    { id: "s24", name: "S24", path: "", timeFrame: "future", finish: "2026-12-04T00:00:00Z" },
  ];

  it("keeps a remembered sprint that still exists, else opens the current one", () => {
    expect(defaultIterationId(iterations, "s21")).toBe("s21");
    expect(defaultIterationId(iterations, "gone")).toBe("s23");
    expect(defaultIterationId(iterations, undefined)).toBe("s23");
  });

  it("opens the latest past sprint without a current one, else the first", () => {
    expect(defaultIterationId(iterations.filter((it) => it.id !== "s23"), undefined)).toBe("s22");
    expect(defaultIterationId([iterations[3]], undefined)).toBe("s24");
    expect(defaultIterationId([], undefined)).toBe("");
  });
});

describe("iterationTimeFrameKey", () => {
  it("names the past, current, and future tags", () => {
    expect(iterationTimeFrameKey("past")).toBe("sprintPast");
    expect(iterationTimeFrameKey("Current")).toBe("sprintCurrent");
    expect(iterationTimeFrameKey("future")).toBe("sprintFuture");
    expect(iterationTimeFrameKey(undefined)).toBeUndefined();
  });
});

describe("filterRowsByPerson", () => {
  const card = (id: number, assignedTo?: string) => ({
    id, revision: 1, title: `#${id}`, state: "New", type: "Task", columnId: "c", ...(assignedTo ? { assignedTo } : {}),
  });
  const rows: AzureDevOpsSprintRow[] = [
    { tasks: [card(1, "Ana"), card(2)] },
    { parent: { id: 10, revision: 1, title: "Story", state: "New", type: "User Story", assignedTo: "Rui" }, tasks: [card(3, "Ana")] },
    { parent: { id: 11, revision: 1, title: "Story", state: "New", type: "User Story" }, tasks: [card(4, "Rui")] },
  ];

  it("keeps every row for All", () => {
    expect(filterRowsByPerson(rows, ANY_OPTION)).toBe(rows);
  });

  it("keeps the person's tasks and the rows that contain them or whose backlog item is theirs", () => {
    const ana = filterRowsByPerson(rows, "Ana");
    expect(ana.map((row) => [row.parent?.id ?? 0, ...row.tasks.map((task) => task.id)])).toEqual([[0, 1], [10, 3]]);
    const rui = filterRowsByPerson(rows, "Rui");
    expect(rui.map((row) => [row.parent?.id ?? 0, ...row.tasks.map((task) => task.id)])).toEqual([[10], [11, 4]]);
  });

  it("filters unassigned tasks", () => {
    const unassigned = filterRowsByPerson(rows, UNASSIGNED_PERSON);
    expect(unassigned.map((row) => [row.parent?.id ?? 0, ...row.tasks.map((task) => task.id)])).toEqual([[0, 2], [11]]);
  });
});

describe("sprintDateRange", () => {
  it("formats the sprint's start and finish days in UTC", () => {
    expect(sprintDateRange("2026-11-09T00:00:00Z", "2026-11-20T00:00:00Z", "en-GB")).toBe("9 Nov – 20 Nov");
  });

  it("is empty without dates", () => {
    expect(sprintDateRange(undefined, "2026-11-20T00:00:00Z", "en-GB")).toBe("");
    expect(sprintDateRange("bad", "2026-11-20T00:00:00Z", "en-GB")).toBe("");
  });
});

describe("tasksInColumn", () => {
  const columns = [
    { id: "todo", name: "To Do" },
    { id: "done", name: "Done" },
  ];
  const task = (id: number, columnId: string) => ({ id, revision: 1, title: "", state: "", type: "Task", columnId });
  const row = { tasks: [task(1, "todo"), task(2, ""), task(3, "done"), task(4, "gone")] };

  it("places tasks without a known column in the first column", () => {
    expect(tasksInColumn(row, columns, "todo").map((item) => item.id)).toEqual([1, 2, 4]);
    expect(tasksInColumn(row, columns, "done").map((item) => item.id)).toEqual([3]);
  });
});
