import { describe, expect, it } from "vitest";
import {
  editListDrafts,
  listDraftsDirty,
  listDraftsEditable,
  listDraftsFrom,
  listDraftsSaveBody,
  resetListDrafts,
} from "./settings-drafts";

const defaults = { workItems: ["default work item"], pullRequests: ["default pull request"] };
const stored = { workItems: ["custom work item"], pullRequests: ["custom pull request"] };

describe("settings list drafts", () => {
  it("restores the defaults when Reset is saved unchanged", () => {
    const drafts = resetListDrafts(defaults);
    expect(listDraftsDirty(drafts, stored)).toBe(true);
    expect(listDraftsSaveBody(drafts)).toEqual({ workItems: null, pullRequests: null });
  });

  it("saves edits made after Reset instead of restoring the defaults", () => {
    const drafts = editListDrafts(resetListDrafts(defaults), "pullRequests", [
      "default pull request",
      "added after reset",
    ]);
    expect(listDraftsSaveBody(drafts)).toEqual({
      workItems: ["default work item"],
      pullRequests: ["default pull request", "added after reset"],
    });
    expect(listDraftsDirty(drafts, stored)).toBe(true);
  });

  it("is clean when the edits match the stored lists", () => {
    const drafts = editListDrafts(listDraftsFrom(stored), "workItems", ["custom work item"]);
    expect(listDraftsDirty(drafts, stored)).toBe(false);
  });

  it("allows editing only after the stored lists were read", () => {
    expect(listDraftsEditable({ loading: true, loadError: null })).toBe(false);
    expect(listDraftsEditable({ loading: false, loadError: "offline" })).toBe(false);
    expect(listDraftsEditable({ loading: false, loadError: null })).toBe(true);
  });
});
