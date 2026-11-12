package azure

import (
	"context"
	"testing"
)

// The mock follows Azure DevOps: no status means active, and "all" means
// every status.
func TestMockClientPullRequestStatusMatchesAzureDevOps(t *testing.T) {
	client := NewMockClient()
	client.Seed(MockState{PullRequests: []PullRequest{
		{ID: 1, Status: "active", RepositoryID: "r"},
		{ID: 2, Status: "completed", RepositoryID: "r"},
		{ID: 3, Status: "abandoned", RepositoryID: "r"},
	}})
	counts := map[string]int{"": 1, "active": 1, "completed": 1, "all": 3}
	for status, want := range counts {
		page, err := client.ListPullRequests(context.Background(), PullRequestFilter{RepositoryID: "r", Status: status})
		if err != nil || len(page.Items) != want {
			t.Fatalf("status %q = %d items, %v; want %d", status, len(page.Items), err, want)
		}
	}
}

// The mock honours the two searches the browse page builds: a work item
// number and words in the title.
func TestMockClientWIQLSearchesByIDAndTitle(t *testing.T) {
	client := NewMockClient()
	client.Seed(MockState{WorkItems: []WorkItem{{ID: 101, Title: "Handle token rotation"}, {ID: 102, Title: "Fix build"}}})
	cases := map[string][]int{
		"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.Id] = 102":                                       {102},
		"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.Title] CONTAINS 'ROTATION' ORDER BY [System.Id]": {101},
		"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.Title] CONTAINS 'nothing'":                       {},
		"SELECT [System.Id] FROM WorkItems WHERE [System.Title] CONTAINS 'rotation' AND [System.Title] CONTAINS 'handle'":                     {101},
		"SELECT [System.Id] FROM WorkItems WHERE [System.Title] CONTAINS 'rotation' AND [System.Title] CONTAINS 'build'":                      {},
		"SELECT [System.Id] FROM WorkItems": {101, 102},
	}
	for wiql, want := range cases {
		result, err := client.QueryWIQL(context.Background(), "", wiql, 50)
		if err != nil || len(result.Items) != len(want) {
			t.Fatalf("%s = %+v, %v; want %v", wiql, result, err, want)
		}
		for index, id := range want {
			if result.Items[index].ID != id {
				t.Fatalf("%s item %d = %d, want %d", wiql, index, result.Items[index].ID, id)
			}
		}
	}
}
