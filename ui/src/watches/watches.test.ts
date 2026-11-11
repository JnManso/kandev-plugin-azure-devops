import { describe, expect, it, vi } from "vitest";
import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import { createWatchesApi, emptyWatchInput, inputFromWatch, missingWatchFields, withProject, type Watch } from "./api";
import { disabledReasonText } from "./watch-settings";

const watch: Watch = {
  ...emptyWatchInput("pull_request", "Platform"),
  id: "w1",
  workspaceId: "ws-1",
  workflowId: "wf",
  workflowStepId: "todo",
  agentProfileId: "agent",
  executorProfileId: "gone",
  enabled: false,
  generation: 3,
  disabledReason: "executor_profile_unavailable",
  lastError: "task creation failed",
};

const t = (key: string, options?: Record<string, unknown>) => `${key}:${JSON.stringify(options ?? {})}`;

describe("watch settings", () => {
  it("requires the launch fields and WIQL for work-item watches", () => {
    expect(missingWatchFields(emptyWatchInput("work_item"))).toEqual([
      "projectId", "workflowId", "workflowStepId", "agentProfileId", "executorProfileId", "wiql",
    ]);
    expect(missingWatchFields(inputFromWatch(watch))).toEqual([]);
  });

  it("keeps only editable fields when editing", () => {
    const input = inputFromWatch(watch) as Record<string, unknown>;
    expect(input.id).toBeUndefined();
    expect(input.generation).toBeUndefined();
    expect(input.disabledReason).toBeUndefined();
    expect(input.prompt).toBe("Review {{pull_request.title}}");
  });

  it("names the missing executor in the disabled reason", () => {
    const options = { workflows: [], agentProfiles: [], executorProfiles: [{ id: "gone", name: "Ana's laptop" }], repositories: [] };
    expect(disabledReasonText(t, watch, options)).toBe('watchDisabledExecutorProfile:{"name":"Ana\'s laptop"}');
    expect(disabledReasonText(t, { ...watch, disabledReason: "agent_profile_unavailable" }, options)).toBe(
      'watchDisabledAgentProfile:{"name":"agent"}',
    );
    expect(disabledReasonText(t, { ...watch, disabledReason: undefined }, options)).toBeNull();
  });

  it("sends watch actions with the workspace selector", async () => {
    const invokeAction = vi.fn(async () => ({ count: 4 }));
    const api = createWatchesApi({ api: { invokeAction } } as unknown as PluginHostApi, "ws-1");
    await expect(api.previewReset("w1")).resolves.toBe(4);
    expect(invokeAction).toHaveBeenCalledWith("watches.preview_reset", { workspaceId: "ws-1", body: { id: "w1" } }, { signal: undefined });
  });
});

describe("withProject", () => {
  it("drops the repository of the previous project", () => {
    const input = { ...emptyWatchInput("pull_request", "Platform"), azureRepositoryId: "repo-1" };
    expect(withProject(input, "Billing")).toMatchObject({ projectId: "Billing", azureRepositoryId: "" });
    expect(withProject(input, "Platform")).toBe(input);
  });
});
