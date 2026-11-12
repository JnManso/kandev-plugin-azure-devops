package plugin

import (
	"context"
	"errors"
	"sort"

	"github.com/kdlbs/kandev/pluginsdk"
)

const maxDescribedWorkspaces = 200

// WorkspaceSummary names a workspace and whether it has a connection. It is
// the copy-source picker's data; no connection details are returned.
type WorkspaceSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// describeWorkspaces returns summaries for the requested workspace IDs that
// exist. The browser supplies only the IDs it already knows.
func describeWorkspaces(ctx context.Context, connections *Connections, workspaces WorkspaceLister, ids []string) ([]WorkspaceSummary, error) {
	if len(ids) > maxDescribedWorkspaces {
		return nil, invalidArgument(errors.New("too many workspaces requested"))
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	summaries := make([]WorkspaceSummary, 0, len(ids))
	cursor := ""
	for len(wanted) > 0 {
		page, info, err := workspaces.List(ctx, pluginsdk.Page{Limit: workspacePageSize, Cursor: cursor})
		if err != nil {
			return nil, unavailable(err)
		}
		for _, workspace := range page {
			if !wanted[workspace.ID] {
				continue
			}
			delete(wanted, workspace.ID)
			_, configured, err := connections.store.Connection(ctx, workspace.ID)
			if err != nil {
				return nil, unavailable(err)
			}
			summaries = append(summaries, WorkspaceSummary{ID: workspace.ID, Name: workspace.Name, Configured: configured})
		}
		if info == nil || !info.HasMore || info.NextCursor == "" || info.NextCursor == cursor {
			break
		}
		cursor = info.NextCursor
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}
