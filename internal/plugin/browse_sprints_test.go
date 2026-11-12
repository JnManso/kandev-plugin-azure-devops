package plugin

import (
	"errors"
	"testing"
)

func TestTeamsListNamesTheTeamsOfTheConnectedIdentity(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	status, body := h.action("ws-1", "teams.list", map[string]any{"projectId": "p1"})
	if status != 200 || flatten(body["mineIds"]) != `["team-1"]` || len(body["teams"].([]any)) != 1 {
		t.Fatalf("teams.list status = %d body = %v", status, body)
	}
}

func TestTeamsListStillListsTeamsWhenMyTeamsFail(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	h.azure.myTeamsErr = errors.New("forbidden")
	status, body := h.action("ws-1", "teams.list", map[string]any{"projectId": "p1"})
	if status != 200 || flatten(body["mineIds"]) != `[]` || len(body["teams"].([]any)) != 1 {
		t.Fatalf("teams.list status = %d body = %v", status, body)
	}
}

func TestTeamSettingsListSprints(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	if status, _ := h.action("ws-1", "teams.settings", map[string]any{"projectId": "p1"}); status != 400 {
		t.Fatalf("missing team status = %d, want 400", status)
	}
	status, body := h.action("ws-1", "teams.settings", map[string]any{"projectId": "p1", "teamId": "team-2"})
	if status != 200 || len(body["iterations"].([]any)) != 3 || field(body, "areaField") != "System.AreaPath" {
		t.Fatalf("teams.settings status = %d body = %v", status, body)
	}
}

func TestSprintBoardReturnsRowsWithoutProviderFields(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	if status, _ := h.action("ws-1", "sprints.board", map[string]any{"projectId": "p1", "teamId": "team-2"}); status != 400 {
		t.Fatalf("missing sprint status = %d, want 400", status)
	}
	status, body := h.action("ws-1", "sprints.board", map[string]any{"projectId": "p1", "teamId": "team-2", "iterationId": "sprint-23"})
	rows, _ := body["rows"].([]any)
	if status != 200 || len(rows) != 3 || field(body, "iteration", "name") != "Sprint 23" {
		t.Fatalf("sprints.board status = %d body = %v", status, body)
	}
	parent := field(rows[1], "parent")
	task := rows[1].(map[string]any)["tasks"].([]any)[0]
	if field(parent, "title") != "Redesign the offer checkout" || field(parent, "fields") != nil ||
		field(task, "fields") != nil || field(task, "columnId") != "col-doing" {
		t.Fatalf("row = %v", rows[1])
	}
	if status, _ := h.action("ws-1", "sprints.board", map[string]any{"projectId": "p1", "teamId": "team-2", "iterationId": "nope"}); status != 404 {
		t.Fatalf("unknown sprint status = %d, want 404", status)
	}
}

func TestSavedWorkItemViewsKeepTheirTeamAndSprint(t *testing.T) {
	h := newHarness(t)
	view := map[string]any{
		"id": "offers-sprint", "kind": "work_item", "label": "Offers sprint", "projectId": "p1",
		"wiql": "SELECT [System.Id] FROM WorkItems", "teamId": " team-2 ", "iterationId": "sprint-23",
	}
	if status, body := h.action("ws-1", "views.save", map[string]any{"views": []any{view}}); status != 200 {
		t.Fatalf("save status = %d body = %v", status, body)
	}
	_, loaded := h.action("ws-1", "views.get", nil)
	saved := loaded["views"].([]any)[0]
	if field(saved, "teamId") != "team-2" || field(saved, "iterationId") != "sprint-23" {
		t.Fatalf("saved view = %v", saved)
	}
	view["teamId"], view["id"] = "", "sprint-only"
	if status, _ := h.action("ws-1", "views.save", map[string]any{"views": []any{view}}); status != 400 {
		t.Fatalf("sprint without team status = %d, want 400", status)
	}
}
