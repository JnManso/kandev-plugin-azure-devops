package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"kandev-plugin-azure-devops/internal/store"
	"kandev-plugin-azure-devops/internal/watches"
)

const (
	taskLinkKeyPrefix       = "task."
	pullRequestLinksSuffix  = ".pullRequests"
	workItemLinksSuffix     = ".workItems"
	maxLinksPerTaskAndKind  = 50
	errTaskLinkLimitMessage = "a task can hold at most 50 links of each kind"
)

// PullRequestLink associates a Kandev task with one Azure Repos pull request.
// Identity is the repository GUID plus the pull request number.
type PullRequestLink struct {
	// Organization is the lowercase organization name the link was made in.
	Organization   string    `json:"organization,omitempty"`
	ProjectID      string    `json:"projectId"`
	ProjectName    string    `json:"projectName,omitempty"`
	RepositoryID   string    `json:"repositoryId"`
	RepositoryName string    `json:"repositoryName,omitempty"`
	PullRequestID  int       `json:"pullRequestId"`
	Title          string    `json:"title,omitempty"`
	URL            string    `json:"url,omitempty"`
	SourceBranch   string    `json:"sourceBranch,omitempty"`
	TargetBranch   string    `json:"targetBranch,omitempty"`
	LinkedAt       time.Time `json:"linkedAt"`
}

func (l PullRequestLink) sameAs(other PullRequestLink) bool {
	return strings.EqualFold(l.RepositoryID, other.RepositoryID) && l.PullRequestID == other.PullRequestID
}

// WorkItemLink associates a Kandev task with one Azure Boards work item. Work
// item IDs are unique within an organization, which Organization records.
type WorkItemLink struct {
	Organization string    `json:"organization,omitempty"`
	ProjectID    string    `json:"projectId"`
	WorkItemID   int       `json:"workItemId"`
	Title        string    `json:"title,omitempty"`
	URL          string    `json:"url,omitempty"`
	State        string    `json:"state,omitempty"`
	Type         string    `json:"type,omitempty"`
	LinkedAt     time.Time `json:"linkedAt"`
}

// WorkspaceLinks groups every link of one workspace by task.
type WorkspaceLinks struct {
	PullRequests map[string][]PullRequestLink `json:"pullRequests"`
	WorkItems    map[string][]WorkItemLink    `json:"workItems"`
}

type pullRequestLinkRecord struct {
	Links []PullRequestLink `json:"links"`
}

type workItemLinkRecord struct {
	Links []WorkItemLink `json:"links"`
}

// Links stores task links in workspace state under
// task.<taskId>.pullRequests and task.<taskId>.workItems.
type Links struct {
	store *store.Store
	keys  func(ctx context.Context, workspaceID string) ([]string, error)
	now   func() time.Time
	mu    sync.Mutex
}

// NewLinks returns a link store. keys lists a workspace's state keys.
func NewLinks(store *store.Store, keys func(ctx context.Context, workspaceID string) ([]string, error)) *Links {
	return &Links{store: store, keys: keys, now: time.Now}
}

func pullRequestLinksKey(taskID string) string {
	return taskLinkKeyPrefix + taskID + pullRequestLinksSuffix
}
func workItemLinksKey(taskID string) string { return taskLinkKeyPrefix + taskID + workItemLinksSuffix }

func requireLinkScope(workspaceID, taskID string) error {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(taskID) == "" {
		return errors.New("workspace and task are required")
	}
	return nil
}

// PullRequests returns a task's pull request links.
func (l *Links) PullRequests(ctx context.Context, workspaceID, taskID string) ([]PullRequestLink, error) {
	if err := requireLinkScope(workspaceID, taskID); err != nil {
		return nil, err
	}
	var record pullRequestLinkRecord
	if _, err := l.store.LoadState(ctx, workspaceID, pullRequestLinksKey(taskID), &record); err != nil {
		return nil, err
	}
	return nonNil(record.Links), nil
}

// WorkItems returns a task's work item links.
func (l *Links) WorkItems(ctx context.Context, workspaceID, taskID string) ([]WorkItemLink, error) {
	if err := requireLinkScope(workspaceID, taskID); err != nil {
		return nil, err
	}
	var record workItemLinkRecord
	if _, err := l.store.LoadState(ctx, workspaceID, workItemLinksKey(taskID), &record); err != nil {
		return nil, err
	}
	return nonNil(record.Links), nil
}

// AddPullRequest links a pull request, refreshing the stored summary when the
// link already exists.
func (l *Links) AddPullRequest(ctx context.Context, workspaceID, taskID string, link PullRequestLink) ([]PullRequestLink, error) {
	if link.RepositoryID == "" || link.PullRequestID <= 0 {
		return nil, errors.New("a repository and pull request number are required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	links, err := l.PullRequests(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	for index, existing := range links {
		if existing.sameAs(link) {
			link.LinkedAt = existing.LinkedAt
			links[index] = link
			return links, l.savePullRequests(ctx, workspaceID, taskID, links)
		}
	}
	if len(links) >= maxLinksPerTaskAndKind {
		return nil, invalidArgument(errors.New(errTaskLinkLimitMessage))
	}
	link.LinkedAt = l.now().UTC()
	links = append(links, link)
	return links, l.savePullRequests(ctx, workspaceID, taskID, links)
}

// RemovePullRequest unlinks a pull request. Removing a missing link is not an error.
func (l *Links) RemovePullRequest(ctx context.Context, workspaceID, taskID, repositoryID string, pullRequestID int) ([]PullRequestLink, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	links, err := l.PullRequests(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	target := PullRequestLink{RepositoryID: repositoryID, PullRequestID: pullRequestID}
	kept := links[:0]
	for _, existing := range links {
		if !existing.sameAs(target) {
			kept = append(kept, existing)
		}
	}
	return kept, l.savePullRequests(ctx, workspaceID, taskID, kept)
}

// AddWorkItem links a work item, refreshing the stored summary when the link
// already exists.
func (l *Links) AddWorkItem(ctx context.Context, workspaceID, taskID string, link WorkItemLink) ([]WorkItemLink, error) {
	if link.WorkItemID <= 0 {
		return nil, errors.New("a work item ID is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	links, err := l.WorkItems(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	for index, existing := range links {
		if existing.WorkItemID == link.WorkItemID {
			link.LinkedAt = existing.LinkedAt
			links[index] = link
			return links, l.saveWorkItems(ctx, workspaceID, taskID, links)
		}
	}
	if len(links) >= maxLinksPerTaskAndKind {
		return nil, invalidArgument(errors.New(errTaskLinkLimitMessage))
	}
	link.LinkedAt = l.now().UTC()
	links = append(links, link)
	return links, l.saveWorkItems(ctx, workspaceID, taskID, links)
}

// RemoveWorkItem unlinks a work item. Removing a missing link is not an error.
func (l *Links) RemoveWorkItem(ctx context.Context, workspaceID, taskID string, workItemID int) ([]WorkItemLink, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	links, err := l.WorkItems(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	kept := links[:0]
	for _, existing := range links {
		if existing.WorkItemID != workItemID {
			kept = append(kept, existing)
		}
	}
	return kept, l.saveWorkItems(ctx, workspaceID, taskID, kept)
}

// DeleteTask removes every link of a deleted task.
func (l *Links) DeleteTask(ctx context.Context, workspaceID, taskID string) error {
	if err := requireLinkScope(workspaceID, taskID); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.store.DeleteState(ctx, workspaceID, pullRequestLinksKey(taskID)); err != nil {
		return err
	}
	return l.store.DeleteState(ctx, workspaceID, workItemLinksKey(taskID))
}

// LinkedTaskIDs returns the sorted IDs of tasks that hold links in a workspace.
func (l *Links) LinkedTaskIDs(ctx context.Context, workspaceID string) ([]string, error) {
	keys, err := l.keys(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if taskID, ok := linkTaskID(key); ok {
			seen[taskID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// Workspace returns every link of a workspace, grouped by task.
func (l *Links) Workspace(ctx context.Context, workspaceID string) (WorkspaceLinks, error) {
	result := WorkspaceLinks{PullRequests: map[string][]PullRequestLink{}, WorkItems: map[string][]WorkItemLink{}}
	taskIDs, err := l.LinkedTaskIDs(ctx, workspaceID)
	if err != nil {
		return result, err
	}
	for _, taskID := range taskIDs {
		pullRequests, err := l.PullRequests(ctx, workspaceID, taskID)
		if err != nil {
			return result, err
		}
		if len(pullRequests) > 0 {
			result.PullRequests[taskID] = pullRequests
		}
		workItems, err := l.WorkItems(ctx, workspaceID, taskID)
		if err != nil {
			return result, err
		}
		if len(workItems) > 0 {
			result.WorkItems[taskID] = workItems
		}
	}
	return result, nil
}

func linkTaskID(key string) (string, bool) {
	if !strings.HasPrefix(key, taskLinkKeyPrefix) {
		return "", false
	}
	for _, suffix := range []string{pullRequestLinksSuffix, workItemLinksSuffix} {
		if strings.HasSuffix(key, suffix) {
			id := strings.TrimSuffix(strings.TrimPrefix(key, taskLinkKeyPrefix), suffix)
			return id, id != ""
		}
	}
	return "", false
}

func (l *Links) savePullRequests(ctx context.Context, workspaceID, taskID string, links []PullRequestLink) error {
	if len(links) == 0 {
		return l.store.DeleteState(ctx, workspaceID, pullRequestLinksKey(taskID))
	}
	if err := l.store.SaveState(ctx, workspaceID, pullRequestLinksKey(taskID), pullRequestLinkRecord{Links: links}); err != nil {
		return fmt.Errorf("save pull request links: %w", err)
	}
	return nil
}

func (l *Links) saveWorkItems(ctx context.Context, workspaceID, taskID string, links []WorkItemLink) error {
	if len(links) == 0 {
		return l.store.DeleteState(ctx, workspaceID, workItemLinksKey(taskID))
	}
	if err := l.store.SaveState(ctx, workspaceID, workItemLinksKey(taskID), workItemLinkRecord{Links: links}); err != nil {
		return fmt.Errorf("save work item links: %w", err)
	}
	return nil
}

// linkWatchItem links a task a watch created to the item it matched.
func (l *Links) linkWatchItem(ctx context.Context, workspaceID, taskID string, item watches.ItemLink) error {
	if item.PullRequestID > 0 {
		_, err := l.AddPullRequest(ctx, workspaceID, taskID, PullRequestLink{
			Organization: item.Organization, ProjectID: item.ProjectID, ProjectName: item.ProjectName,
			RepositoryID: item.RepositoryID, RepositoryName: item.RepositoryName, PullRequestID: item.PullRequestID,
			Title: item.Title, URL: item.URL, SourceBranch: item.SourceBranch, TargetBranch: item.TargetBranch,
		})
		return err
	}
	if item.WorkItemID > 0 {
		_, err := l.AddWorkItem(ctx, workspaceID, taskID, WorkItemLink{
			Organization: item.Organization, ProjectID: item.ProjectID, WorkItemID: item.WorkItemID,
			Title: item.Title, URL: item.URL, State: item.State, Type: item.Type,
		})
		return err
	}
	return nil
}
