package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// pagedServer serves count named records the way Azure DevOps does: at most
// $top per response (100 when unset), starting at $skip.
func pagedServer(t *testing.T, path, prefix string, count int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		top, skip := 100, 0
		if value := r.URL.Query().Get("$top"); value != "" {
			top, _ = strconv.Atoi(value)
		}
		if value := r.URL.Query().Get("$skip"); value != "" {
			skip, _ = strconv.Atoi(value)
		}
		top = min(top, 100)
		values := []map[string]string{}
		// Newest first, like Azure DevOps, so the result is not already sorted.
		for index := count - 1 - skip; index >= 0 && len(values) < top; index-- {
			values = append(values, map[string]string{"id": fmt.Sprintf("%s-%03d", prefix, index), "name": fmt.Sprintf("%s %03d", prefix, index)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(values), "value": values})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRESTClientListsEveryTeamSortedByName(t *testing.T) {
	client := newTestRESTClient(t, pagedServer(t, "/acme/_apis/projects/project-1/teams", "Team", 250), "pat")
	teams, err := client.ListTeams(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 250 || teams[0].Name != "Team 000" || teams[249].Name != "Team 249" {
		t.Fatalf("teams = %d, first %+v, last %+v; want 250 sorted by name", len(teams), teams[0], teams[len(teams)-1])
	}
}

func TestRESTClientListsEveryProjectSortedByName(t *testing.T) {
	client := newTestRESTClient(t, pagedServer(t, "/acme/_apis/projects", "Project", 130), "pat")
	projects, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 130 || projects[0].Name != "Project 000" || projects[129].Name != "Project 129" {
		t.Fatalf("projects = %d, first %+v, last %+v; want 130 sorted by name", len(projects), projects[0], projects[len(projects)-1])
	}
}
