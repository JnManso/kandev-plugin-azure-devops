package azure

import (
	"context"
	"strings"
)

// ListPullRequestPeople lists the seeded authors and reviewers of the
// repository's pull requests, or of every pull request without a repository.
func (c *MockClient) ListPullRequestPeople(_ context.Context, _, repositoryID string) (*PullRequestPeople, error) {
	state := c.snapshot()
	var creators, reviewers []Identity
	for _, pr := range state.PullRequests {
		if repositoryID != "" && pr.RepositoryID != repositoryID {
			continue
		}
		creators = append(creators, pr.Author)
		for _, reviewer := range state.Feedback[pr.ID].Reviewers {
			reviewers = append(reviewers, reviewer.Identity)
		}
	}
	return &PullRequestPeople{Creators: DistinctPeople(creators), Reviewers: DistinctPeople(reviewers)}, nil
}

func mockReviewedBy(state MockState, pullRequestID int, reviewerID string) bool {
	for _, reviewer := range state.Feedback[pullRequestID].Reviewers {
		if strings.EqualFold(reviewer.ID, reviewerID) {
			return true
		}
	}
	return false
}
