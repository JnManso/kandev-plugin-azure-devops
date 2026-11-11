import { describe, expect, it } from "vitest";
import {
  canSearch,
  pickedValue,
  pickerFilter,
  pullRequestSearchQuery,
  repositoryForProject,
  workItemSearchWiql,
} from "./search-query";
import type { AzureDevOpsFiltersState } from "./filters";

const filters: AzureDevOpsFiltersState = {
  projectId: "p1",
  repositoryId: "r1",
  wiql: "SELECT [System.Id] FROM WorkItems",
  text: "",
  top: 50,
  status: "active",
  creator: "",
  reviewer: "@me",
  teamId: "",
  iterationId: "",
};

describe("pull request search query", () => {
  it("asks Azure DevOps for every status when All is selected", () => {
    // Azure DevOps returns only active pull requests when status is omitted.
    expect(pullRequestSearchQuery({ ...filters, status: "all" }, 0, 25)).toEqual({
      project: "p1",
      repository: "r1",
      status: "all",
      creator: undefined,
      reviewer: "@me",
      skip: 0,
      top: 25,
    });
  });
});

describe("repository for project", () => {
  const p1 = [
    { id: "r1", projectId: "p1" },
    { id: "r2", projectId: "p1" },
  ];
  const p2 = [{ id: "r9", projectId: "p2" }];

  it("keeps a repository that belongs to the project", () => {
    expect(repositoryForProject("p1", "r2", p1)).toBeUndefined();
  });

  it("picks the first repository of the project when none is chosen", () => {
    expect(repositoryForProject("p1", "", p1)).toBe("r1");
  });

  it("replaces a repository from another project", () => {
    expect(repositoryForProject("p1", "r9", p1)).toBe("r1");
  });

  it("waits while the list still belongs to the previous project", () => {
    expect(repositoryForProject("p1", "", p2)).toBeUndefined();
    expect(repositoryForProject("p1", "r9", p2)).toBeUndefined();
  });

  it("does nothing without repositories", () => {
    expect(repositoryForProject("p1", "r9", [])).toBeUndefined();
  });
});

describe("work item search", () => {
  const wiql = "SELECT [System.Id] FROM WorkItems WHERE [System.State] = 'Active'";

  it("uses the WIQL query when the search box is empty", () => {
    expect(workItemSearchWiql("  ", wiql)).toBe(wiql);
  });

  it("finds a work item by number", () => {
    expect(workItemSearchWiql(" #1152692 ", wiql)).toBe(
      "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.Id] = 1152692",
    );
  });

  it("finds work items whose title has every word, in any order, and escapes quotes", () => {
    // "DD Backoffice | Context Configuration" must match "Backoffice Context".
    expect(workItemSearchWiql("  Rui's   offload ", wiql)).toBe(
      "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND " +
        "[System.Title] CONTAINS 'Rui''s' AND [System.Title] CONTAINS 'offload' " +
        "ORDER BY [System.ChangedDate] DESC",
    );
  });
});

describe("picker filter", () => {
  it("keeps names that contain the typed text, ignoring case", () => {
    expect(pickerFilter("team-2", "off", ["Offers Team", ""])).toBe(1);
    expect(pickerFilter("team-1", "OFF", ["Platform Team", ""])).toBe(0);
    expect(pickerFilter("team-1", "  ", ["Platform Team", ""])).toBe(1);
  });
});

describe("picked value", () => {
  it("keeps the selection when the selected option is picked again", () => {
    // The host combobox reports "" when the selected option is picked again.
    expect(pickedValue("p1", "")).toBe("p1");
  });

  it("takes a different option", () => {
    expect(pickedValue("p1", "p2")).toBe("p2");
  });
});

describe("can search", () => {
  it("needs a project", () => {
    expect(canSearch("work-items", { ...filters, projectId: "", text: "Backoffice" })).toBe(false);
    expect(canSearch("pull-requests", { ...filters, projectId: "" })).toBe(false);
  });

  it("needs search text or a WIQL query for work items", () => {
    expect(canSearch("work-items", { ...filters, text: "Backoffice", wiql: "" })).toBe(true);
    expect(canSearch("work-items", { ...filters, text: "", wiql: "SELECT 1" })).toBe(true);
    expect(canSearch("work-items", { ...filters, text: " ", wiql: " " })).toBe(false);
  });

  it("needs a repository for pull requests", () => {
    expect(canSearch("pull-requests", filters)).toBe(true);
    expect(canSearch("pull-requests", { ...filters, repositoryId: "" })).toBe(false);
  });

  it("never searches from the board", () => {
    expect(canSearch("board", filters)).toBe(false);
    expect(canSearch("sprints", filters)).toBe(false);
  });
});
