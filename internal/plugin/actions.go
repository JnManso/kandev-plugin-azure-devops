package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kdlbs/kandev/pluginsdk"
)

const maxActionBodyBytes = 64 << 10

func decodeBody(body []byte, target any) error {
	if len(body) > maxActionBodyBytes {
		return invalidArgument(errors.New("request body is too large"))
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return invalidArgument(errors.New("request body is not valid JSON"))
	}
	return nil
}

// handleConnectionAction serves the workspace-scoped connection, settings,
// and browse actions.
func (r *services) handleConnectionAction(ctx context.Context, request *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	workspaceID := request.Context.WorkspaceID
	if workspaceID == "" {
		return nil, pluginsdk.CategorizeActionError(pluginsdk.ActionErrorPermissionDenied, errors.New("a verified workspace is required"))
	}
	switch request.ActionKey {
	case "connection.get":
		view, err := r.connections.Get(ctx, workspaceID)
		if err != nil {
			return nil, unavailable(err)
		}
		return jsonAction(map[string]any{"connection": view})
	case "connection.save":
		var input ConnectionInput
		if err := decodeBody(request.Body, &input); err != nil {
			return nil, err
		}
		view, err := r.connections.Save(ctx, workspaceID, input)
		if err != nil {
			return nil, err
		}
		return jsonAction(map[string]any{"connection": view})
	case "connection.test":
		var input ConnectionInput
		if err := decodeBody(request.Body, &input); err != nil {
			return nil, err
		}
		result, err := r.connections.Test(ctx, workspaceID, input)
		if err != nil {
			return nil, err
		}
		return jsonAction(map[string]any{"result": result})
	case "connection.copy":
		var input struct {
			SourceWorkspaceID string `json:"sourceWorkspaceId"`
		}
		if err := decodeBody(request.Body, &input); err != nil {
			return nil, err
		}
		view, err := r.connections.Copy(ctx, workspaceID, input.SourceWorkspaceID)
		if err != nil {
			return nil, err
		}
		return jsonAction(map[string]any{"connection": view})
	case "connection.delete":
		if err := r.connections.Delete(ctx, workspaceID); err != nil {
			return nil, unavailable(err)
		}
		return jsonAction(map[string]any{"deleted": true})
	case "workspaces.describe":
		var input struct {
			WorkspaceIDs []string `json:"workspaceIds"`
		}
		if err := decodeBody(request.Body, &input); err != nil {
			return nil, err
		}
		summaries, err := describeWorkspaces(ctx, r.connections, r.workspaces, input.WorkspaceIDs)
		if err != nil {
			return nil, err
		}
		return jsonAction(map[string]any{"workspaces": summaries})
	case "oauth.availability":
		return jsonAction(r.connections.Availability(ctx))
	case "oauth.start":
		var input OAuthStartInput
		if err := decodeBody(request.Body, &input); err != nil {
			return nil, err
		}
		start, err := r.connections.StartOAuth(ctx, workspaceID, request.Context.ActorID, input)
		if err != nil {
			return nil, err
		}
		return jsonAction(start)
	case "oauth.complete":
		var input struct {
			Code  string `json:"code"`
			State string `json:"state"`
		}
		if err := decodeBody(request.Body, &input); err != nil {
			return nil, err
		}
		view, err := r.connections.CompleteOAuth(ctx, workspaceID, request.Context.ActorID, input.Code, input.State)
		if err != nil {
			return nil, err
		}
		return jsonAction(map[string]any{"connection": view})
	case "oauth.sign_out":
		view, err := r.connections.SignOut(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		return jsonAction(map[string]any{"connection": view})
	case "views.get":
		views, err := r.connections.Views(ctx, workspaceID)
		if err != nil {
			return nil, unavailable(err)
		}
		return jsonAction(map[string]any{"views": views})
	case "views.save":
		var input struct {
			Views []SavedView `json:"views"`
		}
		if err := decodeBody(request.Body, &input); err != nil {
			return nil, err
		}
		views, err := r.connections.SaveViews(ctx, workspaceID, input.Views)
		if err != nil {
			return nil, err
		}
		return jsonAction(map[string]any{"views": views})
	case "workspace_settings.get":
		settings, err := r.connections.Settings(ctx, workspaceID)
		if err != nil {
			return nil, unavailable(err)
		}
		return jsonAction(settings)
	case "workspace_settings.patch":
		var patch map[string]json.RawMessage
		if err := decodeBody(request.Body, &patch); err != nil {
			return nil, err
		}
		settings, err := r.connections.PatchSettings(ctx, workspaceID, patch)
		if err != nil {
			return nil, err
		}
		return jsonAction(settings)
	case "projects.list":
		client, _, err := r.connections.Client(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return nil, repositoryReadError(err)
		}
		return jsonAction(map[string]any{"projects": projects})
	default:
		if result, found, err := r.watches.Handle(ctx, workspaceID, request.ActionKey, request.Body); found {
			if err != nil {
				return nil, err
			}
			return jsonAction(result)
		}
		result, found, err := r.browse.Handle(ctx, workspaceID, request.ActionKey, request.Body)
		if !found {
			return nil, notFound(errors.New("unsupported action"))
		}
		if err != nil {
			return nil, err
		}
		return jsonAction(result)
	}
}
