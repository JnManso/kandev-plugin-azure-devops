package azure

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// pullRequestPeopleScan bounds how many of a repository's latest pull
// requests are read to list its people.
const pullRequestPeopleScan = 300

const pullRequestPeoplePage = 100

// PullRequestPeople are the people who created or reviewed a repository's
// recent pull requests, the values its creator and reviewer filters accept.
type PullRequestPeople struct {
	Creators  []Identity `json:"creators"`
	Reviewers []Identity `json:"reviewers"`
}

// DistinctPeople drops identities without an ID and repeated IDs (ignoring
// case), and sorts the rest by display name.
func DistinctPeople(identities []Identity) []Identity {
	seen := map[string]bool{}
	people := []Identity{}
	for _, identity := range identities {
		key := strings.ToLower(strings.TrimSpace(identity.ID))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		people = append(people, identity)
	}
	sort.SliceStable(people, func(i, j int) bool {
		left, right := strings.ToLower(people[i].DisplayName), strings.ToLower(people[j].DisplayName)
		if left != right {
			return left < right
		}
		return people[i].ID < people[j].ID
	})
	return people
}

// ListPullRequestPeople reads the creators and reviewers of the latest pull
// requests of every status, in the repository, or in the whole project when
// repositoryID is empty.
func (c *RESTClient) ListPullRequestPeople(ctx context.Context, projectID, repositoryID string) (*PullRequestPeople, error) {
	var creators, reviewers []Identity
	for skip := 0; skip < pullRequestPeopleScan; skip += pullRequestPeoplePage {
		values := url.Values{
			"api-version":           {restAPIVersion},
			"searchCriteria.status": {"all"},
			"$top":                  {strconv.Itoa(pullRequestPeoplePage)},
			"$skip":                 {strconv.Itoa(skip)},
		}
		endpoint := fmt.Sprintf("/%s/_apis/git/pullrequests?%s", pathPart(projectID), values.Encode())
		if strings.TrimSpace(repositoryID) != "" {
			endpoint = fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests?%s", pathPart(projectID), pathPart(repositoryID), values.Encode())
		}
		var response struct {
			Value []struct {
				CreatedBy Identity   `json:"createdBy"`
				Reviewers []Identity `json:"reviewers"`
			} `json:"value"`
		}
		if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
			return nil, err
		}
		for _, pr := range response.Value {
			creators = append(creators, pr.CreatedBy)
			reviewers = append(reviewers, pr.Reviewers...)
		}
		if len(response.Value) < pullRequestPeoplePage {
			break
		}
	}
	return &PullRequestPeople{Creators: DistinctPeople(creators), Reviewers: DistinctPeople(reviewers)}, nil
}
