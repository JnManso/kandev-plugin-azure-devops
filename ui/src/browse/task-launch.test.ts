import { describe, expect, it } from "vitest";
import { launchRepository, launchText, organizationFromUrl } from "./task-launch";
import type { AzureDevOpsPullRequest, AzureDevOpsWorkItem } from "./types";

const pullRequest = {
  id: 42, title: "Ship it", status: "active", isDraft: false, sourceBranch: "refs/heads/feature/x", targetBranch: "main",
  author: { id: "u", displayName: "Ada" }, projectId: "p1", projectName: "Platform", repositoryId: "guid-api",
  repositoryName: "api", webUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/42", apiUrl: "",
} as AzureDevOpsPullRequest;

describe("task launch", () => {
  it("builds work item and pull request task text", () => {
    const item = { id: 101, title: "Fix login", type: "Bug", webUrl: "https://w/101", description: "<p>Steps</p>" } as AzureDevOpsWorkItem;
    expect(launchText({ kind: "work-item", item }, "(none)")).toEqual({
      title: "Bug 101: Fix login",
      description: "Azure DevOps work item: 101\nURL: https://w/101\n\nFix login\n\nSteps",
    });
    const action = { id: "a", label: "Review", hint: "", icon: "", promptTemplate: "Review {{url}} ({{title}})" };
    expect(launchText({ kind: "pull-request", pullRequest, action }, "(none)")).toEqual({
      title: "Review: Ship it",
      description: "Review https://dev.azure.com/acme/Platform/_git/api/pullrequest/42 (Ship it)",
    });
  });

  it("prefers the workspace repository and falls back to the repository URL", () => {
    const payload = { kind: "pull-request" as const, pullRequest };
    expect(launchRepository(payload, (scope, id) => (scope === "acme/Platform" && id === "guid-api" ? "repo-1" : undefined), "acme"))
      .toEqual({ repositoryId: "repo-1", branch: "feature/x", checkoutBranch: "feature/x" });
    expect(launchRepository(payload, () => undefined, "acme")).toEqual({
      remoteUrl: "https://dev.azure.com/acme/Platform/_git/api", branch: "feature/x", checkoutBranch: "feature/x",
    });
    expect(organizationFromUrl("https://dev.azure.com/acme")).toBe("acme");
  });
});
