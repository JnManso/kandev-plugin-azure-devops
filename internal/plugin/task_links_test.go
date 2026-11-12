package plugin

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/azure"
)

const linkedPRURL = "https://dev.azure.com/acme/Platform%20Team/_git/api/pullrequest/42"

func linksHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.connect("ws-1")
	h.host.AddRepository(pluginsdk.Repository{
		ID: "repo-1", WorkspaceID: "ws-1", ProviderID: "azure_devops", OwnerOrProject: "Platform Team",
		ProviderRepositoryID: "guid-api", RemoteURL: "https://dev.azure.com/acme/Platform%20Team/_git/api",
	})
	h.host.AddRepository(pluginsdk.Repository{ID: "repo-gh", WorkspaceID: "ws-1", ProviderID: "github", RemoteURL: "https://github.com/acme/api"})
	h.host.AddTask(pluginsdk.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Ship links", Repositories: []pluginsdk.TaskRepository{
		{RepositoryID: "repo-gh"}, {RepositoryID: "repo-1", BaseBranch: "main"},
	}})
	h.azure.pullRequests = map[int]azure.PullRequest{42: {
		ID: 42, Title: "Rotate credentials", Status: "active", SourceBranch: "feature/x", TargetBranch: "main",
		ProjectID: "p1", ProjectName: "Platform Team", RepositoryID: "guid-api", RepositoryName: "api", WebURL: linkedPRURL,
	}}
	return h
}

func (h *harness) taskAction(key string, body any) (int, map[string]any) {
	h.t.Helper()
	return h.actionWith(pluginsdk.VerifiedActionContext{WorkspaceID: "ws-1", TaskID: "task-1"}, key, body)
}

func TestLinkPullRequestByURLNumberAndShortForm(t *testing.T) {
	for _, reference := range []string{linkedPRURL, "!42", "42", "Platform Team/api!42"} {
		t.Run(reference, func(t *testing.T) {
			h := linksHarness(t)
			status, body := h.taskAction("pull_requests.link", map[string]any{"reference": reference})
			if status != 200 || field(body, "pullRequest", "repositoryId") != "guid-api" || field(body, "pullRequest", "pullRequestId") != float64(42) {
				t.Fatalf("link status = %d body = %v", status, body)
			}
			status, body = h.taskAction("task_links.get", nil)
			prs, _ := body["pullRequests"].([]any)
			if status != 200 || len(prs) != 1 {
				t.Fatalf("task_links.get = %d %v", status, body)
			}
			if got := field(prs[0], "taskStatus", "pipelineState"); got != "pending" {
				t.Fatalf("pipeline state = %v, want pending (running blocking policy)", got)
			}
			if got := field(prs[0], "taskStatus", "review", "state"); got != "approved" {
				t.Fatalf("review state = %v", got)
			}
		})
	}
}

func TestLinkPullRequestRejectsAnotherOrganizationAndUnknownNumbers(t *testing.T) {
	h := linksHarness(t)
	status, body := h.taskAction("pull_requests.link", map[string]any{"reference": "https://dev.azure.com/other/P/_git/r/pullrequest/42"})
	if status != 400 {
		t.Fatalf("other organization status = %d body = %v", status, body)
	}
	status, body = h.taskAction("pull_requests.link", map[string]any{"reference": "!77"})
	if status != 404 {
		t.Fatalf("unknown number status = %d body = %v", status, body)
	}
	status, _ = h.taskAction("pull_requests.link", map[string]any{"reference": "not a reference"})
	if status != 400 {
		t.Fatalf("invalid reference status = %d", status)
	}
}

func TestUnlinkPullRequestAndWorkItem(t *testing.T) {
	h := linksHarness(t)
	h.taskAction("pull_requests.link", map[string]any{"reference": "!42"})
	if status, body := h.taskAction("work_items.link", map[string]any{"reference": "#101"}); status != 200 || field(body, "workItem", "workItemId") != float64(101) {
		t.Fatalf("work item link = %d %v", status, body)
	}
	_, workspace := h.action("ws-1", "task_links.workspace", nil)
	if field(workspace, "pullRequests", "task-1") == nil || field(workspace, "workItems", "task-1") == nil ||
		field(workspace, "tasks", "task-1", "title") != "Ship links" {
		t.Fatalf("workspace links = %v", workspace)
	}
	h.taskAction("pull_requests.unlink", map[string]any{"repositoryId": "guid-api", "pullRequestId": 42})
	h.taskAction("work_items.unlink", map[string]any{"workItemId": 101})
	_, body := h.taskAction("task_links.get", nil)
	if prs, _ := body["pullRequests"].([]any); len(prs) != 0 {
		t.Fatalf("pull requests after unlink = %v", prs)
	}
	if items, _ := body["workItems"].([]any); len(items) != 0 {
		t.Fatalf("work items after unlink = %v", items)
	}
	if keys := h.host.StateKeys("workspace", "ws-1"); containsKeyPrefix(keys, "task.task-1.") {
		t.Fatalf("empty link records remain: %v", keys)
	}
}

func containsKeyPrefix(keys []string, prefix string) bool {
	for _, key := range keys {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func TestTaskLinkActionsRequireAVerifiedTask(t *testing.T) {
	h := linksHarness(t)
	status, _ := h.action("ws-1", "task_links.get", nil)
	if status != 400 {
		t.Fatalf("workspace-only context status = %d", status)
	}
}

func TestNativeCreatePullRequestUsesTheVerifiedBranchAndLinks(t *testing.T) {
	h := linksHarness(t)
	h.taskAction("work_items.link", map[string]any{"reference": "101"})
	verified := pluginsdk.VerifiedActionContext{WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", SessionID: "s-1", HeadBranch: "feature/y"}
	status, body := h.actionWith(verified, "pull_requests.create", map[string]any{"title": "Add Y", "description": "Body", "draft": true})
	if status != 200 || body["linked"] != true || body["url"] == "" {
		t.Fatalf("create = %d %v", status, body)
	}
	created := h.azure.createdPRs[0]
	if created.SourceBranch != "feature/y" || created.TargetBranch != "main" || !created.IsDraft || len(created.WorkItemIDs) != 1 {
		t.Fatalf("created request = %+v", created)
	}
	// A retry for the same branch reuses the open pull request.
	status, again := h.actionWith(verified, "pull_requests.create", map[string]any{"title": "Add Y"})
	if status != 200 || again["number"] != body["number"] || len(h.azure.createdPRs) != 1 {
		t.Fatalf("retry = %d %v, created %d", status, again, len(h.azure.createdPRs))
	}
}

func TestNativeCreatePullRequestRejectsANonAzureRepository(t *testing.T) {
	h := linksHarness(t)
	verified := pluginsdk.VerifiedActionContext{WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-gh", SessionID: "s-1", HeadBranch: "x"}
	if status, _ := h.actionWith(verified, "pull_requests.create", map[string]any{"title": "X"}); status != 400 {
		t.Fatalf("status = %d", status)
	}
}

func TestTaskDeletedEventAndReconciliationRemoveLinks(t *testing.T) {
	h := linksHarness(t)
	h.taskAction("pull_requests.link", map[string]any{"reference": "!42"})
	err := h.runtime.OnEvent(context.Background(), &pluginsdk.Event{EventType: TaskDeletedEvent, Payload: map[string]any{"task_id": "task-1", "workspace_id": "ws-1"}})
	if err != nil || containsKeyPrefix(h.host.StateKeys("workspace", "ws-1"), "task.task-1.") {
		t.Fatalf("event err = %v, keys = %v", err, h.host.StateKeys("workspace", "ws-1"))
	}

	h.taskAction("pull_requests.link", map[string]any{"reference": "!42"})
	h.host.AddWorkspace("ws-1")
	h.host.RemoveTask("task-1")
	services, _ := h.runtime.ready()
	services.links.reconcileDeletedTasks(context.Background(), h.host.Workspaces(), h.host.Tasks())
	if containsKeyPrefix(h.host.StateKeys("workspace", "ws-1"), "task.task-1.") {
		t.Fatalf("reconciliation kept links: %v", h.host.StateKeys("workspace", "ws-1"))
	}
}
