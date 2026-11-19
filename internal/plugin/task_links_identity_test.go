package plugin

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func TestNativeCreatePullRequestSupportsARepositoryRecordedWithoutACloneURL(t *testing.T) {
	h := linksHarness(t)
	h.host.AddRepository(pluginsdk.Repository{
		ID: "repo-hostonly", WorkspaceID: "ws-1", ProviderID: "azure_devops", ProviderHost: "https://dev.azure.com",
		OwnerOrProject: "Platform Team", ProviderName: "api", ProviderRepositoryID: "guid-api",
	})
	h.host.AddTask(pluginsdk.Task{ID: "task-2", WorkspaceID: "ws-1", Repositories: []pluginsdk.TaskRepository{{RepositoryID: "repo-hostonly", BaseBranch: "main"}}})
	verified := pluginsdk.VerifiedActionContext{WorkspaceID: "ws-1", TaskID: "task-2", RepositoryID: "repo-hostonly", SessionID: "s-1", HeadBranch: "feature/z"}
	status, body := h.actionWith(verified, "pull_requests.create", map[string]any{"title": "Add Z"})
	if status != 200 || body["linked"] != true {
		t.Fatalf("create = %d %v", status, body)
	}
	if created := h.azure.createdPRs[0]; created.SourceBranch != "feature/z" || created.TargetBranch != "main" {
		t.Fatalf("created request = %+v", created)
	}
}

func TestCreatePullRequestAttachesOnlyWorkItemsOfTheConnectedOrganization(t *testing.T) {
	h := linksHarness(t)
	if status, _ := h.taskAction("work_items.link", map[string]any{"reference": "101"}); status != 200 {
		t.Fatalf("link status = %d", status)
	}
	services, _ := h.runtime.ready()
	ctx := context.Background()
	for _, link := range []WorkItemLink{{Organization: "other", WorkItemID: 7}, {WorkItemID: 8}} {
		if _, err := services.links.AddWorkItem(ctx, "ws-1", "task-1", link); err != nil {
			t.Fatal(err)
		}
	}
	verified := pluginsdk.VerifiedActionContext{WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", SessionID: "s-1", HeadBranch: "feature/y"}
	if status, body := h.actionWith(verified, "pull_requests.create", map[string]any{"title": "Add Y"}); status != 200 {
		t.Fatalf("create = %d %v", status, body)
	}
	if ids := h.azure.createdPRs[0].WorkItemIDs; len(ids) != 1 || ids[0] != 101 {
		t.Fatalf("attached work items = %v, want [101]", ids)
	}
	links, _ := services.links.PullRequests(ctx, "ws-1", "task-1")
	if len(links) != 1 || links[0].Organization != "acme" {
		t.Fatalf("pull request links = %+v", links)
	}
}
