package plugin

import (
	"context"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
)

// HealthInterval is how often every connected workspace is probed.
const HealthInterval = 90 * time.Second

const workspacePageSize = 100

// WorkspaceLister reads workspaces through the Host.
type WorkspaceLister interface {
	List(ctx context.Context, page pluginsdk.Page) ([]pluginsdk.Workspace, *pluginsdk.PageInfo, error)
}

// HealthPoller records connection health for every configured workspace.
type HealthPoller struct {
	connections *Connections
	workspaces  WorkspaceLister
	interval    time.Duration
}

// NewHealthPoller returns a poller that runs every HealthInterval.
func NewHealthPoller(connections *Connections, workspaces WorkspaceLister) *HealthPoller {
	return &HealthPoller{connections: connections, workspaces: workspaces, interval: HealthInterval}
}

// Run probes immediately and then on every tick until ctx is cancelled.
func (p *HealthPoller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		p.PollOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// PollOnce probes each workspace that has a connection.
func (p *HealthPoller) PollOnce(ctx context.Context) {
	cursor := ""
	for ctx.Err() == nil {
		workspaces, info, err := p.workspaces.List(ctx, pluginsdk.Page{Limit: workspacePageSize, Cursor: cursor})
		if err != nil {
			return
		}
		for _, workspace := range workspaces {
			if ctx.Err() != nil {
				return
			}
			if _, found, err := p.connections.store.Connection(ctx, workspace.ID); err == nil && found {
				p.connections.RecordHealth(ctx, workspace.ID)
			}
		}
		if info == nil || !info.HasMore || info.NextCursor == "" || info.NextCursor == cursor {
			return
		}
		cursor = info.NextCursor
	}
}

// listWorkspaceIDs pages every workspace visible to the plugin.
func listWorkspaceIDs(ctx context.Context, workspaces WorkspaceLister) ([]string, error) {
	var ids []string
	cursor := ""
	for ctx.Err() == nil {
		page, info, err := workspaces.List(ctx, pluginsdk.Page{Limit: workspacePageSize, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, workspace := range page {
			ids = append(ids, workspace.ID)
		}
		if info == nil || !info.HasMore || info.NextCursor == "" || info.NextCursor == cursor {
			break
		}
		cursor = info.NextCursor
	}
	return ids, ctx.Err()
}
