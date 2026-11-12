package plugin

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kdlbs/kandev/pluginsdk"
)

func invalidArgument(err error) error {
	return pluginsdk.CategorizeActionError(pluginsdk.ActionErrorInvalidArgument, err)
}

func notFound(err error) error {
	return pluginsdk.CategorizeActionError(pluginsdk.ActionErrorNotFound, err)
}

// failedPrecondition marks a workspace that is not connected or must
// reconnect. Kandev has no 412 category; 409 tells the UI to fix settings.
func failedPrecondition(err error) error {
	return pluginsdk.CategorizeActionError(pluginsdk.ActionErrorConflict, err)
}

func unavailable(err error) error {
	return pluginsdk.CategorizeActionError(pluginsdk.ActionErrorUnavailable, err)
}

// safeMessages are the only error texts returned to the browser verbatim.
var safeMessages = map[pluginsdk.ActionErrorCode]bool{
	pluginsdk.ActionErrorInvalidArgument: true,
	pluginsdk.ActionErrorNotFound:        true,
	pluginsdk.ActionErrorConflict:        true,
}

// actionFailure converts err into a JSON action response. Cancellation is
// returned as an error so the host can abandon the request.
func actionFailure(err error) (*pluginsdk.PluginActionResponse, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	code := pluginsdk.ActionErrorUpstream
	message := "Azure DevOps request failed"
	var actionErr *pluginsdk.ActionError
	if errors.As(err, &actionErr) {
		code = actionErr.Code
		if safeMessages[code] && actionErr.Err != nil {
			message = actionErr.Err.Error()
		} else if code == pluginsdk.ActionErrorUnavailable {
			message = "Azure DevOps is unavailable"
		}
	}
	body, _ := json.Marshal(map[string]string{"error": message, "code": string(code)})
	return &pluginsdk.PluginActionResponse{
		Status:  pluginsdk.ActionErrorHTTPStatus(code),
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	}, nil
}

func jsonAction(value any) (*pluginsdk.PluginActionResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &pluginsdk.PluginActionResponse{
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	}, nil
}
