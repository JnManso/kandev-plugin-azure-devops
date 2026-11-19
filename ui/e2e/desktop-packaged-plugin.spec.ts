import { expect, test, type Page } from "@playwright/test";
import {
  MOCK_CLIENT_ID,
  ORGANIZATION_URL,
  PLUGIN_ID,
  action,
  choose,
  connectWithPAT,
  launchOptions,
  openSettings,
  search,
  tasks,
  workspaceId,
} from "./helpers";

test.describe.configure({ mode: "serial" });

const saveBar = (page: Page) => page.getByTestId("settings-floating-save");
const saveChanges = (page: Page) => saveBar(page).getByRole("button", { name: "Save changes" });

test("connects a workspace with a personal access token", async ({ page, request }) => {
  await openSettings(page);
  await page.getByTestId("azure-devops-organization-url").fill(ORGANIZATION_URL);
  await expect(saveBar(page)).toContainText("Enter a personal access token to save this connection.");
  await expect(saveChanges(page)).toBeDisabled();
  await page.getByTestId("azure-devops-pat").fill("mock-pat");
  await saveChanges(page).click();
  await expect(saveBar(page)).not.toBeVisible();
  const workspace = await workspaceId(request);
  await expect
    .poll(async () => (await action<{ connection: { hasSecret?: boolean } }>(request, "connection.get", { workspaceId: workspace, body: {} })).connection.hasSecret)
    .toBe(true);
  await expect(page.getByTestId("azure-devops-watch-settings")).toBeVisible();
});

test("signs in with a Microsoft account through the mock authority", async ({ page, request }) => {
  await openSettings(page);
  await page.getByTestId("azure-devops-auth-method").click();
  await page.getByTestId("azure-devops-auth-oauth").click();
  await page.getByTestId("azure-devops-oauth-client-id").fill(MOCK_CLIENT_ID);
  const popup = page.waitForEvent("popup");
  await page.getByTestId("azure-devops-oauth-connect").click();
  await (await popup).waitForEvent("close");
  await expect(page.getByTestId("azure-devops-oauth-account")).toBeVisible();

  await page.reload();
  const clientId = page.getByTestId("azure-devops-oauth-client-id");
  const connect = page.getByTestId("azure-devops-oauth-connect");
  await expect(clientId).toHaveValue(MOCK_CLIENT_ID);
  await expect(page.getByTestId("azure-devops-oauth-client-required")).toHaveCount(0);
  await expect(connect).toBeEnabled();
  const [fieldBox, buttonBox] = await Promise.all([clientId.boundingBox(), connect.boundingBox()]);
  expect(fieldBox!.y).toBeLessThan(buttonBox!.y);

  const organization = page.getByTestId("azure-devops-organization-url");
  await organization.fill("https://dev.azure.com/other");
  await expect(page.getByTestId("azure-devops-oauth-organization-changed")).toBeVisible();
  await expect(saveChanges(page)).toBeDisabled();
  await organization.fill(ORGANIZATION_URL);
  await expect(page.getByTestId("azure-devops-oauth-organization-changed")).toHaveCount(0);
  await expect(saveBar(page)).not.toBeVisible();

  await choose(page, "azure-devops-default-project", "Platform");
  await saveChanges(page).click();
  await expect(saveBar(page)).not.toBeVisible();
  const workspace = await workspaceId(request);
  const stored = await action<{ connection: { defaultProjectName?: string } }>(request, "connection.get", { workspaceId: workspace, body: {} });
  expect(stored.connection.defaultProjectName).toBe("Platform");

  await connectWithPAT(request, await workspaceId(request));
});

test("loads the quick actions once and keeps them editable", async ({ page }) => {
  let loads = 0;
  page.on("request", (request) => {
    if (request.url().includes("/actions/workspace_settings.get")) loads += 1;
  });
  await openSettings(page);
  await expect(page.getByTestId("azure-devops-quick-actions-card").getByRole("textbox").first()).toBeEnabled();
  // A reload loop re-requests the settings continuously; give it time to show.
  await page.waitForTimeout(1_500);
  expect(loads).toBeLessThanOrEqual(4);
});

test("browses work items, boards, and pull requests", async ({ page, request }) => {
  await connectWithPAT(request, await workspaceId(request));
  await page.goto("/azure-devops");
  await expect(page.getByTestId("azure-devops-page")).toBeVisible();
  await page.getByTestId("azure-devops-work-items-mode").click();
  await search(page);
  await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
  await page.getByTestId("azure-devops-pull-requests-mode").click();
  await search(page);
  await expect(page.getByTestId("azure-devops-pull-request-results")).toContainText("Rotate integration credentials");
});

test("searches work items by title words and by number", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-work-items-mode").click();
  const box = page.getByTestId("azure-devops-work-item-search");
  try {
    await box.fill("rotation");
    await box.press("Enter");
    await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
    await box.fill("no such work item");
    await box.press("Enter");
    await expect(page.getByText("No matching results.")).toBeVisible();
    await expect(page.getByTestId("azure-work-item-row-101")).toHaveCount(0);
    await box.fill("#101");
    await box.press("Enter");
    await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
    // Every word must be in the title, in any order.
    await box.fill("rotation handle");
    await box.press("Enter");
    await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
  } finally {
    // The search box is saved with the browse filters; leave it empty.
    await box.fill("");
    await box.press("Enter");
    await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
  }
});

test("filters the board team picker by typing", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await page.getByTestId("azure-board-team-select").click();
  await page.getByPlaceholder("Search").last().fill("Off");
  await expect(page.getByRole("option", { name: "Offers Team" })).toBeVisible();
  await expect(page.getByRole("option", { name: "Platform Team" })).toHaveCount(0);
  await page.getByRole("option", { name: "Offers Team" }).click();
  await expect(page.getByTestId("azure-board-team-select")).toContainText("Offers Team");
});

test("keeps the board project when it is picked again", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  const project = page.getByTestId("azure-board-project-select");
  await expect(project).toContainText("Platform");
  await project.click();
  await page.getByRole("option", { name: "Platform", exact: true }).click();
  await expect(project).toContainText("Platform");
  await expect(page.getByTestId("azure-board-team-select")).toBeVisible();
});

test("opens the remembered team and board before the team list arrives", async ({ page }) => {
  const columns = page.locator('[data-testid^="azure-board-column-"]:not([data-testid="azure-board-column-picker"])');
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await choose(page, "azure-board-team-select", "Platform Team");
  const saved = page.waitForRequest(
    (request) => request.method() !== "GET" && (request.postData() ?? "").includes('"boardName":"Features"'),
  );
  await choose(page, "azure-board-select", "Features");
  await expect(columns.first()).toBeVisible();
  await saved;

  let teamsListed = false;
  await page.route("**/actions/teams.list", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 4000));
    teamsListed = true;
    await route.continue();
  });
  await page.reload();
  await expect(page.getByTestId("azure-board-team-select")).toContainText("Platform Team", { timeout: 3000 });
  await expect(page.getByTestId("azure-board-select")).toContainText("Features", { timeout: 3000 });
  await expect(columns.first()).toBeVisible({ timeout: 3000 });
  expect(teamsListed).toBe(false);
  await expect(page.getByTestId("azure-board-team-select")).toContainText("Platform Team");
  await page.unroute("**/actions/teams.list");
});

test("drops the previous board's cards when a board fails to load", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await choose(page, "azure-board-team-select", "Platform Team");
  await choose(page, "azure-board-select", "Stories");
  await expect(page.getByTestId("azure-board-card-101")).toBeVisible();
  await page.route("**/actions/boards.get", (route) =>
    (route.request().postData() ?? "").includes("board-2") ? route.abort() : route.continue(),
  );
  await choose(page, "azure-board-select", "Features");
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page.getByTestId("azure-board-card-101")).toHaveCount(0);
  await page.unroute("**/actions/boards.get");
  await choose(page, "azure-board-select", "Stories");
  await expect(page.getByTestId("azure-board-card-101")).toBeVisible();
});

test("shows the board loading state from the start", async ({ page }) => {
  const delay = (ms: number) => async (route: import("@playwright/test").Route) => {
    await new Promise((resolve) => setTimeout(resolve, ms));
    await route.continue();
  };
  await page.route("**/actions/teams.list", delay(3000));
  await page.route("**/actions/boards.get", delay(2000));
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await expect(page.getByTestId("azure-devops-board-loading")).toBeVisible({ timeout: 1000 });
  await page.unroute("**/actions/teams.list");
  await page.unroute("**/actions/boards.get");
});

test("opens the board on the first project when the connection has no default project", async ({ page, request }) => {
  const workspace = await workspaceId(request);
  await action(request, "connection.save", {
    workspaceId: workspace,
    body: { organizationUrl: ORGANIZATION_URL, authMethod: "pat", pat: "mock-pat" },
  });
  try {
    await page.goto("/azure-devops");
    await page.getByTestId("azure-devops-board-mode").click();
    await expect(page.getByTestId("azure-board-project-select")).toContainText("Platform");
    await expect(page.getByTestId("azure-board-team-select")).toBeVisible();
  } finally {
    await connectWithPAT(request, workspace);
  }
});

test("lists my teams first in the board team picker", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await page.getByTestId("azure-board-team-select").click();
  await expect(page.getByRole("option", { name: /Alpha Team/ })).toBeVisible();
  const options = (await page.getByRole("option").allInnerTexts()).map((text) => text.replace(/\s+/g, " "));
  const mine = options.findIndex((text) => text.includes("Offers Team"));
  expect(options[mine]).toContain("My teams");
  expect(mine).toBeLessThan(options.findIndex((text) => text.includes("Alpha Team")));
  await page.keyboard.press("Escape");
});

test("offers task actions on a board card without opening it", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  const card = page.locator('[data-testid^="azure-board-card-"]').first();
  await expect(card).toBeVisible();
  await card.locator('[data-testid^="azure-card-task-actions-"]').getByRole("button").click();
  await expect(page.getByRole("menuitem").first()).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.keyboard.press("Escape");
});

async function openBoard(page: Page, board: string) {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-board-mode").click();
  await choose(page, "azure-board-team-select", "Platform Team");
  const select = page.getByTestId("azure-board-select");
  await expect(select).toBeEnabled();
  if (!(await select.textContent())?.includes(board)) await choose(page, "azure-board-select", board);
  await expect(select).toContainText(board);
}

test("shows a card's child items and opens a child without opening the card", async ({ page }) => {
  await openBoard(page, "Stories");
  const toggle = page.getByTestId("azure-board-children-toggle-101");
  await expect(toggle).toHaveText("Task: 1/2");
  await expect(page.getByTestId("azure-board-children-101")).toHaveCount(0);
  await toggle.click();
  const children = page.getByTestId("azure-board-children-101");
  await expect(children.locator('[data-testid^="azure-board-child-"]')).toHaveCount(2);
  await expect(children.getByTestId("azure-board-child-301")).toContainText("Rotate the signing key");
  await expect(page.getByTestId("azure-board-child-303")).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);

  await page.getByTestId("azure-board-child-302").locator('[data-testid^="azure-card-task-actions-"]').getByRole("button").click();
  await expect(page.getByRole("menuitem").first()).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);

  await page.getByTestId("azure-board-child-302").click();
  await expect(page.getByRole("dialog")).toContainText("Refresh the token cache");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await toggle.click();
  await expect(page.getByTestId("azure-board-children-101")).toHaveCount(0);
});

test("counts children of several types without naming a type", async ({ page }) => {
  await openBoard(page, "Features");
  await expect(page.getByTestId("azure-board-children-toggle-102")).toHaveText("Child items: 0/2");
});

test("expands child lines down to their tasks, reading each level once", async ({ page }) => {
  const reads: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/actions/boards.children")) reads.push(request.postData() ?? "");
  });
  await openBoard(page, "Features");
  await page.getByTestId("azure-board-children-toggle-102").click();
  const features = page.getByTestId("azure-board-children-102");
  await expect(features.getByTestId("azure-board-children-toggle-101")).toHaveText("Task: 1/2");
  await expect(features.getByTestId("azure-board-child-304").getByRole("button", { name: /child items/i })).toHaveCount(0);
  await features.getByTestId("azure-board-children-toggle-101").click();
  const tasks = page.getByTestId("azure-board-children-101");
  await expect(tasks.getByTestId("azure-board-child-301")).toContainText("Rotate the signing key");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await features.getByTestId("azure-board-children-toggle-101").click();
  await expect(page.getByTestId("azure-board-children-101")).toHaveCount(0);
  await features.getByTestId("azure-board-children-toggle-101").click();
  await tasks.getByTestId("azure-board-child-302").click();
  await expect(page.getByRole("dialog")).toContainText("Refresh the token cache");
  await page.keyboard.press("Escape");
  const levels = reads.filter((body) => /"ids":\[(101,304|304,101)\]/.test(body) || body.includes('"ids":[102]'));
  expect(levels.length).toBe(2);
});

test("remembers expand all for the board after a reload", async ({ page }) => {
  await openBoard(page, "Stories");
  const expandAll = page.getByTestId("azure-board-expand-children");
  await expect(expandAll).toHaveText("Expand all");
  const saved = page.waitForRequest(
    (request) => request.method() !== "GET" && (request.postData() ?? "").includes('"expandChildren"'),
  );
  await expandAll.click();
  await expect(page.getByTestId("azure-board-children-101")).toBeVisible();
  await expect(expandAll).toHaveText("Collapse all");
  await saved;
  await page.reload();
  await expect(page.getByTestId("azure-board-children-101")).toBeVisible();
  await expect(page.getByTestId("azure-board-expand-children")).toHaveText("Collapse all");
  await page.getByTestId("azure-board-expand-children").click();
  await expect(page.getByTestId("azure-board-children-101")).toHaveCount(0);
});

test("shows the cards before their children and says when children are unavailable", async ({ page }) => {
  let release: () => void = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/actions/boards.children", async (route) => {
    await held;
    await route.continue();
  });
  await openBoard(page, "Stories");
  await expect(page.getByTestId("azure-board-card-101")).toBeVisible();
  await expect(page.getByTestId("azure-board-children-toggle-101")).toHaveCount(0);
  release();
  await expect(page.getByTestId("azure-board-children-toggle-101")).toBeVisible();
  await page.unroute("**/actions/boards.children");

  await page.route("**/actions/boards.children", (route) => route.abort());
  await page.reload();
  await expect(page.getByTestId("azure-board-card-101")).toBeVisible();
  await expect(page.getByTestId("azure-board-children-notice")).toHaveText("Child items are unavailable.");
  await expect(page.getByTestId("azure-board-children-toggle-101")).toHaveCount(0);
  await page.unroute("**/actions/boards.children");
});

test("shows a team without sprints, then a team's current sprint as a taskboard", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-sprints-mode").click();
  await choose(page, "azure-sprint-team-select", "Platform Team");
  await expect(page.getByTestId("azure-sprint-no-sprints")).toBeVisible();
  await choose(page, "azure-sprint-team-select", "Offers Team");
  await expect(page.getByTestId("azure-sprint-select")).toContainText("Sprint 23");
  await expect(page.getByTestId("azure-sprint-days-left")).toBeVisible();
  const rows = page.locator('[data-testid^="azure-sprint-row-"]:not([data-testid^="azure-sprint-row-toggle-"])');
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toHaveAttribute("data-testid", "azure-sprint-row-unparented");
  await expect(page.getByTestId("azure-sprint-row-201").getByTestId("azure-sprint-card-211")).toBeVisible();
  await expect(page.getByTestId("azure-sprint-column-col-doing")).toHaveText("In Progress");
  await page.getByTestId("azure-sprint-select").click();
  await expect(page.getByRole("option", { name: /Sprint 22/ })).toContainText("Past");
  await page.getByRole("option", { name: /Sprint 22/ }).click();
  await expect(page.getByTestId("azure-sprint-empty")).toBeVisible();
});

test("filters the sprint by person and collapses its rows", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-sprints-mode").click();
  await choose(page, "azure-sprint-team-select", "Offers Team");
  await choose(page, "azure-sprint-select", /Sprint 23/);
  await expect(page.getByTestId("azure-sprint-card-211")).toBeVisible();
  await choose(page, "azure-sprint-person-select", "Bruno Costa");
  await expect(page.getByTestId("azure-sprint-card-213")).toBeVisible();
  await expect(page.getByTestId("azure-sprint-card-221")).toBeVisible();
  await expect(page.getByTestId("azure-sprint-row-201")).toHaveCount(0);
  await choose(page, "azure-sprint-person-select", "Unassigned");
  await expect(page.getByTestId("azure-sprint-card-212")).toBeVisible();
  await expect(page.getByTestId("azure-sprint-card-213")).toHaveCount(0);
  await choose(page, "azure-sprint-person-select", "All people");
  await page.getByTestId("azure-sprint-row-toggle-201").click();
  await expect(page.getByTestId("azure-sprint-card-211")).toHaveCount(0);
  await expect(page.getByTestId("azure-sprint-card-213")).toBeVisible();
  await page.getByTestId("azure-sprint-collapse-all").click();
  await expect(page.getByTestId("azure-sprint-card-213")).toHaveCount(0);
  await page.getByTestId("azure-sprint-collapse-all").click();
  await expect(page.getByTestId("azure-sprint-card-211")).toBeVisible();
});

test("opens a sprint card and offers its task actions", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-sprints-mode").click();
  await choose(page, "azure-sprint-team-select", "Offers Team");
  await choose(page, "azure-sprint-select", /Sprint 23/);
  await page.getByTestId("azure-sprint-card-211").click();
  await expect(page.getByRole("dialog")).toContainText("Build the checkout form");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByTestId("azure-card-task-actions-211").getByRole("button").click();
  await expect(page.getByRole("menuitem").first()).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.keyboard.press("Escape");
});

test("narrows work items to a team and sprint", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-work-items-mode").click();
  await choose(page, "azure-devops-filter-team", "Offers Team");
  await choose(page, "azure-devops-filter-sprint", /Sprint 23/);
  await search(page);
  await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
  await choose(page, "azure-devops-filter-sprint", /Sprint 22/);
  await search(page);
  await expect(page.getByTestId("azure-work-item-row-101")).toHaveCount(0);
  await choose(page, "azure-devops-filter-team", "All teams");
  await search(page);
  await expect(page.getByTestId("azure-work-item-row-101")).toBeVisible();
});

test("lists pull requests of every status with All", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-pull-requests-mode").click();
  await search(page);
  const results = page.getByTestId("azure-devops-pull-request-results");
  await expect(results).toContainText("Rotate integration credentials");
  await expect(results).not.toContainText("Retire legacy tokens");
  await page.getByRole("combobox", { name: "Status" }).first().click();
  await page.getByRole("option", { name: "All", exact: true }).click();
  await search(page);
  await expect(results).toContainText("Retire legacy tokens");
  await page.getByRole("combobox", { name: "Status" }).first().click();
  await page.getByRole("option", { name: "Active", exact: true }).click();
});

test("shows a pull-request filter hidden under Advanced", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-pull-requests-mode").click();
  const scopeBar = page.getByTestId("azure-devops-presets-scope-bar");
  await page.getByRole("button", { name: "Advanced" }).first().click();
  await choose(page, "azure-devops-filter-reviewer", /^Me/);
  await page.getByRole("button", { name: /Advanced/ }).first().click();
  await expect(page.getByTestId("azure-devops-advanced-count").first()).toHaveText("1");
  await expect(scopeBar.getByRole("button", { name: "Review requested" })).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: /Advanced/ }).first().click();
  await choose(page, "azure-devops-filter-reviewer", "Anyone");
  await expect(page.getByTestId("azure-devops-advanced-count")).toHaveCount(0);
  await expect(scopeBar.getByRole("button", { name: "Open" })).toHaveAttribute("aria-pressed", "true");
});

test("filters pull requests by people picked by name", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-pull-requests-mode").click();
  await page.getByRole("button", { name: "Advanced" }).first().click();
  await expect(page.getByTestId("azure-devops-filter-creator")).toContainText("Anyone");
  await page.getByTestId("azure-devops-filter-reviewer").click();
  await expect(page.getByRole("option", { name: /^Me/ })).toBeVisible();
  await page.getByRole("option", { name: /Grace Reviewer/ }).click();
  await expect(page.getByTestId("azure-devops-filter-reviewer")).toContainText("Grace Reviewer");
  await page.getByRole("combobox", { name: "Status" }).first().click();
  await page.getByRole("option", { name: "All", exact: true }).click();
  await search(page);
  const results = page.getByTestId("azure-devops-pull-request-results");
  await expect(results).toContainText("Rotate integration credentials");
  await expect(results).not.toContainText("Retire legacy tokens");
  await choose(page, "azure-devops-filter-reviewer", "Anyone");
  await page.getByRole("combobox", { name: "Status" }).first().click();
  await page.getByRole("option", { name: "Active", exact: true }).click();
});

test("names the signed-in user under Me instead of listing them twice", async ({ page }) => {
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-pull-requests-mode").click();
  await page.getByRole("button", { name: /Advanced/ }).first().click();
  await page.getByTestId("azure-devops-filter-creator").click();
  await expect(page.getByRole("option")).toHaveText(["Anyone", /^Me - Ada Reviewer/]);
  await page.keyboard.press("Escape");
});

test("picks pull-request watch people by name", async ({ page }) => {
  await openSettings(page);
  await page.getByTestId("azure-add-pull-request-watch").click();
  await page.getByTestId("azure-pull_request-watch-project").click();
  await page.getByRole("option", { name: "Platform", exact: true }).click();
  await page.getByTestId("azure-pull-request-watch-reviewer").click();
  await expect(page.getByRole("option", { name: /^Me/ })).toContainText("Ada Reviewer");
  await page.getByRole("option", { name: /Grace Reviewer/ }).click();
  await expect(page.getByTestId("azure-pull-request-watch-reviewer")).toContainText("Grace Reviewer");
});

test("starts a task from a work item and links it", async ({ page, request }) => {
  const workspace = await workspaceId(request);
  await page.goto("/azure-devops");
  await page.getByTestId("azure-devops-work-items-mode").click();
  await search(page);
  await page.getByRole("button", { name: /Task actions for work item 101/ }).click();
  await page.getByRole("menuitem").first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("_workitems/edit/101");
  await dialog.getByRole("radio", { name: "None", exact: true }).click();
  await dialog.getByRole("button", { name: "Start task", exact: true }).click();
  await expect(page).toHaveURL(/\/t\/[^/?]+/);
  await expect
    .poll(async () => Object.keys((await action<{ workItems: Record<string, unknown> }>(request, "task_links.workspace", { workspaceId: workspace, body: {} })).workItems).length)
    .toBeGreaterThan(0);
});

test("links a pull request from the Link menu and shows its status", async ({ page, request }) => {
  const workspace = await workspaceId(request);
  const links = await action<{ workItems: Record<string, unknown> }>(request, "task_links.workspace", { workspaceId: workspace, body: {} });
  const taskId = Object.keys(links.workItems)[0];
  await page.goto(`/t/${taskId}`);
  // The row's action button appears on hover.
  await page.getByRole("button", { name: /Implement the Azure DevOps work item/ }).first().hover();
  await page.getByRole("button", { name: "Task actions", exact: true }).first().click();
  await page.getByRole("menuitem", { name: "Link" }).hover();
  await page.getByRole("menuitem", { name: "Azure DevOps Pull Request" }).click();
  await page.getByTestId("azure-devops-link-pullRequest-reference").fill(`${ORGANIZATION_URL}/Platform/_git/api/pullrequest/42`);
  await page.getByTestId("azure-devops-link-pullRequest-submit").click();
  await expect(page.getByRole("button", { name: /#42/ }).first()).toBeVisible();
  const status = await action<{ pullRequests: Array<{ taskStatus?: { state: string } }> }>(request, "task_links.get", { taskId, body: {} });
  expect(status.pullRequests[0].taskStatus?.state).toBe("open");
});

test("creates a work-item watch in settings and runs it once per item", async ({ page, request }) => {
  const workspace = await workspaceId(request);
  const launch = await launchOptions(request, workspace);
  await openSettings(page);
  await page.getByTestId("azure-add-work-item-watch").click();
  const editor = page.getByTestId("azure-watch-editor");
  await editor.getByTestId("azure-work-item-watch-wiql").fill("SELECT [System.Id] FROM WorkItems");
  await choose(page, "azure-work_item-watch-workflow", /.+/);
  await choose(page, "azure-work_item-watch-agent", "Mock");
  await choose(page, "azure-work_item-watch-executor", "Local");
  await editor.getByTestId("azure-watch-save").click();
  await expect(editor).toHaveCount(0);
  const { watches } = await action<{ watches: Array<{ id: string; kind: string; workflowStepId: string }> }>(request, "watches.list", { workspaceId: workspace, body: {} });
  const watch = watches.find((candidate) => candidate.kind === "work_item")!;
  expect(watch.workflowStepId).toBe(launch.stepId);
  const before = (await tasks(request, workspace)).length;
  await page.getByTestId(`azure-watch-run-${watch.id}`).click();
  await expect(page.getByRole("status")).toContainText(/1/);
  await page.getByTestId(`azure-watch-run-${watch.id}`).click();
  await expect(page.getByRole("status")).toContainText(/0/);
  const created = (await tasks(request, workspace)).filter((task) => JSON.stringify(task.metadata ?? {}).includes(watch.id));
  expect(created).toHaveLength(1);
  expect((await tasks(request, workspace)).length).toBe(before + 1);
});

test("disables a watch whose executor is no longer available", async ({ page, request }) => {
  const workspace = await workspaceId(request);
  const launch = await launchOptions(request, workspace);
  const { watch } = await action<{ watch: { id: string } }>(request, "watches.create", {
    workspaceId: workspace,
    body: {
      kind: "pull_request", projectId: "Platform", workflowId: launch.workflowId, workflowStepId: launch.stepId,
      agentProfileId: launch.agentProfileId, executorProfileId: "removed-executor", prompt: "Review {{pull_request.title}}",
    },
  });
  await action(request, "watches.trigger", { workspaceId: workspace, body: { id: watch.id } });
  await openSettings(page);
  await expect(page.getByTestId(`azure-watch-disabled-reason-${watch.id}`)).toContainText("removed-executor");
});

test("enables and disables the integration per workspace from Kandev's settings", async ({ page, request }) => {
  const workspace = await workspaceId(request);
  const index = `/settings/workspaces/${workspace}/integrations`;
  const save = page.getByTestId("settings-floating-save").getByRole("button", { name: "Save changes" });
  const toggle = page.getByTestId(`integration-card-${PLUGIN_ID}-azure-devops`).getByRole("switch", { name: "Enable Azure DevOps" });

  await page.goto(index);
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await expect(page).toHaveURL(new RegExp(`${index}$`));
  await save.click();
  await expect(save).not.toBeVisible();

  await page.reload();
  await expect(toggle).toHaveAttribute("aria-checked", "false");

  await page.goto(`${index}/azure-devops`);
  const headerToggle = page.getByRole("switch", { name: "Enable Azure DevOps" });
  await expect(headerToggle).toHaveAttribute("aria-checked", "false");
  await headerToggle.click();
  await save.click();
  await expect(save).not.toBeVisible();

  await page.goto(index);
  await expect(toggle).toHaveAttribute("aria-checked", "true");
});
