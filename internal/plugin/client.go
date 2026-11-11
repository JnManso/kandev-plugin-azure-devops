package plugin

import (
	"context"
	"time"

	"kandev-plugin-azure-devops/internal/azure"
)

// Client is the Azure DevOps surface the plugin runtime uses.
type Client interface {
	TestAuth(ctx context.Context) (*azure.TestConnectionResult, error)
	ListProjects(ctx context.Context) ([]azure.Project, error)
	ListAllRepositories(ctx context.Context) ([]azure.Repository, error)
	GetRepository(ctx context.Context, project, repositoryIDOrName string) (*azure.Repository, error)
	ListBranches(ctx context.Context, projectID, repositoryID string) ([]azure.Branch, error)
	ListRepositories(ctx context.Context, projectID string) ([]azure.Repository, error)

	QueryWIQL(ctx context.Context, projectID, wiql string, top int) (*azure.WorkItemSearchResult, error)
	GetWorkItemDetail(ctx context.Context, projectID string, id int) (*azure.WorkItemDetail, error)
	ListWorkItemComments(ctx context.Context, projectID string, id int, continuationToken string) (*azure.WorkItemCommentPage, error)
	UpdateWorkItem(ctx context.Context, projectID string, id int, request azure.WorkItemAssignmentRequest) (*azure.WorkItem, error)
	GetCurrentIdentity(ctx context.Context) (*azure.Identity, error)

	ListTeams(ctx context.Context, projectID string) ([]azure.Team, error)
	ListMyTeamIDs(ctx context.Context, projectID string) ([]string, error)
	GetTeamSettings(ctx context.Context, projectID, teamID string) (*azure.TeamSettings, error)
	GetSprintBoard(ctx context.Context, projectID, teamID, iterationID string, today time.Time) (*azure.SprintBoard, error)
	ListChildren(ctx context.Context, projectID string, parentIDs []int) (*azure.BoardChildren, error)
	ListBoards(ctx context.Context, projectID, teamID string) ([]azure.BoardReference, error)
	GetBoardSnapshot(ctx context.Context, projectID, teamID, boardID string) (*azure.BoardSnapshot, error)
	UpdateBoardWorkItem(ctx context.Context, projectID, teamID, boardID string, id int, request azure.BoardWorkItemUpdateRequest) (*azure.BoardWorkItem, error)

	ListPullRequests(ctx context.Context, filter azure.PullRequestFilter) (*azure.PullRequestPage, error)
	ListPullRequestPeople(ctx context.Context, projectID, repositoryID string) (*azure.PullRequestPeople, error)
	GetPullRequest(ctx context.Context, projectID, repositoryID string, id int) (*azure.PullRequest, error)
	ListReviewers(ctx context.Context, projectID, repositoryID string, id int) ([]azure.Reviewer, error)
	ListThreads(ctx context.Context, projectID, repositoryID string, id int) ([]azure.Thread, error)
	ListLinkedWorkItems(ctx context.Context, projectID, repositoryID string, id int) ([]azure.WorkItemRef, error)
	ListPolicyEvaluations(ctx context.Context, projectID string, id int) ([]azure.PolicyEvaluation, error)
	CreatePullRequest(ctx context.Context, projectID, repositoryID string, request azure.CreatePullRequestRequest) (*azure.PullRequest, error)
}

var _ Client = (*azure.RESTClient)(nil)

// ClientFactory builds a client for one organization and credential.
type ClientFactory func(organizationURL string, auth azure.Authorizer) Client

// DefaultClientFactory builds the REST client.
func DefaultClientFactory(organizationURL string, auth azure.Authorizer) Client {
	return azure.NewRESTClient(organizationURL, auth, nil)
}
