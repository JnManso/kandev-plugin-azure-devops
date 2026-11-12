import { describe, expect, it, vi } from "vitest";
import type { PluginHostApi, PluginRegistry, RepositoryProviderRegistration, TaskContext } from "@kdlbs/plugin-sdk";
import { createAzureDevOpsPlugin, matchesAzureReposURL } from "./plugin";

function setup(actions: Record<string, unknown> = {}) {
  let provider: RepositoryProviderRegistration | undefined;
  const settings: Array<{ id: string; label: string; action?: unknown }> = [];
  const routes: string[] = [];
  const navItems: Array<{ id: string; path: string; section?: string }> = [];
  const registry = {
    registerRoute: vi.fn((path: string) => routes.push(path)),
    registerNavItem: vi.fn((item: { id: string; path: string; section?: string }) => navItems.push(item)),
    registerTranslations: vi.fn(),
    registerIntegrationSettings: vi.fn((entry: { id: string; label: string; action?: unknown }) => settings.push(entry)),
    registerRepositoryProvider: vi.fn((entry: RepositoryProviderRegistration) => {
      provider = entry;
    }),
    registerTaskAction: vi.fn((action: TaskActionEntry) => taskActions.push(action)),
    registerReviewProvider: vi.fn((entry: ReviewProviderEntry) => {
      reviewProvider = entry;
    }),
  } as unknown as PluginRegistry;
  const taskActions: TaskActionEntry[] = [];
  let reviewProvider: ReviewProviderEntry | undefined;
  const invokeAction = vi.fn(async (key: string) => actions[key] ?? {});
  const openTaskLinkDialog = vi.fn((options: LinkDialogOptions) => {
    dialogs.push(options);
    return { close: vi.fn() };
  });
  const dialogs: LinkDialogOptions[] = [];
  const host = {
    jsx: vi.fn(),
    i18n: { t: (key: string) => key },
    api: { invokeAction },
    openTaskLinkDialog,
    setIntegrationEnabled: vi.fn(),
    storage: { get: vi.fn(async () => undefined), set: vi.fn(async () => ({ updatedAt: "" })) },
    context: { getWorkspaceIds: () => [], subscribeWorkspaces: () => () => undefined },
  } as unknown as PluginHostApi;
  createAzureDevOpsPlugin().initialize(registry, host);
  return { provider: provider!, settings, routes, navItems, invokeAction, taskActions, reviewProvider: reviewProvider!, dialogs };
}

type TaskActionEntry = { id: string; label: string; placement: string; run(context: TaskContext): Promise<void> };
type LinkDialogOptions = { inputLabel: string; onSubmit(reference: string, signal: AbortSignal): Promise<void> };
type ReviewProviderEntry = {
  id: string;
  refresh(taskId: string, signal: AbortSignal): Promise<void>;
  getSnapshot(taskId: string): readonly { reviewKey: string; taskStatus?: { pipelineState: string } }[];
  refreshAssociations(workspaceId: string, signal: AbortSignal): Promise<void>;
  getAssociationSnapshot(workspaceId: string): readonly { taskId: string; changeRequestNumber: number | string }[];
};

const taskContext: TaskContext = { workspaceId: "ws-1", taskId: "task-1", repositories: [], pathname: "/t/task-1", presentation: "desktop" };
const signal = () => new AbortController().signal;

describe("Azure DevOps plugin registration", () => {
  it("registers the browse page in the integrations navigation", () => {
    const { routes, navItems } = setup();
    expect(routes).toEqual(["/azure-devops"]);
    expect(navItems).toEqual([expect.objectContaining({ id: "azure-devops", path: "/azure-devops", section: "integrations" })]);
  });

  it("registers the integration settings and the azure_devops provider", () => {
    const { provider, settings } = setup();
    expect(settings.map((entry) => entry.id)).toEqual(["azure-devops"]);
    expect(settings[0].label).toBe("azureDevOps");
    expect(settings[0].action).toEqual(expect.any(Function));
    expect(provider.id).toBe("azure_devops");
    expect(provider.label).toBe("Azure DevOps");
  });

  it("maps an inspected repository and treats no match as null", async () => {
    const descriptor = {
      provider_id: "azure_devops",
      provider_host: "https://dev.azure.com",
      provider_scope: "acme/Platform",
      provider_repository_id: "guid-1",
      owner_or_project: "Platform",
      name: "api",
      clone_url: "https://dev.azure.com/acme/Platform/_git/api",
      default_branch: "main",
    };
    const matched = setup({ "repositories.inspect": { repository: descriptor } });
    await expect(
      matched.provider.inspectURL({ workspaceId: "ws-1", url: "https://dev.azure.com/acme/Platform/_git/api", signal: new AbortController().signal }),
    ).resolves.toMatchObject({ repositoryId: "guid-1", providerScope: "acme/Platform", defaultBranch: "main" });

    const unmatched = setup({ "repositories.inspect": { matched: false } });
    await expect(
      unmatched.provider.inspectURL({ workspaceId: "ws-1", url: "https://github.com/a/b", signal: new AbortController().signal }),
    ).resolves.toBeNull();
  });
});

describe("Azure Repos URL hint", () => {
  it("recognizes Azure Repos hosts only", () => {
    for (const url of [
      "https://dev.azure.com/acme/P/_git/r",
      "git@ssh.dev.azure.com:v3/acme/P/r",
      "https://acme.visualstudio.com/P/_git/r",
    ]) {
      expect(matchesAzureReposURL(url), url).toBe(true);
    }
    expect(matchesAzureReposURL("https://github.com/acme/r")).toBe(false);
    expect(matchesAzureReposURL("not a url")).toBe(false);
  });
});

describe("Azure DevOps task integrations", () => {
  it("adds pull request and work item entries to the Link submenu", async () => {
    const { taskActions, dialogs, invokeAction } = setup();
    expect(taskActions.map((action) => [action.label, action.placement])).toEqual([
      ["linkPullRequestMenu", "link"],
      ["linkWorkItemMenu", "link"],
    ]);
    await taskActions[1].run(taskContext);
    await dialogs[0].onSubmit(" #101 ", signal());
    expect(invokeAction).toHaveBeenCalledWith(
      "work_items.link",
      { workspaceId: "ws-1", taskId: "task-1", body: { reference: "#101" } },
      expect.anything(),
    );
    await expect(dialogs[0].onSubmit("  ", signal())).rejects.toThrow("linkWorkItemEmpty");
  });

  it("publishes linked pull requests with their task status", async () => {
    const link = {
      projectId: "p1", repositoryId: "GUID-API", pullRequestId: 42, title: "Ship", url: "https://x/pr/42",
      taskStatus: { number: 42, state: "open", pipelineState: "pending", checks: [] },
    };
    const { reviewProvider } = setup({
      "task_links.get": { pullRequests: [link], workItems: [] },
      "task_links.workspace": { pullRequests: { "task-1": [link] }, workItems: {}, tasks: {} },
    });
    expect(reviewProvider.id).toBe("azure_devops");
    await reviewProvider.refresh("task-1", signal());
    expect(reviewProvider.getSnapshot("task-1")).toEqual([
      expect.objectContaining({ reviewKey: "guid-api!42", taskStatus: expect.objectContaining({ pipelineState: "pending" }) }),
    ]);
    await reviewProvider.refreshAssociations("ws-1", signal());
    expect(reviewProvider.getAssociationSnapshot("ws-1")).toEqual([
      expect.objectContaining({ taskId: "task-1", changeRequestNumber: 42 }),
    ]);
  });

  it("creates pull requests through the verified task session", async () => {
    const { provider, invokeAction } = setup({ "pull_requests.create": { url: "https://x/pr/7", linked: true } });
    expect(provider.supportsDraft).toBe(true);
    const result = await provider.createChangeRequest!({
      workspaceId: "ws-1", taskId: "task-1", sessionId: "s-1", repositoryId: "repo-1",
      repository: {} as never, title: "T", body: "B", draft: true, signal: signal(),
    });
    expect(result).toEqual({ url: "https://x/pr/7", provider: "azure_devops", linked: true });
    expect(invokeAction).toHaveBeenCalledWith(
      "pull_requests.create",
      { workspaceId: "ws-1", taskId: "task-1", sessionId: "s-1", repositoryId: "repo-1", body: { title: "T", description: "B", draft: true } },
      expect.anything(),
    );
  });
});
