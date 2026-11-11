package plugin

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/azure"
)

// Composer reference sources declared in the manifest.
const (
	WorkItemReferenceSource    = "azure_work_items"
	PullRequestReferenceSource = "azure_pull_requests"

	defaultMentionLimit = 5
	maxMentionLimit     = 10
	maxMentionProjects  = 5
	mentionPRPageSize   = 100
	maxMentionPRPages   = 5
)

// Mentions serves composer # references to work items and pull requests.
type Mentions struct{ connections *Connections }

// NewMentions returns the reference source handler.
func NewMentions(connections *Connections) *Mentions { return &Mentions{connections: connections} }

// Search returns display candidates. Kandev authorizes a selection again
// before a message is submitted.
func (m *Mentions) Search(ctx context.Context, request *pluginsdk.SearchEntityReferencesRequest) (*pluginsdk.SearchEntityReferencesResponse, error) {
	if request == nil || request.WorkspaceID == "" {
		return nil, invalidArgument(errors.New("a workspace is required"))
	}
	query := strings.TrimSpace(request.Query)
	limit := int(request.Limit)
	if limit <= 0 {
		limit = defaultMentionLimit
	}
	limit = min(limit, maxMentionLimit)
	response := &pluginsdk.SearchEntityReferencesResponse{Candidates: []pluginsdk.EntityReferenceCandidate{}}
	if query == "" {
		return response, nil
	}
	client, connection, err := m.connections.Client(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	projects, err := mentionProjects(ctx, client, connection.DefaultProjectID, connection.DefaultProjectName)
	if err != nil {
		return nil, browseError(err)
	}
	switch request.Source {
	case WorkItemReferenceSource:
		response.Candidates, err = searchWorkItemMentions(ctx, client, projects, query, limit)
	case PullRequestReferenceSource:
		response.Candidates, err = searchPullRequestMentions(ctx, client, projects, query, limit)
	default:
		return nil, invalidArgument(errors.New("unknown reference source"))
	}
	if err != nil {
		return nil, browseError(err)
	}
	return response, nil
}

// Authorize re-reads the referenced item with the workspace connection.
func (m *Mentions) Authorize(ctx context.Context, request *pluginsdk.AuthorizeEntityReferenceRequest) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
	deny := func(reason string) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
		return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false, Reason: reason}, nil
	}
	if request == nil || request.WorkspaceID == "" || (request.Purpose != "search" && request.Purpose != "submission") {
		return deny("invalid Azure DevOps reference")
	}
	key, ok := referenceKey(request.Reference)
	if !ok {
		return deny("incomplete Azure DevOps reference")
	}
	client, _, err := m.connections.Client(ctx, request.WorkspaceID)
	if err != nil {
		return deny("Azure DevOps is not connected")
	}
	switch request.Source {
	case WorkItemReferenceSource:
		reference, err := parseWorkItemReference(key)
		if err != nil || reference.Project == "" {
			return deny("invalid Azure DevOps work item reference")
		}
		item, err := client.GetWorkItemDetail(ctx, reference.Project, reference.ID)
		if err != nil || item.ID != reference.ID {
			return deny("the Azure DevOps work item is unavailable")
		}
	case PullRequestReferenceSource:
		reference, err := parsePullRequestReference(key)
		if err != nil || reference.Project == "" || reference.Repository == "" {
			return deny("invalid Azure DevOps pull request reference")
		}
		pr, err := client.GetPullRequest(ctx, reference.Project, reference.Repository, reference.ID)
		if err != nil || pr.ID != reference.ID {
			return deny("the Azure DevOps pull request is unavailable")
		}
	default:
		return deny("unknown reference source")
	}
	return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: true}, nil
}

// referenceKey returns the canonical key; Kandev sends it as both id and key.
func referenceKey(reference map[string]any) (string, bool) {
	key, _ := reference["key"].(string)
	id, _ := reference["id"].(string)
	key, id = strings.TrimSpace(key), strings.TrimSpace(id)
	if key != "" && id != "" && key != id {
		return "", false
	}
	if key == "" {
		key = id
	}
	return key, key != ""
}

// mentionProjects is the default project, or the first projects by name.
func mentionProjects(ctx context.Context, client Client, defaultID, defaultName string) ([]azure.Project, error) {
	if strings.TrimSpace(defaultID) != "" {
		return []azure.Project{{ID: defaultID, Name: firstNonEmpty(defaultName, defaultID)}}, nil
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(projects, func(left, right int) bool {
		return strings.ToLower(projects[left].Name) < strings.ToLower(projects[right].Name)
	})
	filtered := make([]azure.Project, 0, maxMentionProjects)
	for _, project := range projects {
		if project.ID != "" && project.Name != "" {
			filtered = append(filtered, project)
		}
		if len(filtered) == maxMentionProjects {
			break
		}
	}
	return filtered, nil
}

func mentionWIQL(query string) string {
	if id, ok := positiveID(strings.TrimPrefix(query, "#")); ok {
		return "SELECT [System.Id] FROM WorkItems WHERE [System.Id] = " + strconv.Itoa(id)
	}
	query = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(query)
	query = strings.ReplaceAll(query, "'", "''")
	return "SELECT [System.Id] FROM WorkItems WHERE [System.Title] CONTAINS '" + query + "' ORDER BY [System.ChangedDate] DESC"
}

func searchWorkItemMentions(ctx context.Context, client Client, projects []azure.Project, query string, limit int) ([]pluginsdk.EntityReferenceCandidate, error) {
	candidates := make([]pluginsdk.EntityReferenceCandidate, 0, limit)
	wiql := mentionWIQL(query)
	for _, project := range projects {
		if len(candidates) == limit {
			break
		}
		result, err := client.QueryWIQL(ctx, project.ID, wiql, limit-len(candidates))
		if err != nil {
			return nil, err
		}
		if result == nil {
			continue
		}
		for _, item := range result.Items {
			if item.ID <= 0 || strings.TrimSpace(item.Title) == "" {
				continue
			}
			key := workItemReferenceKey(project.Name, item.ID)
			candidates = append(candidates, pluginsdk.EntityReferenceCandidate{
				ProviderLocalID: key, Title: item.Title, URL: item.WebURL,
				Attributes: map[string]any{"projectId": project.ID, "workItemId": item.ID, "type": item.Type, "state": item.State},
			})
			if len(candidates) == limit {
				break
			}
		}
	}
	return candidates, nil
}

func searchPullRequestMentions(ctx context.Context, client Client, projects []azure.Project, query string, limit int) ([]pluginsdk.EntityReferenceCandidate, error) {
	candidates := make([]pluginsdk.EntityReferenceCandidate, 0, limit)
	needle := strings.ToLower(strings.TrimLeft(query, "!#"))
	for _, project := range projects {
		for page := 0; page < maxMentionPRPages && len(candidates) < limit; page++ {
			result, err := client.ListPullRequests(ctx, azure.PullRequestFilter{
				ProjectID: project.ID, Status: "active", Skip: page * mentionPRPageSize, Top: mentionPRPageSize,
			})
			if err != nil {
				return nil, err
			}
			for _, pr := range result.Items {
				if pr.ID <= 0 || pr.RepositoryName == "" || !pullRequestMatches(pr, needle) {
					continue
				}
				candidates = append(candidates, pluginsdk.EntityReferenceCandidate{
					ProviderLocalID: pullRequestReferenceKey(firstNonEmpty(pr.ProjectName, project.Name), pr.RepositoryName, pr.ID),
					Title:           pr.Title, URL: pr.WebURL,
					Attributes: map[string]any{"projectId": project.ID, "repositoryId": pr.RepositoryID, "pullRequestId": pr.ID},
				})
				if len(candidates) == limit {
					return candidates, nil
				}
			}
			if len(result.Items) < mentionPRPageSize {
				break
			}
		}
	}
	return candidates, nil
}

func pullRequestMatches(pr azure.PullRequest, needle string) bool {
	return strconv.Itoa(pr.ID) == needle || strings.Contains(strings.ToLower(pr.Title), needle)
}
