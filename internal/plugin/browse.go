package plugin

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"kandev-plugin-azure-devops/internal/azure"
)

const (
	// maxWorkItemResults bounds a search response below the 1 MiB action limit.
	maxWorkItemResults     = 200
	defaultPullRequestPage = 50
	maxPullRequestPage     = 100
	currentUserFilter      = "@me"
)

var errProjectRequired = errors.New("a project is required")

// Browse serves the read and mutation actions behind the Azure DevOps page.
type Browse struct{ connections *Connections }

// NewBrowse returns the browse action handler.
func NewBrowse(connections *Connections) *Browse { return &Browse{connections: connections} }

type browseInput struct {
	ProjectID         string  `json:"projectId"`
	TeamID            string  `json:"teamId"`
	BoardID           string  `json:"boardId"`
	IterationID       string  `json:"iterationId"`
	RepositoryID      string  `json:"repositoryId"`
	ID                int     `json:"id"`
	IDs               []int   `json:"ids"`
	Revision          int     `json:"revision"`
	WIQL              string  `json:"wiql"`
	Top               int     `json:"top"`
	ContinuationToken string  `json:"continuationToken"`
	Status            string  `json:"status"`
	Creator           string  `json:"creator"`
	Reviewer          string  `json:"reviewer"`
	Cursor            string  `json:"cursor"`
	AssigneeAction    *string `json:"assigneeAction"`
	ColumnID          *string `json:"columnId"`
	ColumnDone        *bool   `json:"columnDone"`
}

func (in browseInput) requireProject() error {
	if strings.TrimSpace(in.ProjectID) == "" {
		return invalidArgument(errProjectRequired)
	}
	return nil
}

func (in browseInput) requireWorkItem() error {
	if strings.TrimSpace(in.ProjectID) == "" || in.ID <= 0 {
		return invalidArgument(errors.New("a project and a positive work item id are required"))
	}
	return nil
}

func (in browseInput) requirePullRequest() error {
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.RepositoryID) == "" || in.ID <= 0 {
		return invalidArgument(errors.New("a project, a repository, and a positive pull request id are required"))
	}
	return nil
}

// Handle serves one browse action. found is false for other action keys.
func (b *Browse) Handle(ctx context.Context, workspaceID, key string, body []byte) (any, bool, error) {
	handler, found := browseHandlers[key]
	if !found {
		return nil, false, nil
	}
	var input browseInput
	if err := decodeBody(body, &input); err != nil {
		return nil, true, err
	}
	client, _, err := b.connections.Client(ctx, workspaceID)
	if err != nil {
		return nil, true, err
	}
	result, err := handler(ctx, client, input)
	if err != nil {
		return nil, true, browseError(err)
	}
	return result, true, nil
}

type browseHandler func(context.Context, Client, browseInput) (any, error)

var browseHandlers = map[string]browseHandler{
	"projects.repositories":  listProjectRepositories,
	"teams.list":             listTeams,
	"teams.settings":         getTeamSettings,
	"sprints.board":          getSprintBoard,
	"boards.list":            listBoards,
	"boards.get":             getBoard,
	"boards.children":        listBoardChildren,
	"boards.update_item":     updateBoardItem,
	"work_items.search":      searchWorkItems,
	"work_items.get":         getWorkItem,
	"work_items.comments":    listWorkItemComments,
	"work_items.update":      updateWorkItem,
	"pull_requests.list":     listPullRequests,
	"pull_requests.people":   listPullRequestPeople,
	"pull_requests.get":      getPullRequest,
	"pull_requests.feedback": getPullRequestFeedback,
	"identity.get":           getIdentity,
}

func listProjectRepositories(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireProject(); err != nil {
		return nil, err
	}
	repositories, err := client.ListRepositories(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	enabled := make([]azure.Repository, 0, len(repositories))
	for _, repository := range repositories {
		if !repository.IsDisabled {
			enabled = append(enabled, repository)
		}
	}
	return map[string]any{"repositories": enabled}, nil
}

func listBoards(ctx context.Context, client Client, in browseInput) (any, error) {
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.TeamID) == "" {
		return nil, invalidArgument(errors.New("a project and a team are required"))
	}
	boards, err := client.ListBoards(ctx, in.ProjectID, in.TeamID)
	return map[string]any{"boards": nonNil(boards)}, err
}

func requireBoard(in browseInput) error {
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.TeamID) == "" || strings.TrimSpace(in.BoardID) == "" {
		return invalidArgument(errors.New("a project, a team, and a board are required"))
	}
	return nil
}

func getBoard(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := requireBoard(in); err != nil {
		return nil, err
	}
	snapshot, err := client.GetBoardSnapshot(ctx, in.ProjectID, in.TeamID, in.BoardID)
	if err != nil {
		return nil, err
	}
	for index := range snapshot.Items {
		snapshot.Items[index].WorkItem = summary(snapshot.Items[index].WorkItem)
	}
	return snapshot, nil
}

func updateBoardItem(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := requireBoard(in); err != nil {
		return nil, err
	}
	if in.ID <= 0 {
		return nil, invalidArgument(errors.New("a positive work item id is required"))
	}
	request := azure.BoardWorkItemUpdateRequest{Revision: in.Revision, AssigneeAction: in.AssigneeAction, ColumnID: in.ColumnID, ColumnDone: in.ColumnDone}
	if in.AssigneeAction != nil {
		assignee, err := resolveAssignee(ctx, client, *in.AssigneeAction)
		if err != nil {
			return nil, err
		}
		request = request.WithResolvedAssignee(assignee)
	}
	item, err := client.UpdateBoardWorkItem(ctx, in.ProjectID, in.TeamID, in.BoardID, in.ID, request)
	if err != nil {
		return nil, err
	}
	item.WorkItem = summary(item.WorkItem)
	return map[string]any{"item": item}, nil
}

// resolveAssignee turns an assignee action into the name Azure expects; an
// empty name unassigns.
func resolveAssignee(ctx context.Context, client Client, action string) (string, error) {
	switch action {
	case azure.UnassignAction:
		return "", nil
	case azure.AssignCurrentUserAction:
		identity, err := client.GetCurrentIdentity(ctx)
		if err != nil {
			return "", err
		}
		assignee := strings.TrimSpace(identity.UniqueName)
		if assignee == "" {
			assignee = strings.TrimSpace(identity.DisplayName)
		}
		if assignee == "" {
			return "", invalidArgument(errors.New("the current Azure DevOps identity has no assignable name"))
		}
		return assignee, nil
	default:
		return "", invalidArgument(errors.New("an assign_current_user or unassign action is required"))
	}
}

func searchWorkItems(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireProject(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.WIQL) == "" {
		return nil, invalidArgument(errors.New("a WIQL query is required"))
	}
	top := in.Top
	if top <= 0 || top > maxWorkItemResults {
		top = maxWorkItemResults
	}
	result, err := client.QueryWIQL(ctx, in.ProjectID, in.WIQL, top)
	if err != nil {
		return nil, err
	}
	items := make([]azure.WorkItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, summary(item))
	}
	return azure.WorkItemSearchResult{Items: items, Count: len(items)}, nil
}

func getWorkItem(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireWorkItem(); err != nil {
		return nil, err
	}
	detail, err := client.GetWorkItemDetail(ctx, in.ProjectID, in.ID)
	if err != nil {
		return nil, err
	}
	detail.Fields = nil
	return map[string]any{"item": detail}, nil
}

func listWorkItemComments(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireWorkItem(); err != nil {
		return nil, err
	}
	return client.ListWorkItemComments(ctx, in.ProjectID, in.ID, in.ContinuationToken)
}

func updateWorkItem(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireWorkItem(); err != nil {
		return nil, err
	}
	if in.Revision <= 0 || in.AssigneeAction == nil {
		return nil, invalidArgument(errors.New("a revision and an assignee action are required"))
	}
	assignee, err := resolveAssignee(ctx, client, *in.AssigneeAction)
	if err != nil {
		return nil, err
	}
	request := azure.WorkItemAssignmentRequest{Revision: in.Revision, AssigneeAction: in.AssigneeAction}.WithResolvedAssignee(assignee)
	item, err := client.UpdateWorkItem(ctx, in.ProjectID, in.ID, request)
	if err != nil {
		return nil, err
	}
	return map[string]any{"item": summary(*item)}, nil
}

func listPullRequestPeople(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireProject(); err != nil {
		return nil, err
	}
	return client.ListPullRequestPeople(ctx, in.ProjectID, in.RepositoryID)
}

func listPullRequests(ctx context.Context, client Client, in browseInput) (any, error) {
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.RepositoryID) == "" {
		return nil, invalidArgument(errors.New("a project and a repository are required"))
	}
	skip, err := decodeSkipCursor(in.Cursor)
	if err != nil {
		return nil, err
	}
	top := in.Top
	if top <= 0 {
		top = defaultPullRequestPage
	}
	top = min(top, maxPullRequestPage)
	filter := azure.PullRequestFilter{
		ProjectID: in.ProjectID, RepositoryID: in.RepositoryID, Status: strings.TrimSpace(in.Status),
		CreatorID: strings.TrimSpace(in.Creator), ReviewerID: strings.TrimSpace(in.Reviewer), Skip: skip, Top: top,
	}
	if filter, err = resolveCurrentUser(ctx, client, filter); err != nil {
		return nil, err
	}
	page, err := client.ListPullRequests(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"items": nonNil(page.Items)}
	if len(page.Items) >= top {
		result["nextCursor"] = encodeSkipCursor(skip + len(page.Items))
	}
	return result, nil
}

// resolveCurrentUser replaces `@me` creator and reviewer filters with the
// authenticated Azure DevOps identity.
func resolveCurrentUser(ctx context.Context, client Client, filter azure.PullRequestFilter) (azure.PullRequestFilter, error) {
	creatorIsMe := strings.EqualFold(filter.CreatorID, currentUserFilter)
	reviewerIsMe := strings.EqualFold(filter.ReviewerID, currentUserFilter)
	if !creatorIsMe && !reviewerIsMe {
		return filter, nil
	}
	identity, err := client.TestAuth(ctx)
	if err != nil {
		return filter, err
	}
	if identity == nil || !identity.OK || strings.TrimSpace(identity.ID) == "" {
		return filter, failedPrecondition(errors.New("the authenticated Azure DevOps identity is unavailable"))
	}
	if creatorIsMe {
		filter.CreatorID = identity.ID
	}
	if reviewerIsMe {
		filter.ReviewerID = identity.ID
	}
	return filter, nil
}

func encodeSkipCursor(skip int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("skip:" + strconv.Itoa(skip)))
}

func decodeSkipCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	value, found := strings.CutPrefix(string(decoded), "skip:")
	skip, convErr := strconv.Atoi(value)
	if err != nil || !found || convErr != nil || skip < 0 {
		return 0, invalidArgument(errors.New("the page cursor is not valid"))
	}
	return skip, nil
}

func getPullRequest(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requirePullRequest(); err != nil {
		return nil, err
	}
	pullRequest, err := client.GetPullRequest(ctx, in.ProjectID, in.RepositoryID, in.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"pullRequest": pullRequest}, nil
}

// PullRequestFeedback keeps Azure-native detail with normalized summary
// states for shared task UI.
type PullRequestFeedback struct {
	PullRequest     *azure.PullRequest       `json:"pullRequest"`
	Reviewers       []azure.Reviewer         `json:"reviewers"`
	Threads         []azure.Thread           `json:"threads"`
	LinkedWorkItems []azure.WorkItemRef      `json:"linkedWorkItems"`
	Policies        []azure.PolicyEvaluation `json:"policies"`
	ReviewState     string                   `json:"reviewState"`
	PolicyState     string                   `json:"policyState"`
}

func getPullRequestFeedback(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requirePullRequest(); err != nil {
		return nil, err
	}
	return loadPullRequestFeedback(ctx, client, in.ProjectID, in.RepositoryID, in.ID)
}

func loadPullRequestFeedback(ctx context.Context, client Client, projectID, repositoryID string, id int) (*PullRequestFeedback, error) {
	pullRequest, err := client.GetPullRequest(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	reviewers, err := client.ListReviewers(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	threads, err := client.ListThreads(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	references, err := client.ListLinkedWorkItems(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	policies, err := client.ListPolicyEvaluations(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	return &PullRequestFeedback{
		PullRequest: pullRequest, Reviewers: nonNil(reviewers), Threads: nonNil(threads),
		LinkedWorkItems: nonNil(references), Policies: nonNil(policies),
		ReviewState: summarizeReviewState(reviewers), PolicyState: summarizePolicyState(policies),
	}, nil
}

func summarizeReviewState(reviewers []azure.Reviewer) string {
	approved, waiting := false, false
	for _, reviewer := range reviewers {
		switch reviewer.Vote {
		case -10:
			return "rejected"
		case -5:
			waiting = true
		case 5, 10:
			approved = true
		}
	}
	switch {
	case waiting:
		return "waiting"
	case approved:
		return "approved"
	default:
		return ""
	}
}

func summarizePolicyState(policies []azure.PolicyEvaluation) string {
	if len(policies) == 0 {
		return ""
	}
	pending := false
	for _, policy := range policies {
		switch strings.ToLower(policy.Status) {
		case "rejected", "broken":
			if policy.IsBlocking {
				return "failure"
			}
		case "queued", "running":
			pending = true
		}
	}
	if pending {
		return "pending"
	}
	return "success"
}

func getIdentity(ctx context.Context, client Client, _ browseInput) (any, error) {
	identity, err := client.GetCurrentIdentity(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"identity": identity}, nil
}

// summary drops the provider field map and description from list payloads;
// the detail action returns the description.
func summary(item azure.WorkItem) azure.WorkItem {
	item.Fields = nil
	item.Description = ""
	return item
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// browseError maps Azure DevOps API failures to action categories.
func browseError(err error) error {
	var apiErr *azure.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 400:
			return invalidArgument(errors.New("Azure DevOps rejected the request; check the query and filters"))
		case 401:
			return failedPrecondition(errors.New("Azure DevOps rejected the stored credentials; update them in settings"))
		case 403, 404:
			return notFound(errors.New("the Azure DevOps item was not found or is not accessible"))
		case 409, 412:
			return failedPrecondition(errors.New("the work item changed in Azure DevOps; refresh and try again"))
		}
	}
	if errors.Is(err, azure.ErrReauthRequired) {
		return failedPrecondition(errReauthRequired)
	}
	if errors.Is(err, azure.ErrInvalidConfig) {
		return invalidArgument(err)
	}
	return err
}
