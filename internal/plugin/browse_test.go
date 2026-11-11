package plugin

import (
	"strings"
	"testing"
)

func TestBrowseRequiresAUsableConnection(t *testing.T) {
	h := newHarness(t)
	if status, _ := h.action("ws-1", "teams.list", map[string]any{"projectId": "p1"}); status != 409 {
		t.Fatalf("unconfigured status = %d, want 409", status)
	}
	h.connect("ws-1")
	if status, _ := h.action("ws-1", "teams.list", map[string]any{}); status != 400 {
		t.Fatalf("missing project status = %d, want 400", status)
	}
	if status, _ := h.action("ws-1", "no_such.action", map[string]any{}); status != 404 {
		t.Fatalf("unknown action status = %d, want 404", status)
	}
}

func TestBoardBrowsingDropsProviderFieldsAndMovesCards(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	_, teams := h.action("ws-1", "teams.list", map[string]any{"projectId": "p1"})
	_, boards := h.action("ws-1", "boards.list", map[string]any{"projectId": "p1", "teamId": "team-1"})
	if len(teams["teams"].([]any)) != 1 || len(boards["boards"].([]any)) != 2 {
		t.Fatalf("teams = %v boards = %v", teams, boards)
	}
	status, board := h.action("ws-1", "boards.get", map[string]any{"projectId": "p1", "teamId": "team-1", "boardId": "board-2"})
	item := board["items"].([]any)[0]
	if status != 200 || field(item, "title") != "Plan the next release" || field(item, "fields") != nil || field(item, "description") != nil {
		t.Fatalf("board status = %d item = %v", status, item)
	}
	status, moved := h.action("ws-1", "boards.update_item", map[string]any{
		"projectId": "p1", "teamId": "team-1", "boardId": "board-1", "id": 101, "revision": 3, "columnId": "col-active",
	})
	if status != 200 || field(moved, "item", "columnId") != "col-active" {
		t.Fatalf("move status = %d body = %v", status, moved)
	}
	if status, _ := h.action("ws-1", "boards.update_item", map[string]any{"projectId": "p1", "teamId": "team-1", "boardId": "board-1", "id": 101, "revision": 3, "assigneeAction": "promote"}); status != 400 {
		t.Fatalf("unknown assignee action status = %d, want 400", status)
	}
}

func TestWorkItemSearchDetailCommentsAndAssignment(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	status, search := h.action("ws-1", "work_items.search", map[string]any{"projectId": "p1", "wiql": "SELECT [System.Id] FROM WorkItems", "top": 1000})
	items := search["items"].([]any)
	if status != 200 || len(items) != 2 || field(items[0], "fields") != nil {
		t.Fatalf("search status = %d body = %v", status, search)
	}
	if status, body := h.action("ws-1", "work_items.search", map[string]any{"projectId": "p1", "wiql": "BAD"}); status != 400 || !strings.Contains(body["error"].(string), "query") {
		t.Fatalf("bad query status = %d body = %v", status, body)
	}
	_, detail := h.action("ws-1", "work_items.get", map[string]any{"projectId": "p1", "id": 101})
	if field(detail, "item", "description") != "<p>Rotate the credentials safely.</p>" || field(detail, "item", "fields") != nil {
		t.Fatalf("detail = %v", detail)
	}
	if status, _ := h.action("ws-1", "work_items.get", map[string]any{"projectId": "p1", "id": 999}); status != 404 {
		t.Fatalf("missing work item status = %d, want 404", status)
	}
	_, comments := h.action("ws-1", "work_items.comments", map[string]any{"projectId": "p1", "id": 101, "continuationToken": "t1"})
	if len(comments["comments"].([]any)) != 1 || comments["continuationToken"] != "t1next" {
		t.Fatalf("comments = %v", comments)
	}
	status, updated := h.action("ws-1", "work_items.update", map[string]any{"projectId": "p1", "id": 101, "revision": 3, "assigneeAction": "assign_current_user"})
	if status != 200 || field(updated, "item", "revision") != float64(4) || len(h.azure.browse.workItemUpdates) != 1 {
		t.Fatalf("assignment status = %d body = %v", status, updated)
	}
}

func TestPullRequestsPageWithCursorsAndResolveCurrentUser(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	_, first := h.action("ws-1", "pull_requests.list", map[string]any{"projectId": "p1", "repositoryId": "guid-api", "top": 2, "reviewer": "@me"})
	cursor, _ := first["nextCursor"].(string)
	if len(first["items"].([]any)) != 2 || cursor == "" {
		t.Fatalf("first page = %v", first)
	}
	_, second := h.action("ws-1", "pull_requests.list", map[string]any{"projectId": "p1", "repositoryId": "guid-api", "top": 2, "cursor": cursor})
	if len(second["items"].([]any)) != 1 || second["nextCursor"] != nil {
		t.Fatalf("second page = %v", second)
	}
	if h.azure.browse.pullRequestSkip[1] != 2 || h.azure.browse.pullRequestUser[0] != "|user-1" {
		t.Fatalf("skips = %v users = %v", h.azure.browse.pullRequestSkip, h.azure.browse.pullRequestUser)
	}
	if status, _ := h.action("ws-1", "pull_requests.list", map[string]any{"projectId": "p1", "repositoryId": "guid-api", "cursor": "garbage!"}); status != 400 {
		t.Fatalf("invalid cursor status = %d, want 400", status)
	}
}

func TestPullRequestFeedbackSummarizesReviewsAndPolicies(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	status, feedback := h.action("ws-1", "pull_requests.feedback", map[string]any{"projectId": "p1", "repositoryId": "guid-api", "id": 7})
	if status != 200 || feedback["reviewState"] != "approved" || feedback["policyState"] != "pending" ||
		len(feedback["threads"].([]any)) != 0 || field(feedback, "pullRequest", "title") != "Rotate integration credentials" {
		t.Fatalf("status = %d feedback = %v", status, feedback)
	}
}

func TestSavedViewsValidateAndRoundTrip(t *testing.T) {
	h := newHarness(t)
	view := map[string]any{"id": "security-queue", "kind": "work_item", "label": "Security queue", "projectId": "p1", "wiql": "SELECT [System.Id] FROM WorkItems"}
	status, saved := h.action("ws-1", "views.save", map[string]any{"views": []any{view}})
	if status != 200 || field(saved["views"].([]any)[0], "top") != float64(50) {
		t.Fatalf("save status = %d body = %v", status, saved)
	}
	_, loaded := h.action("ws-1", "views.get", nil)
	if len(loaded["views"].([]any)) != 1 {
		t.Fatalf("views = %v", loaded)
	}
	for name, invalid := range map[string]map[string]any{
		"no repository": {"id": "v", "kind": "pull_request", "label": "PRs", "projectId": "p1"},
		"bad kind":      {"id": "v", "kind": "other", "label": "X", "projectId": "p1"},
		"no label":      {"id": "v", "kind": "work_item", "projectId": "p1", "wiql": "SELECT"},
	} {
		if status, _ := h.action("ws-1", "views.save", map[string]any{"views": []any{invalid}}); status != 400 {
			t.Fatalf("%s: status = %d, want 400", name, status)
		}
	}
	if status, _ := h.action("ws-1", "views.save", map[string]any{"views": []any{view, view}}); status != 400 {
		t.Fatalf("duplicate ids status = %d, want 400", status)
	}
}

func TestWorkspaceSettingsDefaultPatchAndReset(t *testing.T) {
	h := newHarness(t)
	_, settings := h.action("ws-1", "workspace_settings.get", nil)
	if len(settings["workItemQueries"].([]any)) != 4 || len(settings["pullRequestActions"].([]any)) != 3 {
		t.Fatalf("defaults = %v", settings)
	}
	status, patched := h.action("ws-1", "workspace_settings.patch", map[string]any{
		"workItemQueries": []any{map[string]any{"label": "Team queue", "group": "inbox", "filters": map[string]any{"wiql": "SELECT", "top": 10}}},
	})
	queries := patched["workItemQueries"].([]any)
	if status != 200 || len(queries) != 1 || field(queries[0], "label") != "Team queue" || field(queries[0], "id") == "" ||
		len(patched["pullRequestQueries"].([]any)) != 4 {
		t.Fatalf("patch status = %d body = %v", status, patched)
	}
	if status, _ := h.action("ws-1", "workspace_settings.patch", map[string]any{"workItemQueries": []any{map[string]any{"label": ""}}}); status != 400 {
		t.Fatalf("invalid presets status = %d, want 400", status)
	}
	if status, _ := h.action("ws-1", "workspace_settings.patch", map[string]any{"unknown": []any{}}); status != 400 {
		t.Fatalf("unknown key status = %d, want 400", status)
	}
	_, reset := h.action("ws-1", "workspace_settings.patch", map[string]any{"workItemQueries": nil})
	if len(reset["workItemQueries"].([]any)) != 4 {
		t.Fatalf("reset = %v", reset)
	}
}
