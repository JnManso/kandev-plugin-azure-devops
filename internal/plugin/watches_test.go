package plugin

import (
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func watchHarness(t *testing.T) *harness {
	t.Helper()
	h := linksHarness(t)
	h.host.AddWorkflow(pluginsdk.Workflow{ID: "wf", WorkspaceID: "ws-1", Name: "Delivery"},
		pluginsdk.WorkflowStep{ID: "todo", WorkflowID: "wf", Name: "To do"})
	h.host.AddAgentProfile(pluginsdk.AgentProfile{ID: "agent", DisplayName: "Claude"})
	h.host.AddExecutorProfile(pluginsdk.ExecutorProfile{ID: "exec", DisplayName: "Worktree"})
	return h
}

func TestWatchOptionsListWorkflowsProfilesAndRepositories(t *testing.T) {
	h := watchHarness(t)
	status, body := h.action("ws-1", "watches.options", nil)
	if status != 200 {
		t.Fatalf("options = %d %v", status, body)
	}
	workflows, _ := body["workflows"].([]any)
	if len(workflows) != 1 || field(workflows[0], "name") != "Delivery" {
		t.Fatalf("workflows = %v", body["workflows"])
	}
	if steps, _ := field(workflows[0], "steps").([]any); len(steps) != 1 || field(steps[0], "id") != "todo" {
		t.Fatalf("steps = %v", field(workflows[0], "steps"))
	}
	for _, key := range []string{"agentProfiles", "executorProfiles", "repositories"} {
		if list, _ := body[key].([]any); len(list) == 0 {
			t.Fatalf("%s is empty: %v", key, body)
		}
	}
}

func TestWatchActionsCreateTriggerAndReset(t *testing.T) {
	h := watchHarness(t)
	status, body := h.action("ws-1", "watches.create", map[string]any{
		"kind": "work_item", "projectId": "Platform Team", "wiql": "SELECT [System.Id] FROM WorkItems",
		"workflowId": "wf", "workflowStepId": "todo", "agentProfileId": "agent", "executorProfileId": "exec",
		"prompt": "Implement {{work_item.title}}",
	})
	id, _ := field(body, "watch", "id").(string)
	if status != 200 || id == "" {
		t.Fatalf("create = %d %v", status, body)
	}
	status, body = h.action("ws-1", "watches.trigger", map[string]any{"id": id})
	if status != 200 || body["matched"] == float64(0) || body["error"] != nil {
		t.Fatalf("trigger = %d %v", status, body)
	}
	created := h.host.CreatedTasks()
	if len(created) == 0 || created[0].Metadata["azure_devops_watch_id"] != id {
		t.Fatalf("created tasks = %+v", created)
	}
	if status, body = h.action("ws-1", "watches.preview_reset", map[string]any{"id": id}); status != 200 || body["count"] == float64(0) {
		t.Fatalf("preview = %d %v", status, body)
	}
	if status, _ = h.action("ws-1", "watches.reset", map[string]any{"id": id}); status != 200 {
		t.Fatalf("reset = %d", status)
	}
	_, list := h.action("ws-1", "watches.list", nil)
	if watches, _ := list["watches"].([]any); len(watches) != 1 || field(watches[0], "generation") != float64(2) {
		t.Fatalf("list = %v", list)
	}
	if status, _ = h.action("ws-1", "watches.delete", map[string]any{"id": id}); status != 200 {
		t.Fatalf("delete = %d", status)
	}
	if status, _ = h.action("ws-1", "watches.update", map[string]any{"id": id, "enabled": true}); status != 404 {
		t.Fatalf("update after delete = %d", status)
	}
}

func TestWatchCreateRejectsAnIncompleteWatch(t *testing.T) {
	h := watchHarness(t)
	if status, _ := h.action("ws-1", "watches.create", map[string]any{"kind": "work_item"}); status != 400 {
		t.Fatalf("status = %d", status)
	}
}
