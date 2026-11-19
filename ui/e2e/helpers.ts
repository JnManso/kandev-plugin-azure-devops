import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const PLUGIN_ID = "kandev-plugin-azure-devops";
export const ORGANIZATION_URL = "https://dev.azure.com/acme";
export const MOCK_CLIENT_ID = "11111111-2222-3333-4444-555555555555";

/** Invokes a plugin action through the host, as the bundle does. */
export async function action<T = Record<string, unknown>>(
  request: APIRequestContext,
  key: string,
  envelope: Record<string, unknown>,
): Promise<T> {
  const response = await request.post(`/api/plugins/${PLUGIN_ID}/actions/${key}`, { data: envelope });
  expect(response.ok(), `${key}: ${await response.text()}`).toBeTruthy();
  return (await response.json()) as T;
}

export async function workspaceId(request: APIRequestContext): Promise<string> {
  const response = await request.get("/api/v1/workspaces");
  const body = (await response.json()) as { workspaces: Array<{ id: string }> };
  return body.workspaces[0].id;
}

/** Connects the workspace to the mock organization with a PAT. */
export async function connectWithPAT(request: APIRequestContext, workspace: string) {
  await action(request, "connection.save", {
    workspaceId: workspace,
    body: {
      organizationUrl: ORGANIZATION_URL, authMethod: "pat", pat: "mock-pat",
      defaultProjectId: "project-1", defaultProjectName: "Platform",
    },
  });
}

export type Launch = { workflowId: string; stepId: string; agentProfileId: string; executorProfileId: string };

/** The first workflow step and the mock agent on the local executor. */
export async function launchOptions(request: APIRequestContext, workspace: string): Promise<Launch> {
  const options = await action<{
    workflows: Array<{ id: string; steps: Array<{ id: string }> }>;
    agentProfiles: Array<{ id: string; name: string }>;
    executorProfiles: Array<{ id: string; name: string }>;
  }>(request, "watches.options", { workspaceId: workspace, body: {} });
  const agent = options.agentProfiles.find((profile) => profile.name === "Mock") ?? options.agentProfiles[0];
  const executor = options.executorProfiles.find((profile) => profile.name === "Local") ?? options.executorProfiles[0];
  return {
    workflowId: options.workflows[0].id,
    stepId: options.workflows[0].steps[0].id,
    agentProfileId: agent.id,
    executorProfileId: executor.id,
  };
}

export async function tasks(request: APIRequestContext, workspace: string) {
  const response = await request.get(`/api/v1/workspaces/${workspace}/tasks`);
  const body = (await response.json()) as { tasks?: Array<{ id: string; title: string; metadata?: Record<string, unknown> }> };
  return body.tasks ?? [];
}

/** Picks an option of a host Select by its trigger test id. */
export async function choose(page: Page, triggerTestId: string, option: string | RegExp) {
  await page.getByTestId(triggerTestId).click();
  await page.getByRole("option", { name: option }).click();
}

export async function openSettings(page: Page) {
  await page.goto("/settings/integrations/azure-devops");
  await expect(page.getByTestId("azure-devops-settings")).toBeVisible();
}

/** Runs the browse search, from the filter drawer on phones. */
export async function search(page: Page) {
  const filters = page.getByTestId("azure-devops-mobile-filter-button");
  if (await filters.isVisible()) await filters.click();
  await page.getByRole("button", { name: "Search", exact: true }).last().click();
}
