import { expect, test } from "@playwright/test";
import { PLUGIN_ID, action, choose, connectWithPAT, launchOptions, openSettings, search, workspaceId } from "./helpers";

test.describe.configure({ mode: "serial" });

test.beforeAll(async ({ request }) => {
  await connectWithPAT(request, await workspaceId(request));
});

test("shows the connection and watches on a phone without horizontal scrolling", async ({ page }) => {
  await openSettings(page);
  await expect(page.getByTestId("azure-devops-watch-settings")).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});

test("offers the enable switch on the integrations card", async ({ page, request }) => {
  await page.goto(`/settings/workspaces/${await workspaceId(request)}/integrations`);
  const toggle = page.getByTestId(`integration-card-${PLUGIN_ID}-azure-devops`).getByRole("switch", { name: "Enable Azure DevOps" });
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});

test("opens the watch editor as a full-height drawer", async ({ page }) => {
  await openSettings(page);
  await page.getByTestId("azure-add-pull-request-watch").click();
  await expect(page.getByTestId("azure-watch-editor")).toBeVisible();
  await page.getByTestId("azure-watch-editor-close").click();
  await expect(page.getByTestId("azure-watch-editor")).toHaveCount(0);
});

test("shows a disabled watch reason on a phone", async ({ page, request }) => {
  const workspace = await workspaceId(request);
  const launch = await launchOptions(request, workspace);
  const { watch } = await action<{ watch: { id: string } }>(request, "watches.create", {
    workspaceId: workspace,
    body: {
      kind: "work_item", projectId: "Platform", wiql: "SELECT [System.Id] FROM WorkItems", workflowId: launch.workflowId,
      workflowStepId: launch.stepId, agentProfileId: "removed-agent", executorProfileId: launch.executorProfileId, prompt: "Do it",
    },
  });
  await action(request, "watches.trigger", { workspaceId: workspace, body: { id: watch.id } });
  await openSettings(page);
  await expect(page.getByTestId(`azure-watch-disabled-reason-${watch.id}`)).toContainText("removed-agent");
});

test("browses work items on a phone", async ({ page }) => {
  await page.goto("/azure-devops");
  await expect(page.getByTestId("azure-devops-page")).toBeVisible();
  await page.getByTestId("azure-devops-work-items-mode").click();
  await search(page);
  await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
});

test("shows a sprint taskboard on a phone without horizontal scrolling", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-sprints-mode").click();
  await choose(page, "azure-sprint-team-select", "Offers Team");
  await choose(page, "azure-sprint-select", /Sprint 23/);
  await expect(page.getByTestId("azure-sprint-group-201-col-doing")).toContainText("Build the checkout form");
  const selectors = await page.getByTestId("azure-sprint-selectors").boundingBox();
  const team = await page.getByTestId("azure-sprint-team-select").boundingBox();
  expect(team && selectors && team.width).toBeGreaterThan((selectors?.width ?? 0) - 2);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});

test("expands a board card's children on a phone without horizontal scrolling", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await choose(page, "azure-board-team-select", "Platform Team");
  const select = page.getByTestId("azure-board-select");
  await expect(select).toBeEnabled();
  if (!(await select.textContent())?.includes("Stories")) await choose(page, "azure-board-select", "Stories");
  await page.getByTestId("azure-board-children-toggle-101").click();
  await expect(page.getByTestId("azure-board-child-301")).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});

test("expands nested child lines on a phone without horizontal scrolling", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await choose(page, "azure-board-team-select", "Platform Team");
  await expect(page.getByTestId("azure-board-select")).toBeEnabled();
  await choose(page, "azure-board-select", "Features");
  await page.getByTestId("azure-board-children-toggle-102").click();
  await page.getByTestId("azure-board-children-102").getByTestId("azure-board-children-toggle-101").click();
  await expect(page.getByTestId("azure-board-children-101").getByTestId("azure-board-child-302")).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});
