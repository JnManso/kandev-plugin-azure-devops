package plugin

import (
	"testing"

	"kandev-plugin-azure-devops/internal/azure"
)

func TestReviewTaskStatusMapsAzureFeedback(t *testing.T) {
	feedback := &PullRequestFeedback{
		PullRequest: &azure.PullRequest{ID: 9, Status: "active", IsDraft: true},
		Reviewers: []azure.Reviewer{
			{Vote: 10, IsRequired: true}, {Vote: 0, IsRequired: true}, {Vote: 5},
		},
		Policies: []azure.PolicyEvaluation{
			{ID: "a", Name: "Build", Status: "approved", IsBlocking: true},
			{ID: "b", Status: "rejected", IsBlocking: true},
		},
		Threads: []azure.Thread{
			{Status: "active", Comments: []azure.Comment{{CommentType: "text"}}},
			{Status: "active", Comments: []azure.Comment{{CommentType: "system"}}},
			{Status: "fixed", Comments: []azure.Comment{{CommentType: "text"}}},
		},
	}
	status := reviewTaskStatus(feedback)
	if status.Number != 9 || status.State != "draft" || status.PipelineState != "failure" || status.UnresolvedComments != 1 {
		t.Fatalf("status = %+v", status)
	}
	if status.Checks[0].State != "success" || status.Checks[1].State != "failure" || status.Checks[1].Label != "b" {
		t.Fatalf("checks = %+v", status.Checks)
	}
	if status.Review.State != "pending" || status.Review.Approved != 2 || status.Review.Required != 2 {
		t.Fatalf("review = %+v", status.Review)
	}
}

func TestReviewStatesAndPullRequestStates(t *testing.T) {
	if got := reviewApproval([]azure.Reviewer{{Vote: 10}, {Vote: -5}}).State; got != "changes_requested" {
		t.Fatalf("waiting review = %s", got)
	}
	if got := reviewApproval([]azure.Reviewer{{Vote: 10, IsRequired: true}}).State; got != "approved" {
		t.Fatalf("approved review = %s", got)
	}
	if got := reviewApproval(nil).State; got != "pending" {
		t.Fatalf("no reviewers = %s", got)
	}
	for status, want := range map[string]string{"completed": "merged", "abandoned": "closed", "active": "open"} {
		if got := pullRequestState(azure.PullRequest{Status: status}); got != want {
			t.Fatalf("pullRequestState(%s) = %s", status, got)
		}
	}
	if got := pipelineState(nil); got != "neutral" {
		t.Fatalf("no policies = %s", got)
	}
}
