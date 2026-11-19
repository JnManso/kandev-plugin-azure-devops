package plugin

import (
	"context"
	"errors"
	"sort"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/watches"
)

const optionsPageSize = 100

// WatchHost is the host data the watch editor lists.
type WatchHost interface {
	Workflows() pluginsdk.WorkflowReader
	AgentProfiles() pluginsdk.AgentProfileReader
	Repositories() pluginsdk.RepositoryReader
}

// Watches serves the watch settings actions.
type Watches struct {
	service          *watches.Service
	host             WatchHost
	executorProfiles func() (pluginsdk.ExecutorProfileReader, bool)
}

type watchActionInput struct {
	ID string `json:"id"`
	watches.Input
}

// Option is one selectable value in the watch editor.
type Option struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Steps []Option `json:"steps,omitempty"`
}

// Handle serves watch actions. found is false for other keys.
func (w *Watches) Handle(ctx context.Context, workspaceID, key string, body []byte) (any, bool, error) {
	switch key {
	case "watches.list", "watches.options", "watches.create", "watches.update", "watches.delete",
		"watches.trigger", "watches.preview_reset", "watches.reset":
	default:
		return nil, false, nil
	}
	if workspaceID == "" {
		return nil, true, invalidArgument(errors.New("a workspace is required"))
	}
	var input watchActionInput
	if err := decodeBody(body, &input); err != nil {
		return nil, true, err
	}
	result, err := w.handle(ctx, workspaceID, key, input)
	return result, true, watchError(err)
}

func (w *Watches) handle(ctx context.Context, workspaceID, key string, input watchActionInput) (any, error) {
	switch key {
	case "watches.list":
		list, err := w.service.List(ctx, workspaceID)
		return map[string]any{"watches": list}, err
	case "watches.options":
		return w.options(ctx, workspaceID)
	case "watches.create":
		watch, err := w.service.Create(ctx, workspaceID, input.Input)
		return map[string]any{"watch": watch}, err
	case "watches.update":
		watch, err := w.service.Update(ctx, workspaceID, input.ID, input.Input)
		return map[string]any{"watch": watch}, err
	case "watches.delete":
		return map[string]any{"deleted": true}, w.service.Delete(ctx, workspaceID, input.ID)
	case "watches.trigger":
		created, err := w.service.Trigger(ctx, workspaceID, input.ID)
		if err != nil && !errors.Is(err, watches.ErrNotFound) {
			return map[string]any{"matched": created, "error": err.Error()}, nil
		}
		return map[string]any{"matched": created}, err
	case "watches.preview_reset":
		count, err := w.service.PreviewReset(ctx, workspaceID, input.ID)
		return map[string]any{"count": count}, err
	default: // watches.reset
		count, err := w.service.Reset(ctx, workspaceID, input.ID)
		return map[string]any{"count": count}, err
	}
}

func watchError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, watches.ErrNotFound):
		return notFound(err)
	case errors.Is(err, watches.ErrInvalid):
		return invalidArgument(err)
	default:
		return unavailable(err)
	}
}

// options lists the workflows, steps, profiles, and repositories the plugin
// can create tasks with. Kandev returns only shared profiles to plugins.
func (w *Watches) options(ctx context.Context, workspaceID string) (any, error) {
	workflows, err := w.workflowOptions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	agents, err := pageAll(ctx, func(page pluginsdk.Page) ([]Option, *pluginsdk.PageInfo, error) {
		profiles, info, err := w.host.AgentProfiles().List(ctx, page)
		options := make([]Option, 0, len(profiles))
		for _, profile := range profiles {
			options = append(options, Option{ID: profile.ID, Name: firstNonEmpty(profile.DisplayName, profile.Name, profile.ID)})
		}
		return options, info, err
	})
	if err != nil {
		return nil, err
	}
	executors := []Option{}
	if reader, ok := w.executorProfiles(); ok {
		executors, err = pageAll(ctx, func(page pluginsdk.Page) ([]Option, *pluginsdk.PageInfo, error) {
			profiles, info, err := reader.List(ctx, page)
			options := make([]Option, 0, len(profiles))
			for _, profile := range profiles {
				options = append(options, Option{ID: profile.ID, Name: firstNonEmpty(profile.DisplayName, profile.ID)})
			}
			return options, info, err
		})
		if err != nil {
			return nil, err
		}
	}
	repositories, err := pageAll(ctx, func(page pluginsdk.Page) ([]Option, *pluginsdk.PageInfo, error) {
		rows, info, err := w.host.Repositories().List(ctx, workspaceID, page)
		options := make([]Option, 0, len(rows))
		for _, row := range rows {
			options = append(options, Option{ID: row.ID, Name: firstNonEmpty(row.Name, row.ID)})
		}
		return options, info, err
	})
	if err != nil {
		return nil, err
	}
	sortOptions(agents, executors, repositories)
	return map[string]any{
		"workflows": workflows, "agentProfiles": agents, "executorProfiles": executors, "repositories": repositories,
	}, nil
}

func (w *Watches) workflowOptions(ctx context.Context, workspaceID string) ([]Option, error) {
	workflows, err := pageAll(ctx, func(page pluginsdk.Page) ([]pluginsdk.Workflow, *pluginsdk.PageInfo, error) {
		return w.host.Workflows().List(ctx, workspaceID, page)
	})
	if err != nil {
		return nil, err
	}
	options := make([]Option, 0, len(workflows))
	for _, workflow := range workflows {
		steps, err := w.host.Workflows().ListSteps(ctx, workflow.ID)
		if err != nil {
			return nil, err
		}
		option := Option{ID: workflow.ID, Name: workflow.Name, Steps: []Option{}}
		for _, step := range steps {
			option.Steps = append(option.Steps, Option{ID: step.ID, Name: step.Name})
		}
		options = append(options, option)
	}
	return options, nil
}

func pageAll[T any](ctx context.Context, list func(pluginsdk.Page) ([]T, *pluginsdk.PageInfo, error)) ([]T, error) {
	result := []T{}
	cursor := ""
	for ctx.Err() == nil {
		items, info, err := list(pluginsdk.Page{Limit: optionsPageSize, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if info == nil || !info.HasMore || info.NextCursor == "" || info.NextCursor == cursor {
			break
		}
		cursor = info.NextCursor
	}
	return result, ctx.Err()
}

func sortOptions(groups ...[]Option) {
	for _, group := range groups {
		sort.SliceStable(group, func(left, right int) bool { return group[left].Name < group[right].Name })
	}
}
