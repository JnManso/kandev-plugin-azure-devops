package azure

import (
	"context"
	"testing"
)

func TestMockListsPullRequestPeopleAndFiltersByThem(t *testing.T) {
	client := seededMock(t)
	people, err := client.ListPullRequestPeople(context.Background(), "project-1", "azure-repo-1")
	if err != nil || len(people.Creators) != 1 || people.Creators[0].ID != "user-1" || len(people.Reviewers) != 1 || people.Reviewers[0].DisplayName != "Grace Reviewer" {
		t.Fatalf("people = %+v, %v", people, err)
	}
	page, err := client.ListPullRequests(context.Background(), PullRequestFilter{RepositoryID: "azure-repo-1", Status: "all", ReviewerID: "user-2"})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != 42 {
		t.Fatalf("reviewer filter = %+v, %v", page, err)
	}
	page, err = client.ListPullRequests(context.Background(), PullRequestFilter{RepositoryID: "azure-repo-1", Status: "all", CreatorID: "user-2"})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("creator filter = %+v, %v", page, err)
	}
}
