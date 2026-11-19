import { describe, expect, it } from "vitest";
import { filtersFromSavedView, workItemViewScope } from "./saved-view-filters";
import type { AzureDevOpsFiltersState } from "./filters";

const current: AzureDevOpsFiltersState = {
  projectId: "p0", repositoryId: "", wiql: "SELECT 1", text: "typed", top: 25, status: "active",
  creator: "", reviewer: "", teamId: "old-team", iterationId: "old-sprint",
};

describe("saved work-item views", () => {
  it("record the team and sprint filters", () => {
    expect(workItemViewScope({ ...current, teamId: "team-2", iterationId: "sprint-23" })).toEqual({
      teamId: "team-2",
      iterationId: "sprint-23",
    });
    expect(workItemViewScope({ ...current, teamId: "", iterationId: "" })).toEqual({});
  });

  it("restore the team and sprint filters, and clear them when the view has none", () => {
    const view = { id: "v", kind: "work_item" as const, label: "Mine", projectId: "p1", wiql: "SELECT 2", createdAt: "" };
    expect(filtersFromSavedView(current, { ...view, teamId: "team-2", iterationId: "sprint-23" })).toMatchObject({
      projectId: "p1", wiql: "SELECT 2", text: "", teamId: "team-2", iterationId: "sprint-23",
    });
    expect(filtersFromSavedView(current, view)).toMatchObject({ teamId: "", iterationId: "" });
  });
});
