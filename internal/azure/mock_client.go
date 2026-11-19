package azure

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// MockState is the deterministic E2E data exposed by MockClient.
type MockState struct {
	Authenticated    bool                            `json:"authenticated"`
	User             TestConnectionResult            `json:"user"`
	Projects         []Project                       `json:"projects"`
	Teams            []Team                          `json:"teams"`
	Boards           []BoardReference                `json:"boards"`
	BoardSnapshots   map[string]BoardSnapshot        `json:"boardSnapshots"`
	Repositories     []Repository                    `json:"repositories"`
	WorkItems        []WorkItem                      `json:"workItems"`
	WorkItemComments map[int][]WorkItemComment       `json:"workItemComments"`
	PullRequests     []PullRequest                   `json:"pullRequests"`
	Feedback         map[int]MockPullRequestFeedback `json:"feedback"`
	MyTeamIDs        []string                        `json:"myTeamIds"`
	TeamSettings     map[string]TeamSettings         `json:"teamSettings"`
	Sprints          map[string]MockSprint           `json:"sprints"`
	SprintItems      []WorkItem                      `json:"sprintItems"`
	ChildItems       []WorkItem                      `json:"childItems"`
	ChildLinks       []SprintRelation                `json:"childLinks"`
	// StateCategories maps a work-item type and state to its category.
	StateCategories map[string]map[string]string `json:"stateCategories"`
}

// MockClient implements Client with in-memory state for browser tests.
type MockClient struct {
	mu    sync.RWMutex
	state MockState
}

func NewMockClient() *MockClient {
	client := &MockClient{}
	client.Seed(MockState{Authenticated: true, User: TestConnectionResult{OK: true, ID: "mock-user", DisplayName: "Mock User"}})
	return client
}

func (c *MockClient) Seed(state MockState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state.Feedback == nil {
		state.Feedback = make(map[int]MockPullRequestFeedback)
	}
	if state.BoardSnapshots == nil {
		state.BoardSnapshots = make(map[string]BoardSnapshot)
	}
	if state.WorkItemComments == nil {
		state.WorkItemComments = make(map[int][]WorkItemComment)
	}
	c.state = state
}

func (c *MockClient) snapshot() MockState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *MockClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	state := c.snapshot()
	if !state.Authenticated {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}
	result := state.User
	result.OK = true
	return &result, nil
}

func (c *MockClient) ListProjects(context.Context) ([]Project, error) {
	return c.snapshot().Projects, nil
}

func (c *MockClient) ListTeams(_ context.Context, projectID string) ([]Team, error) {
	items := make([]Team, 0)
	for _, team := range c.snapshot().Teams {
		if projectID == "" || team.ProjectID == "" || team.ProjectID == projectID {
			items = append(items, team)
		}
	}
	return items, nil
}

func (c *MockClient) ListBoards(context.Context, string, string) ([]BoardReference, error) {
	return c.snapshot().Boards, nil
}

func (c *MockClient) GetBoardSnapshot(_ context.Context, _, _, boardID string) (*BoardSnapshot, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snapshot, ok := c.state.BoardSnapshots[boardID]
	if !ok {
		return nil, fmt.Errorf("azure devops mock board %q not found", boardID)
	}
	result := cloneMockBoardSnapshot(snapshot)
	return &result, nil
}

func cloneMockBoardSnapshot(snapshot BoardSnapshot) BoardSnapshot {
	clone := snapshot
	clone.Board.Columns = append([]BoardColumn(nil), snapshot.Board.Columns...)
	clone.Board.Rows = append([]BoardRow(nil), snapshot.Board.Rows...)
	clone.Items = make([]BoardWorkItem, len(snapshot.Items))
	for index, item := range snapshot.Items {
		clone.Items[index] = cloneMockBoardWorkItem(item)
	}
	return clone
}

func cloneMockBoardWorkItem(item BoardWorkItem) BoardWorkItem {
	clone := item
	clone.Tags = append([]string(nil), item.Tags...)
	if item.Fields != nil {
		clone.Fields = make(map[string]any, len(item.Fields))
		for key, value := range item.Fields {
			clone.Fields[key] = cloneMockFieldValue(value)
		}
	}
	return clone
}

func cloneMockFieldValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, nested := range typed {
			clone[key] = cloneMockFieldValue(nested)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for index, nested := range typed {
			clone[index] = cloneMockFieldValue(nested)
		}
		return clone
	case map[string]string:
		clone := make(map[string]string, len(typed))
		for key, nested := range typed {
			clone[key] = nested
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func (c *MockClient) UpdateBoardWorkItem(_ context.Context, _, _, boardID string, id int, request BoardWorkItemUpdateRequest) (*BoardWorkItem, error) {
	if err := validateBoardWorkItemUpdate(request); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if request.AssigneeAction != nil && !request.hasResolvedAssignee {
		request.hasResolvedAssignee = true
		if *request.AssigneeAction == AssignCurrentUserAction {
			request.resolvedAssignee = strings.TrimSpace(c.state.User.Email)
			if request.resolvedAssignee == "" {
				request.resolvedAssignee = c.state.User.DisplayName
			}
		}
	}
	snapshot, ok := c.state.BoardSnapshots[boardID]
	if !ok {
		return nil, fmt.Errorf("azure devops mock board %q not found", boardID)
	}
	for index := range snapshot.Items {
		item := &snapshot.Items[index]
		if item.ID != id {
			continue
		}
		if item.Revision != request.Revision {
			return nil, &APIError{StatusCode: 409, Endpoint: "mock board update", Body: "revision conflict"}
		}
		if err := applyMockBoardWorkItemUpdate(item, snapshot.Board, request); err != nil {
			return nil, err
		}
		item.Revision++
		snapshot.Items[index] = *item
		c.state.BoardSnapshots[boardID] = snapshot
		for workItemIndex := range c.state.WorkItems {
			if c.state.WorkItems[workItemIndex].ID == id {
				c.state.WorkItems[workItemIndex] = item.WorkItem
				break
			}
		}
		copy := cloneMockBoardWorkItem(*item)
		return &copy, nil
	}
	return nil, mockNotFound("work item", id)
}

func (c *MockClient) UpdateWorkItem(_ context.Context, _ string, id int, request WorkItemAssignmentRequest) (*WorkItem, error) {
	if err := validateWorkItemAssignment(request); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if request.AssigneeAction != nil && *request.AssigneeAction == AssignCurrentUserAction && !request.hasResolvedAssignee {
		request.hasResolvedAssignee = true
		request.resolvedAssignee = strings.TrimSpace(c.state.User.Email)
		if request.resolvedAssignee == "" {
			request.resolvedAssignee = c.state.User.DisplayName
		}
	}
	for index := range c.state.WorkItems {
		item := &c.state.WorkItems[index]
		if item.ID != id {
			continue
		}
		if item.Revision != request.Revision {
			return nil, &APIError{StatusCode: 409, Endpoint: "mock work item update", Body: "revision conflict"}
		}
		if request.hasResolvedAssignee {
			item.AssignedTo = request.resolvedAssignee
		}
		item.Revision++
		for boardID, snapshot := range c.state.BoardSnapshots {
			for itemIndex := range snapshot.Items {
				if snapshot.Items[itemIndex].ID == id {
					snapshot.Items[itemIndex].AssignedTo = item.AssignedTo
					snapshot.Items[itemIndex].Revision = item.Revision
				}
			}
			c.state.BoardSnapshots[boardID] = snapshot
		}
		copy := *item
		copy.Tags = append([]string(nil), item.Tags...)
		return &copy, nil
	}
	return nil, mockNotFound("work item", id)
}

func applyMockBoardWorkItemUpdate(item *BoardWorkItem, board Board, request BoardWorkItemUpdateRequest) error {
	if request.hasResolvedAssignee {
		item.AssignedTo = request.resolvedAssignee
	}
	if request.ColumnID != nil {
		column, ok := boardColumnByID(board.Columns, *request.ColumnID)
		if !ok {
			return fmt.Errorf("%w: unknown board column", ErrInvalidConfig)
		}
		item.ColumnID = column.ID
		mockBoardFields(item)[board.Fields.ColumnField.ReferenceName] = column.Name
	}
	if request.ColumnDone != nil {
		item.ColumnDone = *request.ColumnDone
		mockBoardFields(item)[board.Fields.DoneField.ReferenceName] = *request.ColumnDone
	}
	return nil
}

func boardColumnByID(columns []BoardColumn, id string) (BoardColumn, bool) {
	for _, column := range columns {
		if column.ID == id {
			return column, true
		}
	}
	return BoardColumn{}, false
}

func mockBoardFields(item *BoardWorkItem) map[string]any {
	if item.Fields == nil {
		item.Fields = make(map[string]any)
	}
	return item.Fields
}

func (c *MockClient) ListRepositories(_ context.Context, projectID string) ([]Repository, error) {
	state := c.snapshot()
	items := make([]Repository, 0)
	for _, repository := range state.Repositories {
		if projectID == "" || repository.ProjectID == projectID {
			items = append(items, repository)
		}
	}
	return items, nil
}

func (c *MockClient) ListBranches(_ context.Context, projectID, repositoryID string) ([]Branch, error) {
	state := c.snapshot()
	for _, repo := range state.Repositories {
		if repo.ProjectID == projectID && repo.ID == repositoryID {
			return []Branch{{Name: strings.TrimPrefix(repo.DefaultBranch, "refs/heads/")}}, nil
		}
	}
	return []Branch{}, nil
}

// BranchExists reports the repository's default branch, the only mock branch.
func (c *MockClient) BranchExists(ctx context.Context, projectID, repositoryID, branch string) (bool, error) {
	branches, err := c.ListBranches(ctx, projectID, repositoryID)
	for _, candidate := range branches {
		if candidate.Name == branch {
			return true, err
		}
	}
	return false, err
}

var (
	mockWIQLID        = regexp.MustCompile(`\[System\.Id\] = (\d+)`)
	mockWIQLTitle     = regexp.MustCompile(`\[System\.Title\] CONTAINS '((?:[^']|'')*)'`)
	mockWIQLArea      = regexp.MustCompile(`\[System\.AreaPath\] (UNDER|=) '((?:[^']|'')*)'`)
	mockWIQLIteration = regexp.MustCompile(`\[System\.IterationPath\] = '((?:[^']|'')*)'`)
)

// mockWIQLMatches honours the work item number, title, team area, and sprint
// conditions the browse page builds; area conditions are ORed, as the page
// joins them. Any other WIQL matches every work item.
func mockWIQLMatches(wiql string, item WorkItem) bool {
	if match := mockWIQLID.FindStringSubmatch(wiql); match != nil {
		return strconv.Itoa(item.ID) == match[1]
	}
	if !mockAreaMatches(wiql, item.AreaPath) {
		return false
	}
	iteration, _ := item.Fields["System.IterationPath"].(string)
	for _, match := range mockWIQLIteration.FindAllStringSubmatch(wiql, -1) {
		if !strings.EqualFold(iteration, mockWIQLValue(match[1])) {
			return false
		}
	}
	title := strings.ToLower(item.Title)
	for _, match := range mockWIQLTitle.FindAllStringSubmatch(wiql, -1) {
		if !strings.Contains(title, strings.ToLower(strings.ReplaceAll(match[1], "''", "'"))) {
			return false
		}
	}
	return true
}

func (c *MockClient) QueryWIQL(_ context.Context, projectID, wiql string, top int) (*WorkItemSearchResult, error) {
	state := c.snapshot()
	items := make([]WorkItem, 0)
	for _, item := range state.WorkItems {
		if !mockWIQLMatches(wiql, item) {
			continue
		}
		if projectID == "" || item.Project == "" || projectMatches(state, item.Project, projectID) {
			items = append(items, item)
		}
		if top > 0 && len(items) >= top {
			break
		}
	}
	return &WorkItemSearchResult{Items: items, Count: len(items)}, nil
}

// QueryWorkItemIDs returns the IDs QueryWIQL would return.
func (c *MockClient) QueryWorkItemIDs(ctx context.Context, projectID, wiql string, top int) ([]int, error) {
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

// GetWorkItems returns the known work items among ids, in the given order.
func (c *MockClient) GetWorkItems(_ context.Context, _ string, ids []int) ([]WorkItem, error) {
	byID := map[int]WorkItem{}
	for _, item := range mockWorkItems(c.snapshot()) {
		byID[item.ID] = item
	}
	items := make([]WorkItem, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (c *MockClient) GetWorkItem(_ context.Context, _ string, id int) (*WorkItem, error) {
	state := c.snapshot()
	for _, item := range mockWorkItems(state) {
		if item.ID == id {
			copy := item
			return &copy, nil
		}
	}
	return nil, mockNotFound("work item", id)
}

func (c *MockClient) GetWorkItemDetail(ctx context.Context, projectID string, id int) (*WorkItemDetail, error) {
	item, err := c.GetWorkItem(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	detail := &WorkItemDetail{WorkItem: *item, PlanningFields: planningFields(item.Fields)}
	detail.Description = sanitizeDescriptionHTML(detail.Description)
	return detail, nil
}

func (c *MockClient) ListWorkItemComments(_ context.Context, _ string, id int, _ string) (*WorkItemCommentPage, error) {
	comments := append([]WorkItemComment(nil), c.snapshot().WorkItemComments[id]...)
	return &WorkItemCommentPage{Comments: comments}, nil
}

func (c *MockClient) GetCurrentIdentity(context.Context) (*Identity, error) {
	state := c.snapshot()
	if !state.Authenticated {
		return nil, errors.New("azure devops mock is not authenticated")
	}
	return &Identity{ID: state.User.ID, DisplayName: state.User.DisplayName, UniqueName: state.User.Email}, nil
}

func (c *MockClient) ListPullRequests(_ context.Context, filter PullRequestFilter) (*PullRequestPage, error) {
	items := make([]PullRequest, 0)
	for _, pr := range c.snapshot().PullRequests {
		if (filter.ProjectID == "" || projectMatches(c.snapshot(), pr.ProjectID, filter.ProjectID)) &&
			(filter.RepositoryID == "" || pr.RepositoryID == filter.RepositoryID) &&
			mockStatusMatches(filter.Status, pr.Status) &&
			(filter.CreatorID == "" || strings.EqualFold(pr.Author.ID, filter.CreatorID)) &&
			(filter.ReviewerID == "" || mockReviewedBy(c.snapshot(), pr.ID, filter.ReviewerID)) {
			items = append(items, pr)
		}
	}
	top := filter.Top
	if top <= 0 || top > 100 {
		top = defaultPRPageSize
	}
	if filter.Skip >= len(items) {
		return &PullRequestPage{Skip: filter.Skip, Top: top}, nil
	}
	items = items[filter.Skip:]
	if len(items) > top {
		items = items[:top]
	}
	return &PullRequestPage{Items: items, Count: len(items), Skip: filter.Skip, Top: top}, nil
}

// mockStatusMatches follows Azure DevOps: an empty status means active and
// "all" matches every status.
func mockStatusMatches(filter, status string) bool {
	switch filter {
	case "all":
		return true
	case "":
		return status == "active"
	default:
		return status == filter
	}
}

func (c *MockClient) GetPullRequest(_ context.Context, _, _ string, id int) (*PullRequest, error) {
	for _, pr := range c.snapshot().PullRequests {
		if pr.ID == id {
			copy := pr
			return &copy, nil
		}
	}
	feedback, ok := c.Feedback(id)
	if ok && feedback.PullRequest != nil {
		return feedback.PullRequest, nil
	}
	return nil, mockNotFound("pull request", id)
}

// CreatePullRequest appends an active pull request to the mock state.
func (c *MockClient) CreatePullRequest(_ context.Context, projectID, repositoryID string, request CreatePullRequestRequest) (*PullRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var repository *Repository
	for index := range c.state.Repositories {
		candidate := &c.state.Repositories[index]
		if (candidate.ID == repositoryID || strings.EqualFold(candidate.Name, repositoryID)) &&
			(projectID == "" || candidate.ProjectID == projectID || strings.EqualFold(candidate.ProjectName, projectID)) {
			repository = candidate
			break
		}
	}
	if repository == nil {
		return nil, mockNotFound("repository", 0)
	}
	nextID := 1
	for _, existing := range c.state.PullRequests {
		if existing.ID >= nextID {
			nextID = existing.ID + 1
		}
	}
	pr := PullRequest{
		ID: nextID, Title: request.Title, Description: request.Description, Status: "active", IsDraft: request.IsDraft,
		SourceBranch: trimBranchRef(request.SourceBranch), TargetBranch: trimBranchRef(request.TargetBranch),
		Author:    Identity{ID: c.state.User.ID, DisplayName: c.state.User.DisplayName},
		ProjectID: repository.ProjectID, ProjectName: repository.ProjectName,
		RepositoryID: repository.ID, RepositoryName: repository.Name,
		WebURL: fmt.Sprintf("%s/pullrequest/%d", strings.TrimRight(repository.WebURL, "/"), nextID),
	}
	c.state.PullRequests = append(c.state.PullRequests, pr)
	return &pr, nil
}

func (c *MockClient) ListReviewers(_ context.Context, _, _ string, id int) ([]Reviewer, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []Reviewer{}, nil
	}
	return feedback.Reviewers, nil
}

func (c *MockClient) ListThreads(_ context.Context, _, _ string, id int) ([]Thread, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []Thread{}, nil
	}
	return feedback.Threads, nil
}

func (c *MockClient) ListLinkedWorkItems(_ context.Context, _, _ string, id int) ([]WorkItemRef, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []WorkItemRef{}, nil
	}
	return feedback.LinkedWorkItems, nil
}

func (c *MockClient) ListPolicyEvaluations(_ context.Context, _ string, id int) ([]PolicyEvaluation, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []PolicyEvaluation{}, nil
	}
	return feedback.Policies, nil
}

func (c *MockClient) Feedback(id int) (MockPullRequestFeedback, bool) {
	state := c.snapshot()
	feedback, ok := state.Feedback[id]
	return feedback, ok
}

func mockNotFound(kind string, id int) error {
	return &APIError{StatusCode: 404, Endpoint: "mock", Body: fmt.Sprintf("%s %d not found", kind, id)}
}

// MockPullRequestFeedback is the seeded review detail of one pull request.
type MockPullRequestFeedback struct {
	PullRequest     *PullRequest       `json:"pullRequest"`
	Reviewers       []Reviewer         `json:"reviewers"`
	Threads         []Thread           `json:"threads"`
	LinkedWorkItems []WorkItemRef      `json:"linkedWorkItems"`
	Policies        []PolicyEvaluation `json:"policies"`
}

// ListAllRepositories lists every seeded repository.
func (c *MockClient) ListAllRepositories(context.Context) ([]Repository, error) {
	state := c.snapshot()
	return append([]Repository(nil), state.Repositories...), nil
}

// GetRepository finds a seeded repository by ID, or by project and name.
func (c *MockClient) GetRepository(_ context.Context, project, repositoryIDOrName string) (*Repository, error) {
	for _, repository := range c.snapshot().Repositories {
		if repository.ID == repositoryIDOrName ||
			(strings.EqualFold(repository.Name, repositoryIDOrName) &&
				(project == "" || strings.EqualFold(repository.ProjectName, project) || repository.ProjectID == project)) {
			found := repository
			return &found, nil
		}
	}
	return nil, &APIError{StatusCode: 404, Endpoint: "mock repository", Body: "repository not found"}
}

//go:embed mock_seed.json
var mockSeed []byte

// NewSeededMockClient returns a mock with the demo organization used by the
// plugin's mock mode and packaged E2E runs.
func NewSeededMockClient() (*MockClient, error) {
	var state MockState
	if err := json.Unmarshal(mockSeed, &state); err != nil {
		return nil, fmt.Errorf("decode mock seed: %w", err)
	}
	client := NewMockClient()
	client.Seed(state)
	return client, nil
}

// projectMatches accepts a project ID or name, as the Azure DevOps REST API does.
func projectMatches(state MockState, projectID, requested string) bool {
	if strings.EqualFold(projectID, requested) {
		return true
	}
	for _, project := range state.Projects {
		if project.ID == projectID && strings.EqualFold(project.Name, requested) {
			return true
		}
	}
	return false
}
