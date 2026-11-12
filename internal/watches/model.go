// Package watches polls Azure DevOps for new work items and pull requests and
// creates one Kandev task per match through the host task API.
package watches

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"kandev-plugin-azure-devops/internal/azure"
)

// Kind selects what a watch matches.
type Kind string

const (
	KindWorkItem    Kind = "work_item"
	KindPullRequest Kind = "pull_request"

	DefaultPollIntervalSeconds = 300
	MinPollIntervalSeconds     = 60
	maxMatchesPerPoll          = 100
	// maxWatchCandidates bounds how far one poll reads past items the watch
	// already handled.
	maxWatchCandidates = 2000

	// Disabled reasons shown, translated, in settings.
	ReasonAgentProfileUnavailable    = "agent_profile_unavailable"
	ReasonExecutorProfileUnavailable = "executor_profile_unavailable"
)

var (
	// ErrInvalid marks a watch that fails validation.
	ErrInvalid = errors.New("invalid watch")
	// ErrNotFound marks a missing watch.
	ErrNotFound = errors.New("watch not found")
)

// Watch is one persisted work-item or pull-request watch.
type Watch struct {
	ID                  string `json:"id"`
	Kind                Kind   `json:"kind"`
	WorkspaceID         string `json:"workspaceId"`
	WorkflowID          string `json:"workflowId"`
	WorkflowStepID      string `json:"workflowStepId"`
	ProjectID           string `json:"projectId"`
	WIQL                string `json:"wiql,omitempty"`
	AzureRepositoryID   string `json:"azureRepositoryId,omitempty"`
	Status              string `json:"status,omitempty"`
	CreatorID           string `json:"creatorId,omitempty"`
	ReviewerID          string `json:"reviewerId,omitempty"`
	RepositoryID        string `json:"repositoryId,omitempty"`
	BaseBranch          string `json:"baseBranch,omitempty"`
	AgentProfileID      string `json:"agentProfileId"`
	ExecutorProfileID   string `json:"executorProfileId"`
	Prompt              string `json:"prompt"`
	Enabled             bool   `json:"enabled"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	MaxInflightTasks    int    `json:"maxInflightTasks,omitempty"`
	Generation          int64  `json:"generation"`
	// Organization is the lowercase organization the current generation's
	// reservations belong to. Connecting the workspace to another
	// organization starts a new generation, because work item numbers are
	// unique only within an organization.
	Organization   string     `json:"organization,omitempty"`
	DisabledReason string     `json:"disabledReason,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	LastErrorAt    *time.Time `json:"lastErrorAt,omitempty"`
	LastPolledAt   *time.Time `json:"lastPolledAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// Input is the editable part of a watch. Nil fields keep their value on update.
type Input struct {
	Kind                *Kind   `json:"kind,omitempty"`
	WorkflowID          *string `json:"workflowId,omitempty"`
	WorkflowStepID      *string `json:"workflowStepId,omitempty"`
	ProjectID           *string `json:"projectId,omitempty"`
	WIQL                *string `json:"wiql,omitempty"`
	AzureRepositoryID   *string `json:"azureRepositoryId,omitempty"`
	Status              *string `json:"status,omitempty"`
	CreatorID           *string `json:"creatorId,omitempty"`
	ReviewerID          *string `json:"reviewerId,omitempty"`
	RepositoryID        *string `json:"repositoryId,omitempty"`
	BaseBranch          *string `json:"baseBranch,omitempty"`
	AgentProfileID      *string `json:"agentProfileId,omitempty"`
	ExecutorProfileID   *string `json:"executorProfileId,omitempty"`
	Prompt              *string `json:"prompt,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
	PollIntervalSeconds *int    `json:"pollIntervalSeconds,omitempty"`
	MaxInflightTasks    *int    `json:"maxInflightTasks,omitempty"`
}

func set[T any](target *T, value *T, normalize func(T) T) {
	if value != nil {
		*target = normalize(*value)
	}
}

func same[T any](value T) T { return value }

// Apply copies the provided input fields onto w.
func (w *Watch) Apply(in Input) {
	set(&w.WorkflowID, in.WorkflowID, strings.TrimSpace)
	set(&w.WorkflowStepID, in.WorkflowStepID, strings.TrimSpace)
	set(&w.ProjectID, in.ProjectID, strings.TrimSpace)
	set(&w.WIQL, in.WIQL, strings.TrimSpace)
	set(&w.AzureRepositoryID, in.AzureRepositoryID, strings.TrimSpace)
	set(&w.Status, in.Status, strings.TrimSpace)
	set(&w.CreatorID, in.CreatorID, strings.TrimSpace)
	set(&w.ReviewerID, in.ReviewerID, strings.TrimSpace)
	set(&w.RepositoryID, in.RepositoryID, strings.TrimSpace)
	set(&w.BaseBranch, in.BaseBranch, strings.TrimSpace)
	set(&w.AgentProfileID, in.AgentProfileID, strings.TrimSpace)
	set(&w.ExecutorProfileID, in.ExecutorProfileID, strings.TrimSpace)
	set(&w.Prompt, in.Prompt, same[string])
	set(&w.Enabled, in.Enabled, same[bool])
	set(&w.PollIntervalSeconds, in.PollIntervalSeconds, normalizePollInterval)
	set(&w.MaxInflightTasks, in.MaxInflightTasks, same[int])
}

func normalizePollInterval(value int) int {
	if value <= 0 {
		return DefaultPollIntervalSeconds
	}
	return max(value, MinPollIntervalSeconds)
}

// Validate checks the fields a watch needs to create tasks.
func (w *Watch) Validate() error {
	missing := []string{}
	require := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	require("projectId", w.ProjectID)
	require("workflowId", w.WorkflowID)
	require("workflowStepId", w.WorkflowStepID)
	require("agentProfileId", w.AgentProfileID)
	require("executorProfileId", w.ExecutorProfileID)
	switch w.Kind {
	case KindWorkItem:
		require("wiql", w.WIQL)
	case KindPullRequest:
		if !validPullRequestStatus(w.Status) {
			return fmt.Errorf("%w: status must be active, completed, abandoned, or all", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: kind must be work_item or pull_request", ErrInvalid)
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s required", ErrInvalid, strings.Join(missing, ", "))
	}
	if w.MaxInflightTasks < 0 {
		return fmt.Errorf("%w: maxInflightTasks must be zero or greater", ErrInvalid)
	}
	if w.BaseBranch != "" && w.RepositoryID == "" {
		return fmt.Errorf("%w: a base branch needs a repository", ErrInvalid)
	}
	return nil
}

func validPullRequestStatus(status string) bool {
	switch status {
	case "", "active", "completed", "abandoned", "all":
		return true
	default:
		return false
	}
}

// Due reports whether the watch should poll at now.
func (w *Watch) Due(now time.Time) bool {
	if !w.Enabled {
		return false
	}
	if w.LastPolledAt == nil {
		return true
	}
	return !now.Before(w.LastPolledAt.Add(time.Duration(normalizePollInterval(w.PollIntervalSeconds)) * time.Second))
}

// Match is one Azure DevOps item a watch found.
type Match struct {
	Key         string
	URL         string
	Title       string
	Description string
	WorkItem    *azure.WorkItem
	PullRequest *azure.PullRequest
}

// ItemLink is what a watch records about a matched item, so the created task
// can be linked to it, including by a later poll after a failed attempt.
// Organization is the lowercase organization name the item belongs to.
type ItemLink struct {
	Organization   string `json:"organization,omitempty"`
	WorkItemID     int    `json:"workItemId,omitempty"`
	PullRequestID  int    `json:"pullRequestId,omitempty"`
	ProjectID      string `json:"projectId,omitempty"`
	ProjectName    string `json:"projectName,omitempty"`
	RepositoryID   string `json:"repositoryId,omitempty"`
	RepositoryName string `json:"repositoryName,omitempty"`
	Title          string `json:"title,omitempty"`
	URL            string `json:"url,omitempty"`
	State          string `json:"state,omitempty"`
	Type           string `json:"type,omitempty"`
	SourceBranch   string `json:"sourceBranch,omitempty"`
	TargetBranch   string `json:"targetBranch,omitempty"`
}

// Link describes the match for a task link in organization.
func (m Match) Link(organization, defaultProject string) ItemLink {
	link := ItemLink{Organization: organization, Title: m.Title, URL: m.URL}
	if pr := m.PullRequest; pr != nil {
		link.PullRequestID, link.ProjectID, link.ProjectName = pr.ID, pr.ProjectID, pr.ProjectName
		link.RepositoryID, link.RepositoryName = pr.RepositoryID, pr.RepositoryName
		link.SourceBranch, link.TargetBranch, link.State = pr.SourceBranch, pr.TargetBranch, pr.Status
		return link
	}
	if item := m.WorkItem; item != nil {
		link.WorkItemID, link.ProjectID = item.ID, firstNonEmpty(item.Project, defaultProject)
		link.State, link.Type = item.State, item.Type
	}
	return link
}

// WorkItemMatch identifies a work item; IDs are unique in an organization.
func WorkItemMatch(item azure.WorkItem) Match {
	return Match{Key: workItemKey(item.ID), URL: item.WebURL, Title: item.Title, WorkItem: &item}
}

func workItemKey(id int) string { return "wi-" + strconv.Itoa(id) }

// PullRequestMatch identifies a pull request by repository and number.
func PullRequestMatch(pr azure.PullRequest) Match {
	return Match{Key: "pr-" + strings.ToLower(pr.RepositoryID) + "-" + strconv.Itoa(pr.ID), URL: pr.WebURL, Title: pr.Title, PullRequest: &pr}
}

// TaskTitleMaxLength is the longest task title, in characters, that Kandev
// accepts when it creates a task.
const TaskTitleMaxLength = 60

// TaskTitle and TaskPrompt render the created task. They are task data in
// English, like the built-in integration. The title is shortened with an
// ellipsis to fit Kandev's limit; the prompt keeps the full item title.
func (w *Watch) TaskTitle(match Match) string {
	if match.PullRequest != nil {
		return truncateTaskTitle(fmt.Sprintf("[%s!%d] %s", firstNonEmpty(match.PullRequest.ProjectName, w.ProjectID), match.PullRequest.ID, match.Title))
	}
	project := w.ProjectID
	if match.WorkItem.Project != "" {
		project = match.WorkItem.Project
	}
	return truncateTaskTitle(fmt.Sprintf("[%s#%d] %s", project, match.WorkItem.ID, match.Title))
}

func truncateTaskTitle(title string) string {
	characters := []rune(title)
	if len(characters) <= TaskTitleMaxLength {
		return title
	}
	return string(characters[:TaskTitleMaxLength-1]) + "…"
}

// TaskPrompt interpolates the watch prompt with the matched item.
func (w *Watch) TaskPrompt(match Match) string {
	if pr := match.PullRequest; pr != nil {
		return strings.NewReplacer(
			"{{pull_request.url}}", pr.WebURL, "{{pull_request.title}}", pr.Title,
			"{{pull_request.project}}", pr.ProjectName, "{{pull_request.id}}", strconv.Itoa(pr.ID),
			"{{pull_request.description}}", pr.Description, "{{pull_request.state}}", pr.Status,
			"{{pull_request.source_branch}}", pr.SourceBranch, "{{pull_request.target_branch}}", pr.TargetBranch,
		).Replace(w.Prompt)
	}
	item := match.WorkItem
	return strings.NewReplacer(
		"{{work_item.url}}", item.WebURL, "{{work_item.title}}", item.Title,
		"{{work_item.project}}", item.Project, "{{work_item.id}}", strconv.Itoa(item.ID),
		"{{work_item.description}}", item.Description, "{{work_item.state}}", item.State,
		"{{work_item.type}}", item.Type,
	).Replace(w.Prompt)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
