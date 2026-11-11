package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/azure"
)

const statusConcurrency = 4

// TaskReader is the subset of the host task API the links service uses.
type TaskReader interface {
	Get(ctx context.Context, id string) (*pluginsdk.Task, error)
}

// TaskLinks serves task links, linked pull request status, and pull request
// creation for verified task contexts.
type TaskLinks struct {
	connections  *Connections
	links        *Links
	tasks        func() TaskReader
	repositories func() RepositoryLister
}

// NewTaskLinks returns the task links service.
func NewTaskLinks(connections *Connections, links *Links, tasks func() TaskReader, repositories func() RepositoryLister) *TaskLinks {
	return &TaskLinks{connections: connections, links: links, tasks: tasks, repositories: repositories}
}

// azureRepository is a Kandev repository resolved to its Azure Repos identity.
type azureRepository struct {
	HostRepositoryID string
	ProjectID        string
	ProjectName      string
	RepositoryID     string
	RepositoryName   string
	DefaultBranch    string
}

type taskLinkInput struct {
	Reference     string `json:"reference"`
	ProjectID     string `json:"projectId"`
	RepositoryID  string `json:"repositoryId"`
	PullRequestID int    `json:"pullRequestId"`
	WorkItemID    int    `json:"workItemId"`
}

type createPullRequestInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	BaseBranch  string `json:"baseBranch"`
	Draft       bool   `json:"draft"`
}

// Handle serves task link actions. found is false for other keys.
func (t *TaskLinks) Handle(ctx context.Context, verified pluginsdk.VerifiedActionContext, key string, body []byte) (any, bool, error) {
	switch key {
	case "task_links.workspace":
		if verified.WorkspaceID == "" {
			return nil, true, invalidArgument(errors.New("a workspace is required"))
		}
		links, err := t.links.Workspace(ctx, verified.WorkspaceID)
		if err != nil {
			return nil, true, unavailable(err)
		}
		return map[string]any{
			"pullRequests": links.PullRequests, "workItems": links.WorkItems, "tasks": t.taskTitles(ctx, links),
		}, true, nil
	case "task_links.get", "pull_requests.link", "pull_requests.unlink", "work_items.link", "work_items.unlink", "pull_requests.create":
	default:
		return nil, false, nil
	}
	if verified.WorkspaceID == "" || verified.TaskID == "" {
		return nil, true, invalidArgument(errors.New("a verified task is required"))
	}
	if key == "pull_requests.create" {
		var input createPullRequestInput
		if err := decodeBody(body, &input); err != nil {
			return nil, true, err
		}
		result, err := t.createFromSession(ctx, verified, input)
		return result, true, err
	}
	var input taskLinkInput
	if err := decodeBody(body, &input); err != nil {
		return nil, true, err
	}
	result, err := t.handleTaskAction(ctx, verified.WorkspaceID, verified.TaskID, key, input)
	return result, true, err
}

func (t *TaskLinks) handleTaskAction(ctx context.Context, workspaceID, taskID, key string, input taskLinkInput) (any, error) {
	switch key {
	case "task_links.get":
		return t.status(ctx, workspaceID, taskID)
	case "pull_requests.link":
		link, err := t.linkPullRequest(ctx, workspaceID, taskID, input)
		if err != nil {
			return nil, err
		}
		return map[string]any{"pullRequest": link}, nil
	case "pull_requests.unlink":
		if input.RepositoryID == "" || input.PullRequestID <= 0 {
			return nil, invalidArgument(errors.New("a repository and pull request number are required"))
		}
		links, err := t.links.RemovePullRequest(ctx, workspaceID, taskID, input.RepositoryID, input.PullRequestID)
		return map[string]any{"pullRequests": links}, unavailableIf(err)
	case "work_items.link":
		link, err := t.linkWorkItem(ctx, workspaceID, taskID, input)
		if err != nil {
			return nil, err
		}
		return map[string]any{"workItem": link}, nil
	default: // work_items.unlink
		if input.WorkItemID <= 0 {
			return nil, invalidArgument(errors.New("a work item ID is required"))
		}
		links, err := t.links.RemoveWorkItem(ctx, workspaceID, taskID, input.WorkItemID)
		return map[string]any{"workItems": links}, unavailableIf(err)
	}
}

func unavailableIf(err error) error {
	if err == nil {
		return nil
	}
	return unavailable(err)
}

// status returns a task's links with the live status of each pull request.
func (t *TaskLinks) status(ctx context.Context, workspaceID, taskID string) (any, error) {
	pullRequests, err := t.links.PullRequests(ctx, workspaceID, taskID)
	if err != nil {
		return nil, unavailable(err)
	}
	workItems, err := t.links.WorkItems(ctx, workspaceID, taskID)
	if err != nil {
		return nil, unavailable(err)
	}
	statuses := make([]PullRequestStatus, len(pullRequests))
	for index, link := range pullRequests {
		statuses[index] = PullRequestStatus{PullRequestLink: link}
	}
	if len(pullRequests) > 0 {
		client, _, err := t.connections.Client(ctx, workspaceID)
		if err != nil {
			for index := range statuses {
				statuses[index].Error = "Azure DevOps is not connected"
			}
		} else {
			loadStatuses(ctx, client, statuses)
		}
	}
	return map[string]any{"pullRequests": statuses, "workItems": workItems}, nil
}

func loadStatuses(ctx context.Context, client Client, statuses []PullRequestStatus) {
	var wg sync.WaitGroup
	limit := make(chan struct{}, statusConcurrency)
	for index := range statuses {
		wg.Add(1)
		go func(entry *PullRequestStatus) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			feedback, err := loadPullRequestFeedback(ctx, client, entry.ProjectID, entry.RepositoryID, entry.PullRequestID)
			if err != nil {
				entry.Error = "Azure DevOps request failed"
				return
			}
			entry.Status, entry.IsDraft = feedback.PullRequest.Status, feedback.PullRequest.IsDraft
			entry.Title, entry.URL = feedback.PullRequest.Title, firstNonEmpty(feedback.PullRequest.WebURL, entry.URL)
			entry.SourceBranch, entry.TargetBranch = feedback.PullRequest.SourceBranch, feedback.PullRequest.TargetBranch
			entry.TaskStatus = reviewTaskStatus(feedback)
		}(&statuses[index])
	}
	wg.Wait()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// linkPullRequest resolves a typed or structured reference, checks it against
// the workspace organization, and links it.
func (t *TaskLinks) linkPullRequest(ctx context.Context, workspaceID, taskID string, input taskLinkInput) (PullRequestLink, error) {
	client, connection, err := t.connections.Client(ctx, workspaceID)
	if err != nil {
		return PullRequestLink{}, err
	}
	reference := pullRequestReference{Project: input.ProjectID, Repository: input.RepositoryID, ID: input.PullRequestID}
	if strings.TrimSpace(input.Reference) != "" {
		if reference, err = parsePullRequestReference(input.Reference); err != nil {
			return PullRequestLink{}, invalidArgument(err)
		}
	}
	if reference.ID <= 0 {
		return PullRequestLink{}, invalidArgument(errInvalidReference)
	}
	if err := requireOrganization(connection.OrganizationURL, reference.Organization); err != nil {
		return PullRequestLink{}, err
	}
	pr, err := t.findPullRequest(ctx, client, workspaceID, taskID, connection.OrganizationURL, reference)
	if err != nil {
		return PullRequestLink{}, err
	}
	link := pullRequestLinkFrom(*pr)
	if _, err := t.links.AddPullRequest(ctx, workspaceID, taskID, link); err != nil {
		return PullRequestLink{}, unavailableIf(err)
	}
	return link, nil
}

func pullRequestLinkFrom(pr azure.PullRequest) PullRequestLink {
	return PullRequestLink{
		ProjectID: pr.ProjectID, ProjectName: pr.ProjectName, RepositoryID: pr.RepositoryID, RepositoryName: pr.RepositoryName,
		PullRequestID: pr.ID, Title: pr.Title, URL: pr.WebURL, SourceBranch: pr.SourceBranch, TargetBranch: pr.TargetBranch,
	}
}

func (t *TaskLinks) findPullRequest(ctx context.Context, client Client, workspaceID, taskID, organizationURL string, reference pullRequestReference) (*azure.PullRequest, error) {
	if reference.Project != "" && reference.Repository != "" {
		pr, err := client.GetPullRequest(ctx, reference.Project, reference.Repository, reference.ID)
		if err != nil {
			return nil, referenceLookupError(err)
		}
		return pr, nil
	}
	repositories, err := t.taskRepositories(ctx, client, workspaceID, taskID, organizationURL)
	if err != nil {
		return nil, err
	}
	for _, repository := range repositories {
		pr, err := client.GetPullRequest(ctx, repository.ProjectID, repository.RepositoryID, reference.ID)
		if err == nil {
			return pr, nil
		}
		if !isNotFound(err) {
			return nil, referenceLookupError(err)
		}
	}
	return nil, notFound(fmt.Errorf("pull request %d was not found in this task's Azure Repos repositories; use its full URL", reference.ID))
}

func (t *TaskLinks) linkWorkItem(ctx context.Context, workspaceID, taskID string, input taskLinkInput) (WorkItemLink, error) {
	client, connection, err := t.connections.Client(ctx, workspaceID)
	if err != nil {
		return WorkItemLink{}, err
	}
	reference := workItemReference{Project: input.ProjectID, ID: input.WorkItemID}
	if strings.TrimSpace(input.Reference) != "" {
		if reference, err = parseWorkItemReference(input.Reference); err != nil {
			return WorkItemLink{}, invalidArgument(err)
		}
	}
	if reference.ID <= 0 {
		return WorkItemLink{}, invalidArgument(errInvalidReference)
	}
	if err := requireOrganization(connection.OrganizationURL, reference.Organization); err != nil {
		return WorkItemLink{}, err
	}
	item, err := client.GetWorkItemDetail(ctx, reference.Project, reference.ID)
	if err != nil {
		return WorkItemLink{}, referenceLookupError(err)
	}
	link := WorkItemLink{
		ProjectID: firstNonEmpty(item.Project, reference.Project), WorkItemID: item.ID, Title: item.Title,
		URL: item.WebURL, State: item.State, Type: item.Type,
	}
	if _, err := t.links.AddWorkItem(ctx, workspaceID, taskID, link); err != nil {
		return WorkItemLink{}, unavailableIf(err)
	}
	return link, nil
}

// requireOrganization rejects references to another organization.
func requireOrganization(organizationURL, organization string) error {
	if organization == "" {
		return nil
	}
	connected, err := azure.OrganizationName(organizationURL)
	if err != nil {
		return failedPrecondition(err)
	}
	if !strings.EqualFold(connected, organization) {
		return invalidArgument(fmt.Errorf("the reference belongs to organization %q, but this workspace is connected to %q", organization, connected))
	}
	return nil
}

func isNotFound(err error) bool {
	var apiErr *azure.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

func referenceLookupError(err error) error {
	if isNotFound(err) {
		return notFound(errors.New("the Azure DevOps item was not found or is not visible to this connection"))
	}
	return browseError(err)
}

// taskRepositories resolves the task's Azure Repos repositories.
func (t *TaskLinks) taskRepositories(ctx context.Context, client Client, workspaceID, taskID, organizationURL string) ([]azureRepository, error) {
	task, err := t.tasks().Get(ctx, taskID)
	if err != nil || task == nil {
		return nil, unavailable(fmt.Errorf("read task: %w", err))
	}
	var result []azureRepository
	for _, entry := range task.Repositories {
		repository, err := t.resolveRepository(ctx, client, workspaceID, entry.RepositoryID, organizationURL)
		if errors.Is(err, errNotAzureRepository) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, repository)
	}
	return result, nil
}

var errNotAzureRepository = errors.New("the repository is not an Azure Repos repository of this connection")

// resolveRepository maps a Kandev repository to its Azure Repos identity.
func (t *TaskLinks) resolveRepository(ctx context.Context, client Client, workspaceID, hostRepositoryID, organizationURL string) (azureRepository, error) {
	repository, err := findHostRepository(ctx, t.repositories(), workspaceID, hostRepositoryID)
	if err != nil {
		return azureRepository{}, err
	}
	if !strings.EqualFold(repository.ProviderID, ProviderID) {
		return azureRepository{}, errNotAzureRepository
	}
	remote, err := azure.ParseRemoteURL(repository.RemoteURL)
	organization, orgErr := azure.OrganizationName(organizationURL)
	if err != nil || orgErr != nil || !strings.EqualFold(remote.Organization, organization) {
		return azureRepository{}, errNotAzureRepository
	}
	project := firstNonEmpty(repository.OwnerOrProject, remote.Project)
	name := firstNonEmpty(repository.ProviderRepositoryID, remote.Repository)
	resolved, err := client.GetRepository(ctx, project, name)
	if err != nil {
		return azureRepository{}, referenceLookupError(err)
	}
	return azureRepository{
		HostRepositoryID: repository.ID, ProjectID: resolved.ProjectID, ProjectName: resolved.ProjectName,
		RepositoryID: resolved.ID, RepositoryName: resolved.Name, DefaultBranch: trimHeads(resolved.DefaultBranch),
	}, nil
}

func trimHeads(branch string) string {
	return strings.TrimPrefix(strings.TrimSpace(branch), "refs/heads/")
}

// findHostRepository pages the workspace repositories for one ID.
func findHostRepository(ctx context.Context, lister RepositoryLister, workspaceID, repositoryID string) (pluginsdk.Repository, error) {
	if lister == nil || repositoryID == "" {
		return pluginsdk.Repository{}, errNotAzureRepository
	}
	cursor := ""
	for {
		repositories, info, err := lister.List(ctx, workspaceID, pluginsdk.Page{Limit: repositoryPageSize, Cursor: cursor})
		if err != nil {
			return pluginsdk.Repository{}, unavailable(fmt.Errorf("list repositories: %w", err))
		}
		for _, repository := range repositories {
			if repository.ID == repositoryID && repository.WorkspaceID == workspaceID {
				return repository, nil
			}
		}
		if info == nil || !info.HasMore || info.NextCursor == "" || info.NextCursor == cursor {
			return pluginsdk.Repository{}, errNotAzureRepository
		}
		cursor = info.NextCursor
	}
}

// openPullRequest creates a pull request for a pushed branch, or reuses the
// active one for the same branch, and links it to the task.
func (t *TaskLinks) openPullRequest(ctx context.Context, workspaceID, taskID string, repository azureRepository, client Client, request azure.CreatePullRequestRequest) (*azure.PullRequest, bool, error) {
	request.SourceBranch, request.TargetBranch = trimHeads(request.SourceBranch), trimHeads(request.TargetBranch)
	if request.TargetBranch == "" {
		request.TargetBranch = repository.DefaultBranch
	}
	if request.SourceBranch == "" || request.TargetBranch == "" || strings.TrimSpace(request.Title) == "" {
		return nil, false, invalidArgument(errors.New("a title, a source branch, and a target branch are required"))
	}
	if request.SourceBranch == request.TargetBranch {
		return nil, false, invalidArgument(errors.New("the source and target branches must differ"))
	}
	existing, err := client.ListPullRequests(ctx, azure.PullRequestFilter{
		ProjectID: repository.ProjectID, RepositoryID: repository.RepositoryID, Status: "active", SourceBranch: request.SourceBranch, Top: 10,
	})
	if err != nil {
		return nil, false, browseError(err)
	}
	var pr *azure.PullRequest
	for index := range existing.Items {
		if existing.Items[index].SourceBranch == request.SourceBranch && existing.Items[index].TargetBranch == request.TargetBranch {
			pr = &existing.Items[index]
			break
		}
	}
	if pr == nil {
		request.WorkItemIDs = t.linkedWorkItemIDs(ctx, workspaceID, taskID)
		if pr, err = client.CreatePullRequest(ctx, repository.ProjectID, repository.RepositoryID, request); err != nil {
			return nil, false, browseError(err)
		}
	}
	_, linkErr := t.links.AddPullRequest(ctx, workspaceID, taskID, pullRequestLinkFrom(*pr))
	return pr, linkErr == nil, nil
}

func (t *TaskLinks) linkedWorkItemIDs(ctx context.Context, workspaceID, taskID string) []int {
	links, err := t.links.WorkItems(ctx, workspaceID, taskID)
	if err != nil {
		return nil
	}
	ids := make([]int, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.WorkItemID)
	}
	return ids
}

// createFromSession serves the native Create PR action. Kandev verified the
// session checkout of the repository and supplies its branch.
func (t *TaskLinks) createFromSession(ctx context.Context, verified pluginsdk.VerifiedActionContext, input createPullRequestInput) (any, error) {
	if verified.RepositoryID == "" || strings.TrimSpace(verified.HeadBranch) == "" {
		return nil, invalidArgument(errors.New("a task session checkout is required"))
	}
	client, connection, err := t.connections.Client(ctx, verified.WorkspaceID)
	if err != nil {
		return nil, err
	}
	repository, err := t.resolveRepository(ctx, client, verified.WorkspaceID, verified.RepositoryID, connection.OrganizationURL)
	if errors.Is(err, errNotAzureRepository) {
		return nil, invalidArgument(err)
	}
	if err != nil {
		return nil, err
	}
	pr, linked, err := t.openPullRequest(ctx, verified.WorkspaceID, verified.TaskID, repository, client, azure.CreatePullRequestRequest{
		SourceBranch: verified.HeadBranch, TargetBranch: input.BaseBranch, Title: input.Title,
		Description: input.Description, IsDraft: input.Draft,
	})
	if err != nil {
		return nil, err
	}
	result := map[string]any{"url": pr.WebURL, "number": pr.ID, "linked": linked}
	if !linked {
		result["associationError"] = "the pull request was created but could not be linked to the task"
	}
	return result, nil
}

// decodeJSONMap re-encodes loosely typed agent arguments into target.
func decodeJSONMap(arguments map[string]any, target any) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

// LinkedTask is the display summary of a linked Kandev task.
type LinkedTask struct {
	Title string `json:"title"`
}

// taskTitles reads the titles of linked tasks. Unreadable tasks are omitted.
func (t *TaskLinks) taskTitles(ctx context.Context, links WorkspaceLinks) map[string]LinkedTask {
	titles := map[string]LinkedTask{}
	reader := t.tasks()
	for _, group := range []map[string]bool{keysOf(links.PullRequests), keysOf(links.WorkItems)} {
		for taskID := range group {
			if _, done := titles[taskID]; done {
				continue
			}
			if task, err := reader.Get(ctx, taskID); err == nil && task != nil {
				titles[taskID] = LinkedTask{Title: task.Title}
			}
		}
	}
	return titles
}

func keysOf[V any](values map[string]V) map[string]bool {
	keys := make(map[string]bool, len(values))
	for key := range values {
		keys[key] = true
	}
	return keys
}
