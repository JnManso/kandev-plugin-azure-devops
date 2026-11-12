package hosttest

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AddTask registers a task for Tasks().Get and Tasks().List.
func (h *Host) AddTask(task pluginsdk.Task) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tasks == nil {
		h.tasks = map[string]pluginsdk.Task{}
	}
	h.tasks[task.ID] = task
	h.taskOrder = append(h.taskOrder, task.ID)
}

// RemoveTask deletes a task, as Kandev does when a user deletes it.
func (h *Host) RemoveTask(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.tasks, id)
}

// CreatedTasks returns the inputs of every Tasks().Create call.
func (h *Host) CreatedTasks() []pluginsdk.CreateTaskInput {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]pluginsdk.CreateTaskInput(nil), h.createdTasks...)
}

func (h *Host) Tasks() pluginsdk.TaskReader { return taskReader{h} }

type taskReader struct{ host *Host }

func (r taskReader) Get(_ context.Context, id string) (*pluginsdk.Task, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	task, found := r.host.tasks[id]
	if !found {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	return &task, nil
}

func (r taskReader) List(_ context.Context, filter pluginsdk.TaskFilter, _ pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	var result []pluginsdk.Task
	for _, id := range r.host.taskOrder {
		task, found := r.host.tasks[id]
		if !found || (len(filter.WorkspaceIDs) > 0 && !contains(filter.WorkspaceIDs, task.WorkspaceID)) {
			continue
		}
		result = append(result, task)
	}
	return result, &pluginsdk.PageInfo{}, nil
}

func (r taskReader) Create(_ context.Context, in pluginsdk.CreateTaskInput) (*pluginsdk.Task, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	if r.host.FailCreateTask != nil {
		return nil, r.host.FailCreateTask
	}
	if utf8.RuneCountInString(in.Title) > kandevTaskTitleMaxLength {
		return nil, errors.New("hosttest: task title is too long: task titles must be 60 characters or fewer")
	}
	if r.host.tasks == nil {
		r.host.tasks = map[string]pluginsdk.Task{}
	}
	r.host.createdTasks = append(r.host.createdTasks, in)
	id := "task-created-" + strconv.Itoa(len(r.host.createdTasks))
	repositories := make([]pluginsdk.TaskRepository, 0, len(in.Repositories))
	for _, repository := range in.Repositories {
		repositories = append(repositories, pluginsdk.TaskRepository{RepositoryID: repository.RepositoryID})
	}
	task := pluginsdk.Task{ID: id, WorkspaceID: in.WorkspaceID, WorkflowID: in.WorkflowID, Title: in.Title,
		Description: in.Description, Metadata: in.Metadata, Repositories: repositories}
	r.host.tasks[id] = task
	r.host.taskOrder = append(r.host.taskOrder, id)
	return &task, nil
}

func (r taskReader) Update(context.Context, pluginsdk.UpdateTaskInput) (*pluginsdk.Task, error) {
	return nil, errors.New("hosttest: task update is not supported")
}

func (r taskReader) Move(context.Context, pluginsdk.MoveTaskInput) (*pluginsdk.MoveTaskOutcome, error) {
	return nil, fmt.Errorf("hosttest: task move is not supported")
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// kandevTaskTitleMaxLength mirrors the host's task title limit.
const kandevTaskTitleMaxLength = 60
