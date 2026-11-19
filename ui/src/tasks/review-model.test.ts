import { describe, expect, it } from "vitest";
import type { AzureDevOpsPullRequestFeedback } from "../browse/types";
import { toAssociations, toChangeRequestDetail, toReviewSummary } from "./review-model";

describe("review model", () => {
  it("drops incomplete links and invalid statuses", () => {
    expect(toReviewSummary({ projectId: "", repositoryId: "r", pullRequestId: 1 }, "x")).toBeNull();
    const summary = toReviewSummary(
      { projectId: "p", repositoryId: "R", pullRequestId: 3, status: "completed", taskStatus: { number: 3, state: "bogus", pipelineState: "success", checks: [] } as never },
      "Pull request !3",
    );
    expect(summary).toMatchObject({ reviewKey: "r!3", title: "Pull request !3", state: "merged" });
    expect(summary?.taskStatus).toBeUndefined();
  });

  it("flattens workspace associations", () => {
    expect(
      toAssociations({ pullRequests: { t1: [{ projectId: "p", repositoryId: "r", pullRequestId: 2 }] }, workItems: {}, tasks: {} }),
    ).toEqual([{ providerId: "azure_devops", taskId: "t1", reviewKey: "r!2", connectionScope: "p", repositoryId: "r", changeRequestNumber: 2 }]);
  });

  it("maps feedback to the native review detail", () => {
    const feedback = {
      pullRequest: {
        id: 9, title: "T", status: "active", isDraft: true, sourceBranch: "a", targetBranch: "main",
        author: { id: "u", displayName: "Ada" }, projectId: "p", projectName: "P", repositoryId: "r", repositoryName: "api",
        webUrl: "https://x/9", apiUrl: "",
      },
      reviewers: [
        { id: "1", displayName: "Grace", vote: 10, isRequired: true, hasDeclined: false },
        { id: "2", displayName: "Linus", vote: 0, isRequired: false, hasDeclined: false },
        { id: "3", displayName: "Ken", vote: -10, isRequired: false, hasDeclined: false },
      ],
      threads: [
        { id: 1, status: "active", comments: [{ id: 1, content: "Fix", author: { id: "1", displayName: "Grace" }, commentType: "text" }] },
        { id: 2, status: "fixed", comments: [{ id: 1, content: "Done", author: { id: "1", displayName: "Grace" }, commentType: "text" }] },
        { id: 3, status: "active", comments: [{ id: 1, content: "Policy", author: { id: "s", displayName: "" }, commentType: "system" }] },
      ],
      linkedWorkItems: [],
      policies: [{ id: "b", name: "Build", status: "running", isBlocking: true }],
      reviewState: "",
      policyState: "pending",
    } as AzureDevOpsPullRequestFeedback;
    const detail = toChangeRequestDetail(feedback, "Azure DevOps");
    expect(detail).toMatchObject({ number: 9, state: "open", draft: true, author: { name: "Ada" } });
    expect(detail.reviews.map((review) => review.state)).toEqual(["APPROVED", "CHANGES_REQUESTED"]);
    expect(detail.requestedReviewers).toEqual([{ name: "Linus" }]);
    expect(detail.checks).toEqual([{ id: "b", name: "Build", state: "pending" }]);
    expect(detail.comments.map((comment) => [comment.body, comment.resolved])).toEqual([["Fix", false], ["Done", true]]);
  });
});

import { linkErrorKey } from "./register";

describe("link errors", () => {
  it("maps action statuses to localized messages", () => {
    expect(linkErrorKey({ status: 404 })).toBe("linkReferenceNotFound");
    expect(linkErrorKey({ status: 400 })).toBe("linkReferenceInvalid");
    expect(linkErrorKey({ status: 409 })).toBe("linkConnectionRequired");
    expect(linkErrorKey(new Error("x"))).toBeNull();
  });
});
