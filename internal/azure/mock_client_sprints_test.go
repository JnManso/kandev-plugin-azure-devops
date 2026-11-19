package azure

import (
	"context"
	"reflect"
	"testing"
)

func seededMock(t *testing.T) *MockClient {
	t.Helper()
	client, err := NewSeededMockClient()
	if err != nil {
		t.Fatalf("NewSeededMockClient: %v", err)
	}
	return client
}

func TestMockListsTheTeamsOfTheSignedInUser(t *testing.T) {
	ids, err := seededMock(t).ListMyTeamIDs(context.Background(), "project-1")
	if err != nil || !reflect.DeepEqual(ids, []string{"team-2"}) {
		t.Fatalf("ListMyTeamIDs = %v, %v", ids, err)
	}
}

func TestMockTeamSettingsListSprintsAndAreas(t *testing.T) {
	client := seededMock(t)
	settings, err := client.GetTeamSettings(context.Background(), "project-1", "team-2")
	if err != nil || len(settings.Iterations) != 3 || settings.Iterations[1].TimeFrame != "current" || len(settings.Areas) != 1 {
		t.Fatalf("settings = %+v, %v", settings, err)
	}
	empty, err := client.GetTeamSettings(context.Background(), "project-1", "team-1")
	if err != nil || len(empty.Iterations) != 0 || empty.Areas == nil {
		t.Fatalf("team without sprints = %+v, %v", empty, err)
	}
}

func TestMockBuildsTheSprintTaskboard(t *testing.T) {
	client := seededMock(t)
	board, err := client.GetSprintBoard(context.Background(), "project-1", "team-2", "sprint-23", day("2026-11-11"))
	if err != nil {
		t.Fatalf("GetSprintBoard: %v", err)
	}
	if got := rowSummary(board.Rows); !reflect.DeepEqual(got, [][]int{{0, 221}, {201, 211, 212}, {202, 213}}) {
		t.Fatalf("rows = %v", got)
	}
	if board.Iteration.Name != "Sprint 23" || board.WorkingDaysLeft != 8 || len(board.Columns) != 3 {
		t.Fatalf("board = %+v", board)
	}
	if !reflect.DeepEqual(board.People, []string{"Ada Reviewer", "Bruno Costa"}) {
		t.Fatalf("people = %v", board.People)
	}
	empty, err := client.GetSprintBoard(context.Background(), "project-1", "team-2", "sprint-22", day("2026-11-11"))
	if err != nil || len(empty.Rows) != 0 {
		t.Fatalf("empty sprint = %+v, %v", empty, err)
	}
}

func TestMockOpensASprintWorkItem(t *testing.T) {
	detail, err := seededMock(t).GetWorkItemDetail(context.Background(), "project-1", 211)
	if err != nil || detail.Title != "Build the checkout form" {
		t.Fatalf("detail = %+v, %v", detail, err)
	}
}

func TestMockWIQLHonoursTeamAreaAndSprintConditions(t *testing.T) {
	item := WorkItem{ID: 1, AreaPath: `Platform\Offers\Web`, Fields: map[string]any{"System.IterationPath": `Platform\Sprint 23`}}
	cases := map[string]bool{
		`SELECT [System.Id] FROM WorkItems WHERE ([System.AreaPath] UNDER 'Platform\Offers')`:                                         true,
		`SELECT [System.Id] FROM WorkItems WHERE ([System.AreaPath] = 'Platform\Offers')`:                                             false,
		`SELECT [System.Id] FROM WorkItems WHERE ([System.AreaPath] = 'Platform\Other' OR [System.AreaPath] UNDER 'Platform\Offers')`: true,
		`SELECT [System.Id] FROM WorkItems WHERE [System.IterationPath] = 'Platform\Sprint 23'`:                                       true,
		`SELECT [System.Id] FROM WorkItems WHERE [System.IterationPath] = 'Platform\Sprint 22'`:                                       false,
	}
	for wiql, want := range cases {
		if got := mockWIQLMatches(wiql, item); got != want {
			t.Errorf("mockWIQLMatches(%s) = %v, want %v", wiql, got, want)
		}
	}
}
