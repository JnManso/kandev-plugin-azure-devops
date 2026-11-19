package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	viewsStateKey    = "views"
	settingsStateKey = "workspace_settings"
	maxSavedViews    = 50
)

// SavedView is a workspace-scoped browse query. It holds only provider-native
// filters; credentials and result data are never stored.
type SavedView struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	Label        string    `json:"label"`
	ProjectID    string    `json:"projectId"`
	RepositoryID string    `json:"repositoryId,omitempty"`
	WIQL         string    `json:"wiql,omitempty"`
	Top          int       `json:"top,omitempty"`
	Status       string    `json:"status,omitempty"`
	Creator      string    `json:"creator,omitempty"`
	Reviewer     string    `json:"reviewer,omitempty"`
	TeamID       string    `json:"teamId,omitempty"`
	IterationID  string    `json:"iterationId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// QueryPreset is a provider-native browse query offered in the scope bar.
type QueryPreset struct {
	ID      string         `json:"id"`
	Label   string         `json:"label"`
	Group   string         `json:"group"`
	Filters map[string]any `json:"filters"`
}

// ActionPreset is a task launcher entry for an Azure DevOps item.
type ActionPreset struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Hint           string `json:"hint"`
	Icon           string `json:"icon"`
	PromptTemplate string `json:"promptTemplate"`
}

// WorkspaceSettings are the effective presets: overrides or the defaults.
type WorkspaceSettings struct {
	WorkItemQueries    []QueryPreset  `json:"workItemQueries"`
	PullRequestQueries []QueryPreset  `json:"pullRequestQueries"`
	WorkItemActions    []ActionPreset `json:"workItemActions"`
	PullRequestActions []ActionPreset `json:"pullRequestActions"`
}

// DefaultWorkItemQueryPresets mirrors the built-in integration's defaults.
func DefaultWorkItemQueryPresets() []QueryPreset {
	const start = "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project"
	const order = " ORDER BY [System.ChangedDate] DESC"
	return []QueryPreset{
		{ID: "recent", Label: "Recently updated", Group: "inbox", Filters: map[string]any{"wiql": start + order, "top": 50}},
		{ID: "assigned", Label: "Assigned to me", Group: "inbox", Filters: map[string]any{"wiql": start + " AND [System.AssignedTo] = @Me" + order, "top": 50}},
		{ID: "active", Label: "Active", Group: "inbox", Filters: map[string]any{"wiql": start + " AND [System.State] <> 'Closed' AND [System.State] <> 'Done'" + order, "top": 50}},
		{ID: "created", Label: "Created by me", Group: "created", Filters: map[string]any{"wiql": start + " AND [System.CreatedBy] = @Me" + order, "top": 50}},
	}
}

// DefaultPullRequestQueryPresets mirrors the built-in integration's defaults.
func DefaultPullRequestQueryPresets() []QueryPreset {
	return []QueryPreset{
		{ID: "review-requested", Label: "Review requested", Group: "inbox", Filters: map[string]any{"status": "active", "reviewer": "@me", "creator": ""}},
		{ID: "active", Label: "Open", Group: "inbox", Filters: map[string]any{"status": "active", "reviewer": "", "creator": ""}},
		{ID: "completed", Label: "Completed", Group: "created", Filters: map[string]any{"status": "completed", "reviewer": "", "creator": ""}},
		{ID: "created", Label: "Created by me", Group: "created", Filters: map[string]any{"status": "active", "reviewer": "", "creator": "@me"}},
	}
}

// DefaultWorkItemActionPresets mirrors the built-in integration's defaults.
func DefaultWorkItemActionPresets() []ActionPreset {
	return []ActionPreset{
		{ID: "implement", Label: "Implement", Hint: "Build and open a PR", Icon: "code", PromptTemplate: "Implement the Azure DevOps work item at {{url}} (title: \"{{title}}\"). Open a pull request when complete."},
		{ID: "investigate", Label: "Investigate", Hint: "Find the root cause", Icon: "search", PromptTemplate: "Investigate the Azure DevOps work item at {{url}} (title: \"{{title}}\"). Identify the root cause and summarize findings."},
		{ID: "reproduce", Label: "Reproduce", Hint: "Document repro steps", Icon: "bug", PromptTemplate: "Reproduce the Azure DevOps work item at {{url}} (title: \"{{title}}\"). Document the reproduction steps."},
	}
}

// DefaultPullRequestActionPresets mirrors the built-in integration's defaults.
func DefaultPullRequestActionPresets() []ActionPreset {
	return []ActionPreset{
		{ID: "review", Label: "Review", Hint: "Read the diff, flag issues", Icon: "eye", PromptTemplate: "Review the Azure DevOps pull request at {{url}}. Provide feedback on code quality and correctness."},
		{ID: "address-feedback", Label: "Address feedback", Hint: "Apply review comments", Icon: "message", PromptTemplate: "Review and address the feedback on the Azure DevOps pull request at {{url}}."},
		{ID: "fix-ci", Label: "Fix CI", Hint: "Diagnose failing checks", Icon: "tool", PromptTemplate: "Investigate and fix CI failures for the Azure DevOps pull request at {{url}}."},
	}
}

// Views returns the valid saved views; invalid stored entries are skipped.
func (c *Connections) Views(ctx context.Context, workspaceID string) ([]SavedView, error) {
	var stored struct {
		Views []SavedView `json:"views"`
	}
	if _, err := c.store.LoadState(ctx, workspaceID, viewsStateKey, &stored); err != nil {
		return nil, err
	}
	views := make([]SavedView, 0, len(stored.Views))
	seen := map[string]bool{}
	for _, candidate := range stored.Views {
		view, err := validateSavedView(candidate, c.now())
		if err != nil || seen[view.ID] {
			continue
		}
		seen[view.ID] = true
		views = append(views, view)
		if len(views) == maxSavedViews {
			break
		}
	}
	return views, nil
}

// SaveViews replaces the saved views after validating every entry.
func (c *Connections) SaveViews(ctx context.Context, workspaceID string, views []SavedView) ([]SavedView, error) {
	if len(views) > maxSavedViews {
		return nil, invalidArgument(fmt.Errorf("at most %d saved views are allowed", maxSavedViews))
	}
	normalized := make([]SavedView, 0, len(views))
	seen := map[string]bool{}
	for _, candidate := range views {
		view, err := validateSavedView(candidate, c.now())
		if err != nil {
			return nil, invalidArgument(err)
		}
		if seen[view.ID] {
			return nil, invalidArgument(errors.New("duplicate saved view id"))
		}
		seen[view.ID] = true
		normalized = append(normalized, view)
	}
	unlock := c.locks.lock(workspaceID)
	defer unlock()
	if err := c.store.SaveState(ctx, workspaceID, viewsStateKey, map[string]any{"views": normalized}); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateSavedView(view SavedView, now time.Time) (SavedView, error) {
	view.ID, view.Kind, view.Label = strings.TrimSpace(view.ID), strings.TrimSpace(view.Kind), strings.TrimSpace(view.Label)
	view.ProjectID, view.RepositoryID, view.WIQL = strings.TrimSpace(view.ProjectID), strings.TrimSpace(view.RepositoryID), strings.TrimSpace(view.WIQL)
	view.Status, view.Creator, view.Reviewer = strings.TrimSpace(view.Status), strings.TrimSpace(view.Creator), strings.TrimSpace(view.Reviewer)
	view.TeamID, view.IterationID = strings.TrimSpace(view.TeamID), strings.TrimSpace(view.IterationID)
	if view.IterationID != "" && view.TeamID == "" {
		return SavedView{}, errors.New("a saved view sprint requires its team")
	}
	if view.ID == "" || len(view.ID) > 100 || view.Label == "" || len(view.Label) > 80 {
		return SavedView{}, errors.New("saved view id and label are required")
	}
	if view.ProjectID == "" {
		return SavedView{}, errors.New("saved view project is required")
	}
	switch view.Kind {
	case "work_item":
		if view.WIQL == "" || len(view.WIQL) > 10_000 {
			return SavedView{}, errors.New("a work item view requires WIQL")
		}
		if view.Top == 0 {
			view.Top = 50
		}
		if view.Top < 1 || view.Top > 200 {
			return SavedView{}, errors.New("the saved view result limit is invalid")
		}
	case "pull_request":
		if view.RepositoryID == "" {
			return SavedView{}, errors.New("a pull request view requires a repository")
		}
	default:
		return SavedView{}, errors.New("the saved view kind is invalid")
	}
	if view.CreatedAt.IsZero() {
		view.CreatedAt = now.UTC()
	}
	return view, nil
}

type settingsOverrides map[string]json.RawMessage

// Settings returns the effective workspace presets.
func (c *Connections) Settings(ctx context.Context, workspaceID string) (WorkspaceSettings, error) {
	overrides := settingsOverrides{}
	if _, err := c.store.LoadState(ctx, workspaceID, settingsStateKey, &overrides); err != nil {
		return WorkspaceSettings{}, err
	}
	settings := WorkspaceSettings{
		WorkItemQueries: DefaultWorkItemQueryPresets(), PullRequestQueries: DefaultPullRequestQueryPresets(),
		WorkItemActions: DefaultWorkItemActionPresets(), PullRequestActions: DefaultPullRequestActionPresets(),
	}
	decodeOverride(overrides, "workItemQueries", &settings.WorkItemQueries)
	decodeOverride(overrides, "pullRequestQueries", &settings.PullRequestQueries)
	decodeOverride(overrides, "workItemActions", &settings.WorkItemActions)
	decodeOverride(overrides, "pullRequestActions", &settings.PullRequestActions)
	return settings, nil
}

func decodeOverride[T any](overrides settingsOverrides, key string, target *[]T) {
	raw, ok := overrides[key]
	if !ok || len(raw) == 0 {
		return
	}
	var value []T
	if json.Unmarshal(raw, &value) == nil && len(value) > 0 {
		*target = value
	}
}

// PatchSettings applies the keys present in patch: an array replaces the
// presets, null restores the defaults, and an absent key is unchanged.
func (c *Connections) PatchSettings(ctx context.Context, workspaceID string, patch map[string]json.RawMessage) (WorkspaceSettings, error) {
	unlock := c.locks.lock(workspaceID)
	overrides := settingsOverrides{}
	_, err := c.store.LoadState(ctx, workspaceID, settingsStateKey, &overrides)
	if err == nil {
		err = applySettingsPatch(overrides, patch)
	}
	if err == nil {
		err = c.store.SaveState(ctx, workspaceID, settingsStateKey, overrides)
	}
	unlock()
	if err != nil {
		return WorkspaceSettings{}, err
	}
	return c.Settings(ctx, workspaceID)
}

func applySettingsPatch(overrides settingsOverrides, patch map[string]json.RawMessage) error {
	for key, raw := range patch {
		var err error
		switch key {
		case "workItemQueries", "pullRequestQueries":
			err = patchPresets(overrides, key, raw, normalizeQueryPresets)
		case "workItemActions", "pullRequestActions":
			err = patchPresets(overrides, key, raw, normalizeActionPresets)
		default:
			err = fmt.Errorf("unknown workspace setting %q", key)
		}
		if err != nil {
			return invalidArgument(err)
		}
	}
	return nil
}

func patchPresets[T any](overrides settingsOverrides, key string, raw json.RawMessage, normalize func([]T) []T) error {
	if string(raw) == "null" {
		delete(overrides, key)
		return nil
	}
	var values []T
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("%s must be a list", key)
	}
	normalized := normalize(values)
	if len(normalized) == 0 {
		return fmt.Errorf("%s must include a valid preset", key)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	overrides[key] = encoded
	return nil
}

func normalizeQueryPresets(values []QueryPreset) []QueryPreset {
	result := make([]QueryPreset, 0, len(values))
	for _, value := range values {
		value.ID, value.Label, value.Group = strings.TrimSpace(value.ID), strings.TrimSpace(value.Label), strings.TrimSpace(value.Group)
		if value.Label == "" || len(value.Filters) == 0 {
			continue
		}
		if value.ID == "" {
			value.ID = newID()
		}
		result = append(result, value)
	}
	return result
}

func normalizeActionPresets(values []ActionPreset) []ActionPreset {
	result := make([]ActionPreset, 0, len(values))
	for _, value := range values {
		value.ID, value.Label, value.Hint = strings.TrimSpace(value.ID), strings.TrimSpace(value.Label), strings.TrimSpace(value.Hint)
		value.Icon, value.PromptTemplate = strings.TrimSpace(value.Icon), strings.TrimSpace(value.PromptTemplate)
		if value.Label == "" || value.PromptTemplate == "" {
			continue
		}
		if value.ID == "" {
			value.ID = newID()
		}
		result = append(result, value)
	}
	return result
}

func newID() string {
	buffer := make([]byte, 8)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}
