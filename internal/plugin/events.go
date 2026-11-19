package plugin

import (
	"context"
	"log"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TaskDeletedEvent is the host event that removes a task's links.
const TaskDeletedEvent = "task.deleted"

// handleTaskDeleted removes the links of a deleted task. It is idempotent.
func (l *Links) handleTaskDeleted(ctx context.Context, event *pluginsdk.Event) error {
	if event == nil || event.EventType != TaskDeletedEvent {
		return nil
	}
	taskID, _ := event.Payload["task_id"].(string)
	workspaceID, _ := event.Payload["workspace_id"].(string)
	if workspaceID == "" {
		workspaceID = event.WorkspaceID
	}
	if taskID == "" || workspaceID == "" {
		return nil
	}
	return l.DeleteTask(ctx, workspaceID, taskID)
}

// reconcileDeletedTasks drops links of tasks deleted while the plugin was not
// running. Only a definite not-found answer removes links.
func (l *Links) reconcileDeletedTasks(ctx context.Context, workspaces WorkspaceLister, tasks TaskReader) {
	ids, err := listWorkspaceIDs(ctx, workspaces)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("azure devops: link reconciliation skipped: list workspaces: %v", err)
		}
		return
	}
	for _, workspaceID := range ids {
		taskIDs, err := l.LinkedTaskIDs(ctx, workspaceID)
		if err != nil {
			continue
		}
		for _, taskID := range taskIDs {
			if ctx.Err() != nil {
				return
			}
			task, err := tasks.Get(ctx, taskID)
			if status.Code(err) == codes.NotFound || (err == nil && task == nil) {
				if err := l.DeleteTask(ctx, workspaceID, taskID); err != nil {
					log.Printf("azure devops: remove links of deleted task: %v", err)
				}
			}
		}
	}
}
