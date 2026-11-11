package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/azure"
)

// CreatePullRequestTool is the manifest name of the agent pull request tool.
const CreatePullRequestTool = "create_pull_request"

type createPullRequestArguments struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"sourceBranch"`
	TargetBranch string `json:"targetBranch"`
	Repository   string `json:"repository"`
	Draft        bool   `json:"draft"`
}

// InvokeCreatePullRequest opens and links a pull request for the agent's task.
// The agent supplies its pushed branch; the plugin resolves the repository
// from the task and never exposes a credential.
func (t *TaskLinks) InvokeCreatePullRequest(ctx context.Context, request *pluginsdk.AgentToolRequest) *pluginsdk.AgentToolResult {
	var arguments createPullRequestArguments
	if err := decodeJSONMap(request.Arguments, &arguments); err != nil {
		return toolError("the arguments are not valid: %v", err)
	}
	workspaceID, taskID := request.Context.WorkspaceID, request.Context.TaskID
	if workspaceID == "" || taskID == "" {
		return toolError("this tool only works inside a Kandev task")
	}
	client, connection, err := t.connections.Client(ctx, workspaceID)
	if err != nil {
		return toolError("Azure DevOps is not connected for this workspace: %s", publicMessage(err))
	}
	repositories, err := t.taskRepositories(ctx, client, workspaceID, taskID, connection.OrganizationURL)
	if err != nil {
		return toolError("the task repositories could not be read: %s", publicMessage(err))
	}
	repository, err := selectRepository(repositories, arguments.Repository)
	if err != nil {
		return toolError("%s", err.Error())
	}
	source := trimHeads(arguments.SourceBranch)
	if source == "" {
		return toolError("sourceBranch is required: pass the branch you pushed (git branch --show-current)")
	}
	if pushed, err := branchExists(ctx, client, repository, source); err != nil {
		return toolError("the branches of %s could not be read: %s", repository.RepositoryName, publicMessage(err))
	} else if !pushed {
		return toolError("branch %q is not on the Azure Repos remote of %s; push it first (git push -u origin %s)", source, repository.RepositoryName, source)
	}
	target := firstNonEmpty(trimHeads(arguments.TargetBranch), t.taskBaseBranch(ctx, taskID, repository.HostRepositoryID))
	pr, linked, err := t.openPullRequest(ctx, workspaceID, taskID, repository, client, azure.CreatePullRequestRequest{
		SourceBranch: source, TargetBranch: target, Title: arguments.Title, Description: arguments.Description, IsDraft: arguments.Draft,
	})
	if err != nil {
		return toolError("the pull request could not be created: %s", publicMessage(err))
	}
	text := fmt.Sprintf("Pull request !%d is open: %s", pr.ID, pr.WebURL)
	if !linked {
		text += " (it could not be linked to the task)"
	}
	return &pluginsdk.AgentToolResult{Text: text, StructuredContent: map[string]any{
		"pullRequestId": pr.ID, "url": pr.WebURL, "repository": repository.RepositoryName,
		"sourceBranch": pr.SourceBranch, "targetBranch": pr.TargetBranch, "linked": linked,
	}}
}

func toolError(format string, values ...any) *pluginsdk.AgentToolResult {
	return &pluginsdk.AgentToolResult{Text: fmt.Sprintf(format, values...), IsError: true}
}

// publicMessage returns the message of a categorized action error and a
// generic text for anything else, so provider bodies never reach the agent.
func publicMessage(err error) string {
	var actionErr *pluginsdk.ActionError
	if errors.As(err, &actionErr) && actionErr.Err != nil && safeMessages[actionErr.Code] {
		return actionErr.Err.Error()
	}
	if errors.Is(err, errNotAzureRepository) {
		return err.Error()
	}
	return "Azure DevOps request failed"
}

func selectRepository(repositories []azureRepository, requested string) (azureRepository, error) {
	requested = strings.TrimSpace(requested)
	if len(repositories) == 0 {
		return azureRepository{}, errors.New("this task has no Azure Repos repository of the connected organization")
	}
	if requested == "" {
		if len(repositories) == 1 {
			return repositories[0], nil
		}
		return azureRepository{}, fmt.Errorf("this task has %d Azure Repos repositories; pass repository with one of: %s", len(repositories), describeRepositoryChoices(repositories))
	}
	for _, repository := range repositories {
		if strings.EqualFold(repository.RepositoryName, requested) || strings.EqualFold(repository.RepositoryID, requested) ||
			strings.EqualFold(repository.ProjectName+"/"+repository.RepositoryName, requested) {
			return repository, nil
		}
	}
	return azureRepository{}, fmt.Errorf("repository %q is not part of this task; use one of: %s", requested, describeRepositoryChoices(repositories))
}

func describeRepositoryChoices(repositories []azureRepository) string {
	names := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		names = append(names, repository.ProjectName+"/"+repository.RepositoryName)
	}
	return strings.Join(names, ", ")
}

func branchExists(ctx context.Context, client Client, repository azureRepository, branch string) (bool, error) {
	branches, err := client.ListBranches(ctx, repository.ProjectID, repository.RepositoryID)
	if err != nil {
		return false, browseError(err)
	}
	for _, candidate := range branches {
		if trimHeads(candidate.Name) == branch {
			return true, nil
		}
	}
	return false, nil
}

// taskBaseBranch returns the base branch the task chose for a repository.
func (t *TaskLinks) taskBaseBranch(ctx context.Context, taskID, hostRepositoryID string) string {
	task, err := t.tasks().Get(ctx, taskID)
	if err != nil || task == nil {
		return ""
	}
	for _, entry := range task.Repositories {
		if entry.RepositoryID == hostRepositoryID {
			return trimHeads(entry.BaseBranch)
		}
	}
	return ""
}
