// Package hosttest provides an in-memory pluginsdk.Host for tests.
package hosttest

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/kdlbs/kandev/pluginsdk"
)

// Host is an in-memory Host with state, secrets, workspaces, and repositories.
type Host struct {
	pluginsdk.UnimplementedHostData

	mu           sync.Mutex
	state        map[string]map[string]any
	secrets      map[string]string
	workspaces   []pluginsdk.Workspace
	repositories []pluginsdk.Repository
	tasks        map[string]pluginsdk.Task
	taskOrder    []string
	createdTasks []pluginsdk.CreateTaskInput

	workflows        []pluginsdk.Workflow
	steps            map[string][]pluginsdk.WorkflowStep
	agentProfiles    []pluginsdk.AgentProfile
	executorProfiles []pluginsdk.ExecutorProfile

	// FailCreateTask, when set, is returned by Tasks().Create.
	FailCreateTask error

	// FailSetState, when set, makes SetState fail for that key.
	FailSetState string
	// Config is returned by GetConfig.
	Config map[string]any
}

// New returns an empty Host.
func New() *Host {
	return &Host{state: map[string]map[string]any{}, secrets: map[string]string{}}
}

func stateKey(scope, scopeID, key string) string { return scope + ":" + scopeID + ":" + key }

func (h *Host) GetState(_ context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	value, found := h.state[stateKey(scope, scopeID, key)]
	return value, found, nil
}

func (h *Host) SetState(_ context.Context, scope, scopeID, key string, value map[string]any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.FailSetState == key {
		return errors.New("state unavailable")
	}
	h.state[stateKey(scope, scopeID, key)] = value
	return nil
}

func (h *Host) DeleteState(_ context.Context, scope, scopeID, key string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.state, stateKey(scope, scopeID, key))
	return nil
}

func (h *Host) ListState(_ context.Context, scope, scopeID string) ([]pluginsdk.StateEntry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	prefix := scope + ":" + scopeID + ":"
	var entries []pluginsdk.StateEntry
	for key, value := range h.state {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, pluginsdk.StateEntry{Key: strings.TrimPrefix(key, prefix), Value: value})
		}
	}
	return entries, nil
}

// StateKeys returns the keys stored for one scope, for assertions.
func (h *Host) StateKeys(scope, scopeID string) []string {
	entries, _ := h.ListState(context.Background(), scope, scopeID)
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return keys
}

func (h *Host) GetConfig(context.Context) (map[string]any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	config := map[string]any{}
	for key, value := range h.Config {
		config[key] = value
	}
	return config, nil
}

func (h *Host) RevealSecret(context.Context, string) (string, error) {
	return "", errors.New("reveal is not supported")
}

func (h *Host) GetSecret(_ context.Context, key string) (string, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	value, found := h.secrets[key]
	return value, found, nil
}

func (h *Host) SetSecret(_ context.Context, key, value string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.secrets[key] = value
	return nil
}

func (h *Host) DeleteSecret(_ context.Context, key string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.secrets, key)
	return nil
}

func (h *Host) EmitEvent(context.Context, string, map[string]any) error { return nil }

// Secret returns a stored secret for assertions.
func (h *Host) Secret(key string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	value, found := h.secrets[key]
	return value, found
}

// AddWorkspace registers a workspace for Workspaces().List.
func (h *Host) AddWorkspace(id string) {
	h.AddNamedWorkspace(id, id)
}

// AddNamedWorkspace registers a workspace with a display name.
func (h *Host) AddNamedWorkspace(id, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workspaces = append(h.workspaces, pluginsdk.Workspace{ID: id, Name: name})
}

// AddRepository registers a repository for Repositories().List.
func (h *Host) AddRepository(repository pluginsdk.Repository) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.repositories = append(h.repositories, repository)
}

func (h *Host) Workspaces() pluginsdk.WorkspaceReader    { return workspaceReader{h} }
func (h *Host) Repositories() pluginsdk.RepositoryReader { return repositoryReader{h} }

type workspaceReader struct{ host *Host }

func (r workspaceReader) List(context.Context, pluginsdk.Page) ([]pluginsdk.Workspace, *pluginsdk.PageInfo, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	return append([]pluginsdk.Workspace(nil), r.host.workspaces...), &pluginsdk.PageInfo{}, nil
}

type repositoryReader struct{ host *Host }

func (r repositoryReader) List(_ context.Context, workspaceID string, _ pluginsdk.Page) ([]pluginsdk.Repository, *pluginsdk.PageInfo, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	var result []pluginsdk.Repository
	for _, repository := range r.host.repositories {
		if repository.WorkspaceID == workspaceID {
			result = append(result, repository)
		}
	}
	return result, &pluginsdk.PageInfo{}, nil
}

var _ pluginsdk.Host = (*Host)(nil)
