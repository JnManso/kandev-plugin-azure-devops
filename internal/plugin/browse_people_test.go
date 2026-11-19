package plugin

import "testing"

func TestPullRequestPeopleListsCreatorsAndReviewers(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	if status, _ := h.action("ws-1", "pull_requests.people", map[string]any{"repositoryId": "azure-repo-1"}); status != 400 {
		t.Fatalf("missing project status = %d, want 400", status)
	}
	if status, body := h.action("ws-1", "pull_requests.people", map[string]any{"projectId": "p1"}); status != 200 || len(body["creators"].([]any)) != 1 {
		t.Fatalf("project-wide status = %d body = %v", status, body)
	}
	status, body := h.action("ws-1", "pull_requests.people", map[string]any{"projectId": "p1", "repositoryId": "azure-repo-1"})
	creators, _ := body["creators"].([]any)
	reviewers, _ := body["reviewers"].([]any)
	if status != 200 || len(creators) != 1 || field(creators[0], "displayName") != "Ada Reviewer" ||
		len(reviewers) != 1 || field(reviewers[0], "id") != "user-2" {
		t.Fatalf("pull_requests.people status = %d body = %v", status, body)
	}
}
