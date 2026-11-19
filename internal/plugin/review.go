package plugin

import (
	"strings"

	"kandev-plugin-azure-devops/internal/azure"
)

// ReviewCheck is one policy evaluation in the host's task status shape.
type ReviewCheck struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// ReviewApproval summarizes reviewer votes.
type ReviewApproval struct {
	State    string `json:"state"`
	Approved int    `json:"approved"`
	Required int    `json:"required,omitempty"`
}

// ReviewTaskStatus mirrors the host's ReviewTaskStatus contract, so the UI can
// publish it without reinterpretation.
type ReviewTaskStatus struct {
	Number             int             `json:"number"`
	State              string          `json:"state"`
	PipelineState      string          `json:"pipelineState"`
	Checks             []ReviewCheck   `json:"checks"`
	Review             *ReviewApproval `json:"review,omitempty"`
	UnresolvedComments int             `json:"unresolvedComments"`
}

// PullRequestStatus is a linked pull request with its live status. Error is
// set, and TaskStatus omitted, when Azure DevOps could not be read.
type PullRequestStatus struct {
	PullRequestLink
	Status     string            `json:"status,omitempty"`
	IsDraft    bool              `json:"isDraft,omitempty"`
	TaskStatus *ReviewTaskStatus `json:"taskStatus,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// reviewTaskStatus maps Azure pull request feedback to the host contract.
func reviewTaskStatus(feedback *PullRequestFeedback) *ReviewTaskStatus {
	if feedback == nil || feedback.PullRequest == nil {
		return nil
	}
	checks := make([]ReviewCheck, 0, len(feedback.Policies))
	for _, policy := range feedback.Policies {
		label := strings.TrimSpace(policy.Name)
		if label == "" {
			label = policy.ID
		}
		checks = append(checks, ReviewCheck{ID: policy.ID, Label: label, State: policyCheckState(policy.Status), Detail: policy.Status})
	}
	return &ReviewTaskStatus{
		Number:             feedback.PullRequest.ID,
		State:              pullRequestState(*feedback.PullRequest),
		PipelineState:      pipelineState(feedback.Policies),
		Checks:             checks,
		Review:             reviewApproval(feedback.Reviewers),
		UnresolvedComments: unresolvedThreads(feedback.Threads),
	}
}

func pullRequestState(pr azure.PullRequest) string {
	switch strings.ToLower(pr.Status) {
	case "completed":
		return "merged"
	case "abandoned":
		return "closed"
	default:
		if pr.IsDraft {
			return "draft"
		}
		return "open"
	}
}

func policyCheckState(status string) string {
	switch strings.ToLower(status) {
	case "approved":
		return "success"
	case "rejected", "broken":
		return "failure"
	case "queued", "running":
		return "pending"
	default:
		return "neutral"
	}
}

// pipelineState is failure when a blocking policy fails, pending while any
// policy runs, success when policies exist and pass, and neutral otherwise.
func pipelineState(policies []azure.PolicyEvaluation) string {
	switch summarizePolicyState(policies) {
	case "failure":
		return "failure"
	case "pending":
		return "pending"
	case "success":
		return "success"
	default:
		return "neutral"
	}
}

func reviewApproval(reviewers []azure.Reviewer) *ReviewApproval {
	approval := &ReviewApproval{State: "pending"}
	requiredPending, rejected := false, false
	for _, reviewer := range reviewers {
		if reviewer.Vote >= 5 {
			approval.Approved++
		}
		if reviewer.Vote < 0 {
			rejected = true
		}
		if reviewer.IsRequired {
			approval.Required++
			if reviewer.Vote < 5 {
				requiredPending = true
			}
		}
	}
	switch {
	case rejected:
		approval.State = "changes_requested"
	case approval.Approved > 0 && !requiredPending:
		approval.State = "approved"
	}
	return approval
}

// unresolvedThreads counts active or pending threads with a human comment.
func unresolvedThreads(threads []azure.Thread) int {
	count := 0
	for _, thread := range threads {
		status := strings.ToLower(thread.Status)
		if status != "active" && status != "pending" {
			continue
		}
		for _, comment := range thread.Comments {
			if !strings.EqualFold(comment.CommentType, "system") {
				count++
				break
			}
		}
	}
	return count
}
