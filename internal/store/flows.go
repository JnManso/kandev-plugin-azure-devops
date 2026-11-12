package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const flowKeyPrefix = "oauth.flow."

// Flow is a pending Microsoft sign-in, keyed by the SHA-256 of its state.
// It expires after ten minutes and can be claimed once.
type Flow struct {
	StateHash          string    `json:"stateHash"`
	WorkspaceID        string    `json:"workspaceId"`
	ActorID            string    `json:"actorId"`
	OrganizationURL    string    `json:"organizationUrl"`
	DefaultProjectID   string    `json:"defaultProjectId,omitempty"`
	DefaultProjectName string    `json:"defaultProjectName,omitempty"`
	TenantID           string    `json:"tenantId"`
	ClientID           string    `json:"clientId"`
	PKCEVerifier       string    `json:"pkceVerifier"`
	RedirectURL        string    `json:"redirectUrl"`
	ExpiresAt          time.Time `json:"expiresAt"`
	// ConnectionFound and ConnectionGeneration record the workspace
	// connection when the sign-in started; completing it after the
	// connection changed is rejected.
	ConnectionFound      bool  `json:"connectionFound,omitempty"`
	ConnectionGeneration int64 `json:"connectionGeneration,omitempty"`
}

// Flows stores pending sign-ins in workspace state. Claims are serialized in
// process, so a state can complete at most one sign-in.
type Flows struct {
	store *Store
	list  func(ctx context.Context, workspaceID string) ([]string, error)
	mu    sync.Mutex
}

// NewFlows returns the flow store. list returns the workspace's state keys.
func NewFlows(store *Store, list func(ctx context.Context, workspaceID string) ([]string, error)) *Flows {
	return &Flows{store: store, list: list}
}

// Put records a flow and removes expired ones.
func (f *Flows) Put(ctx context.Context, flow Flow, now time.Time) error {
	f.Purge(ctx, flow.WorkspaceID, now, false)
	return f.store.save(ctx, flow.WorkspaceID, flowKeyPrefix+flow.StateHash, flow)
}

// Claim removes and returns the flow for stateHash. found is false when it is
// unknown, already claimed, or expired.
func (f *Flows) Claim(ctx context.Context, workspaceID, stateHash string, now time.Time) (Flow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var flow Flow
	key := flowKeyPrefix + stateHash
	found, err := f.store.load(ctx, workspaceID, key, &flow)
	if err != nil || !found {
		return Flow{}, false, err
	}
	if err := f.store.host.DeleteState(ctx, scopeWorkspace, workspaceID, key); err != nil {
		return Flow{}, false, fmt.Errorf("claim sign-in: %w", err)
	}
	if !now.Before(flow.ExpiresAt) {
		return Flow{}, false, nil
	}
	return flow, true, nil
}

// Purge deletes expired flows, or every flow when all is true. Errors are
// ignored: an unpurged flow still expires on claim.
func (f *Flows) Purge(ctx context.Context, workspaceID string, now time.Time, all bool) {
	if f.list == nil {
		return
	}
	keys, err := f.list(ctx, workspaceID)
	if err != nil {
		return
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, flowKeyPrefix) {
			continue
		}
		if !all {
			var flow Flow
			if found, err := f.store.load(ctx, workspaceID, key, &flow); err != nil || !found || now.Before(flow.ExpiresAt) {
				continue
			}
		}
		_ = f.store.host.DeleteState(ctx, scopeWorkspace, workspaceID, key)
	}
}
