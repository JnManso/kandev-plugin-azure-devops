package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// inFlightServer wraps pagedServer's handler, delaying each response and
// recording the most requests it saw at once.
func inFlightServer(t *testing.T, path, prefix string, count int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	inner := pagedServer(t, path, prefix, count)
	var current, peak atomic.Int32
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := current.Add(1)
		mu.Lock()
		if now > peak.Load() {
			peak.Store(now)
		}
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		current.Add(-1)
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &peak
}

func TestRESTClientListsTeamPagesConcurrently(t *testing.T) {
	server, peak := inFlightServer(t, "/acme/_apis/projects/project-1/teams", "Team", 924)
	client := newTestRESTClient(t, server, "pat")
	teams, err := client.ListTeams(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 924 || teams[0].Name != "Team 000" || teams[923].Name != "Team 923" {
		t.Fatalf("teams = %d, first %+v, last %+v; want 924 sorted by name", len(teams), teams[0], teams[len(teams)-1])
	}
	seen := map[string]bool{}
	for _, team := range teams {
		if seen[team.ID] {
			t.Fatalf("team %s listed twice", team.ID)
		}
		seen[team.ID] = true
	}
	if peak.Load() < 2 {
		t.Fatalf("peak concurrent page requests = %d, want pages fetched concurrently", peak.Load())
	}
}

func TestRESTClientListsAnExactMultipleOfThePageSize(t *testing.T) {
	server, _ := inFlightServer(t, "/acme/_apis/projects/project-1/teams", "Team", 400)
	client := newTestRESTClient(t, server, "pat")
	teams, err := client.ListTeams(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 400 {
		t.Fatalf("teams = %d, want 400", len(teams))
	}
}
