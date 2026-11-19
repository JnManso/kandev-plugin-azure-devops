package azure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// sprintServer serves one team's settings, one sprint, and its work items.
func sprintServer(t *testing.T, sprintItems int) *httptest.Server {
	t.Helper()
	const team = "/acme/project-1/team-1/_apis/work"
	routes := map[string]any{
		"/acme/_apis/projects/project-1/teams": map[string]any{"value": []map[string]string{{"id": "team-1", "name": "Offers"}}},
		team + "/teamsettings/teamfieldvalues": map[string]any{
			"field":  map[string]string{"referenceName": "System.AreaPath"},
			"values": []map[string]any{{"value": `Contoso\Offers`, "includeChildren": true}, {"value": `Contoso\Shared`, "includeChildren": false}},
		},
		team + "/teamsettings/iterations": map[string]any{"value": []map[string]any{
			{"id": "it-1", "name": "Sprint 22", "path": `Contoso\Sprint 22`, "attributes": map[string]string{"startDate": "2026-10-26T00:00:00Z", "finishDate": "2026-11-06T00:00:00Z", "timeFrame": "past"}},
			{"id": "it-2", "name": "Sprint 23", "path": `Contoso\Sprint 23`, "attributes": map[string]string{"startDate": "2026-11-09T00:00:00Z", "finishDate": "2026-11-20T00:00:00Z", "timeFrame": "current"}},
		}},
		team + "/teamsettings/iterations/it-2": map[string]any{
			"id": "it-2", "name": "Sprint 23", "path": `Contoso\Sprint 23`,
			"attributes": map[string]string{"startDate": "2026-11-09T00:00:00Z", "finishDate": "2026-11-20T00:00:00Z", "timeFrame": "current"},
		},
		team + "/teamsettings":                             map[string]any{"workingDays": []string{"monday", "tuesday", "wednesday", "thursday", "friday"}},
		team + "/teamsettings/iterations/it-2/teamdaysoff": map[string]any{"daysOff": []map[string]string{{"start": "2026-11-13T00:00:00Z", "end": "2026-11-13T00:00:00Z"}}},
		team + "/taskboardcolumns": map[string]any{"columns": []map[string]any{
			{"id": "todo", "name": "To Do", "mappings": []map[string]string{{"workItemType": "Task", "state": "New"}}},
			{"id": "doing", "name": "In Progress", "mappings": []map[string]string{{"workItemType": "Task", "state": "Active"}}},
		}},
	}
	relations := []map[string]any{{"rel": nil, "source": nil, "target": map[string]int{"id": 1}}}
	for id := 2; id <= sprintItems; id++ {
		relations = append(relations, map[string]any{
			"rel": "System.LinkTypes.Hierarchy-Forward", "source": map[string]int{"id": 1}, "target": map[string]int{"id": id},
		})
	}
	routes[team+"/teamsettings/iterations/it-2/workitems"] = map[string]any{"workItemRelations": relations}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/acme/project-1/_apis/wit/workitemsbatch" {
			var request struct {
				IDs []int `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			value := make([]map[string]any, 0, len(request.IDs))
			for _, id := range request.IDs {
				itemType, state, assignee := "Task", "Active", "Ana"
				if id == 1 {
					itemType, state, assignee = "User Story", "Active", "Rui"
				}
				value = append(value, map[string]any{"id": id, "rev": 1, "fields": map[string]any{
					"System.Title": "Item", "System.WorkItemType": itemType, "System.State": state,
					"System.AssignedTo": map[string]string{"displayName": assignee},
				}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": value})
			return
		}
		if r.URL.Path == "/acme/_apis/projects/project-1/teams" && r.URL.Query().Get("$mine") != "true" {
			http.NotFound(w, r)
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRESTClientListsTheTeamsOfTheConnectedIdentity(t *testing.T) {
	client := newTestRESTClient(t, sprintServer(t, 1), "pat")
	ids, err := client.ListMyTeamIDs(context.Background(), "project-1")
	if err != nil || !reflect.DeepEqual(ids, []string{"team-1"}) {
		t.Fatalf("ListMyTeamIDs = %v, %v", ids, err)
	}
}

func TestRESTClientReadsTeamAreasAndSprints(t *testing.T) {
	client := newTestRESTClient(t, sprintServer(t, 1), "pat")
	settings, err := client.GetTeamSettings(context.Background(), "project-1", "team-1")
	if err != nil {
		t.Fatalf("GetTeamSettings: %v", err)
	}
	wantAreas := []TeamArea{{Path: `Contoso\Offers`, IncludeChildren: true}, {Path: `Contoso\Shared`}}
	if settings.AreaField != "System.AreaPath" || !reflect.DeepEqual(settings.Areas, wantAreas) {
		t.Fatalf("areas = %q %+v", settings.AreaField, settings.Areas)
	}
	if len(settings.Iterations) != 2 || settings.Iterations[1] != (Iteration{
		ID: "it-2", Name: "Sprint 23", Path: `Contoso\Sprint 23`,
		Start: "2026-11-09T00:00:00Z", Finish: "2026-11-20T00:00:00Z", TimeFrame: "current",
	}) {
		t.Fatalf("iterations = %+v", settings.Iterations)
	}
}

func TestRESTClientBuildsTheSprintTaskboard(t *testing.T) {
	client := newTestRESTClient(t, sprintServer(t, 3), "pat")
	board, err := client.GetSprintBoard(context.Background(), "project-1", "team-1", "it-2", day("2026-11-11"))
	if err != nil {
		t.Fatalf("GetSprintBoard: %v", err)
	}
	if board.Iteration.Name != "Sprint 23" || board.WorkingDaysLeft != 7 || board.Truncated {
		t.Fatalf("board = %+v", board)
	}
	if len(board.Columns) != 2 || len(board.Rows) != 1 || board.Rows[0].Parent.ID != 1 || len(board.Rows[0].Tasks) != 2 {
		t.Fatalf("rows = %+v", board.Rows)
	}
	if board.Rows[0].Tasks[0].ColumnID != "doing" || !reflect.DeepEqual(board.People, []string{"Ana", "Rui"}) {
		t.Fatalf("cards = %+v people = %v", board.Rows[0].Tasks, board.People)
	}
}

func TestRESTClientBoundsTheSprintTaskboard(t *testing.T) {
	client := newTestRESTClient(t, sprintServer(t, maxSprintCards+5), "pat")
	board, err := client.GetSprintBoard(context.Background(), "project-1", "team-1", "it-2", day("2026-11-11"))
	if err != nil {
		t.Fatalf("GetSprintBoard: %v", err)
	}
	cards := 0
	for _, row := range board.Rows {
		cards += len(row.Tasks)
		if row.Parent != nil {
			cards++
		}
	}
	if !board.Truncated || cards != maxSprintCards {
		t.Fatalf("truncated = %v cards = %d, want true and %d", board.Truncated, cards, maxSprintCards)
	}
}

func TestRESTClientUsesTheTaskStatesWhenTheTaskboardIsNotCustomized(t *testing.T) {
	base := sprintServer(t, 3)
	const team = "/acme/project-1/team-1/_apis/work"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case team + "/taskboardcolumns":
			_ = json.NewEncoder(w).Encode(map[string]any{"columns": []any{}, "isCustomized": false})
		case team + "/backlogs":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{
				{"id": "Microsoft.RequirementCategory", "type": "requirement", "workItemTypes": []map[string]string{{"name": "User Story"}}},
				{"id": "Microsoft.TaskCategory", "type": "task", "workItemTypes": []map[string]string{{"name": "Task"}}, "defaultWorkItemType": map[string]string{"name": "Task"}},
			}})
		case "/acme/project-1/_apis/wit/workitemtypes/Task/states":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]string{
				{"name": "New", "category": "Proposed"}, {"name": "Active", "category": "InProgress"},
				{"name": "Closed", "category": "Completed"}, {"name": "Removed", "category": "Removed"},
			}})
		default:
			base.Config.Handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	board, err := newTestRESTClient(t, server, "pat").GetSprintBoard(context.Background(), "project-1", "team-1", "it-2", day("2026-11-11"))
	if err != nil {
		t.Fatalf("GetSprintBoard: %v", err)
	}
	var names []string
	for _, column := range board.Columns {
		names = append(names, column.Name)
	}
	if !reflect.DeepEqual(names, []string{"New", "Active", "Closed"}) {
		t.Fatalf("columns = %v", names)
	}
	if len(board.Rows) != 1 || board.Rows[0].Parent == nil || board.Rows[0].Tasks[0].ColumnID != "Active" {
		t.Fatalf("rows = %+v", board.Rows)
	}
}
