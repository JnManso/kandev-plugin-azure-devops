package plugin

import (
	"context"
	"fmt"
	"strconv"

	"kandev-plugin-azure-devops/internal/azure"
)

// browseCalls records mutations the fake received.
type browseCalls struct {
	boardUpdates    []azure.BoardWorkItemUpdateRequest
	workItemUpdates []azure.WorkItemAssignmentRequest
	pullRequestSkip []int
	pullRequestUser []string
}

func boardItem(id int, title, column string) azure.BoardWorkItem {
	return azure.BoardWorkItem{
		WorkItem: azure.WorkItem{
			ID: id, Revision: 3, Title: title, State: "New", Type: "User Story",
			Description: "<p>large description</p>", Fields: map[string]any{"System.Title": title},
		},
		ColumnID: column,
	}
}

func (c *fakeClient) ListRepositories(_ context.Context, projectID string) ([]azure.Repository, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	var result []azure.Repository
	for _, repository := range c.azure.repositories {
		if repository.ProjectID == projectID {
			result = append(result, repository)
		}
	}
	return result, nil
}

func (c *fakeClient) QueryWIQL(_ context.Context, projectID, wiql string, top int) (*azure.WorkItemSearchResult, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	if wiql == "BAD" {
		return nil, &azure.APIError{StatusCode: 400, Endpoint: "/_apis/wit/wiql", Body: "bad query"}
	}
	items := []azure.WorkItem{boardItem(101, "Handle token rotation", "").WorkItem, boardItem(102, "Plan the next release", "").WorkItem}
	if top < len(items) {
		items = items[:top]
	}
	return &azure.WorkItemSearchResult{Items: items, Count: len(items)}, nil
}

func (c *fakeClient) QueryWorkItemIDs(ctx context.Context, projectID, wiql string, top int) ([]int, error) {
	result, err := c.QueryWIQL(ctx, projectID, wiql, top)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(result.Items))
	for _, item := range result.Items {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

func (c *fakeClient) GetWorkItems(_ context.Context, _ string, ids []int) ([]azure.WorkItem, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	items := make([]azure.WorkItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, boardItem(id, "Work item "+strconv.Itoa(id), "").WorkItem)
	}
	return items, nil
}

func (c *fakeClient) GetWorkItemDetail(_ context.Context, _ string, id int) (*azure.WorkItemDetail, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	if id != 101 {
		return nil, &azure.APIError{StatusCode: 404, Endpoint: "/_apis/wit/workitems", Body: "missing"}
	}
	item := boardItem(101, "Handle token rotation", "").WorkItem
	item.Description = "<p>Rotate the credentials safely.</p>"
	return &azure.WorkItemDetail{WorkItem: item, PlanningFields: []azure.PlanningField{}}, nil
}

func (c *fakeClient) ListWorkItemComments(_ context.Context, _ string, _ int, token string) (*azure.WorkItemCommentPage, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return &azure.WorkItemCommentPage{Comments: []azure.WorkItemComment{{ID: 1, Content: "Discussion from Azure"}}, ContinuationToken: token + "next"}, nil
}

func (c *fakeClient) UpdateWorkItem(_ context.Context, _ string, id int, request azure.WorkItemAssignmentRequest) (*azure.WorkItem, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	c.azure.mu.Lock()
	c.azure.browse.workItemUpdates = append(c.azure.browse.workItemUpdates, request)
	c.azure.mu.Unlock()
	item := boardItem(id, "Handle token rotation", "").WorkItem
	item.Revision = request.Revision + 1
	item.AssignedTo = "ada@example.com"
	return &item, nil
}

func (c *fakeClient) GetCurrentIdentity(context.Context) (*azure.Identity, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return &azure.Identity{ID: "user-1", DisplayName: "Ada", UniqueName: "ada@example.com"}, nil
}

func (c *fakeClient) ListTeams(context.Context, string) ([]azure.Team, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return []azure.Team{{ID: "team-1", Name: "Platform", ProjectID: "p1"}}, nil
}

func (c *fakeClient) ListBoards(context.Context, string, string) ([]azure.BoardReference, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return []azure.BoardReference{{ID: "board-1", Name: "Stories"}, {ID: "board-2", Name: "Tasks"}}, nil
}

func (c *fakeClient) GetBoardSnapshot(_ context.Context, _, _, boardID string) (*azure.BoardSnapshot, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	columns := []azure.BoardColumn{{ID: "col-new", Name: "New"}, {ID: "col-active", Name: "Active", IsSplit: true}, {ID: "col-done", Name: "Done"}}
	items := []azure.BoardWorkItem{boardItem(101, "Handle token rotation", "col-new")}
	if boardID == "board-2" {
		items = []azure.BoardWorkItem{boardItem(102, "Plan the next release", "col-new")}
	}
	return &azure.BoardSnapshot{Board: azure.Board{ID: boardID, Columns: columns}, Items: items}, nil
}

func (c *fakeClient) UpdateBoardWorkItem(_ context.Context, _, _, _ string, id int, request azure.BoardWorkItemUpdateRequest) (*azure.BoardWorkItem, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	c.azure.mu.Lock()
	c.azure.browse.boardUpdates = append(c.azure.browse.boardUpdates, request)
	c.azure.mu.Unlock()
	item := boardItem(id, "Handle token rotation", "col-new")
	if request.ColumnID != nil {
		item.ColumnID = *request.ColumnID
	}
	return &item, nil
}

func (c *fakeClient) ListPullRequests(_ context.Context, filter azure.PullRequestFilter) (*azure.PullRequestPage, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	c.azure.mu.Lock()
	if filter.SourceBranch != "" {
		defer c.azure.mu.Unlock()
		var matches []azure.PullRequest
		for _, pr := range c.azure.pullRequests {
			if pr.RepositoryID == filter.RepositoryID && pr.SourceBranch == filter.SourceBranch && pr.Status == "active" {
				matches = append(matches, pr)
			}
		}
		return &azure.PullRequestPage{Items: matches, Count: len(matches)}, nil
	}
	c.azure.browse.pullRequestSkip = append(c.azure.browse.pullRequestSkip, filter.Skip)
	c.azure.browse.pullRequestUser = append(c.azure.browse.pullRequestUser, filter.CreatorID+"|"+filter.ReviewerID)
	c.azure.mu.Unlock()
	const total = 3
	var items []azure.PullRequest
	for id := filter.Skip + 1; id <= total && len(items) < filter.Top; id++ {
		items = append(items, azure.PullRequest{ID: id, Title: fmt.Sprintf("Pull request %d", id)})
	}
	return &azure.PullRequestPage{Items: items, Count: len(items), Skip: filter.Skip, Top: filter.Top}, nil
}

func (c *fakeClient) GetPullRequest(_ context.Context, _, _ string, id int) (*azure.PullRequest, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	c.azure.mu.Lock()
	defer c.azure.mu.Unlock()
	if pr, found := c.azure.pullRequests[id]; found {
		return &pr, nil
	}
	if c.azure.pullRequests != nil {
		return nil, &azure.APIError{StatusCode: 404, Endpoint: "/pullrequests", Body: "not found"}
	}
	return &azure.PullRequest{ID: id, Title: "Rotate integration credentials"}, nil
}

func (c *fakeClient) CreatePullRequest(_ context.Context, projectID, repositoryID string, request azure.CreatePullRequestRequest) (*azure.PullRequest, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	c.azure.mu.Lock()
	defer c.azure.mu.Unlock()
	c.azure.createdPRs = append(c.azure.createdPRs, request)
	if c.azure.pullRequests == nil {
		c.azure.pullRequests = map[int]azure.PullRequest{}
	}
	id := 500 + len(c.azure.createdPRs)
	pr := azure.PullRequest{
		ID: id, Title: request.Title, Status: "active", IsDraft: request.IsDraft, SourceBranch: request.SourceBranch,
		TargetBranch: request.TargetBranch, ProjectID: projectID, ProjectName: "Platform Team", RepositoryID: repositoryID,
		RepositoryName: "api", WebURL: fmt.Sprintf("https://dev.azure.com/acme/Platform%%20Team/_git/api/pullrequest/%d", id),
	}
	c.azure.pullRequests[id] = pr
	return &pr, nil
}

func (c *fakeClient) ListReviewers(context.Context, string, string, int) ([]azure.Reviewer, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return []azure.Reviewer{{Identity: azure.Identity{DisplayName: "Grace Reviewer"}, Vote: 10}}, nil
}

func (c *fakeClient) ListThreads(context.Context, string, string, int) ([]azure.Thread, error) {
	return nil, c.authorize()
}

func (c *fakeClient) ListLinkedWorkItems(context.Context, string, string, int) ([]azure.WorkItemRef, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return []azure.WorkItemRef{{ID: 101}}, nil
}

func (c *fakeClient) ListPolicyEvaluations(context.Context, string, int) ([]azure.PolicyEvaluation, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return []azure.PolicyEvaluation{{ID: "build", Name: "Build", Status: "running", IsBlocking: true}}, nil
}
