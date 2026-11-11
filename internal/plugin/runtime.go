// Package plugin is the Azure DevOps plugin runtime: the value passed to
// pluginsdk.Serve.
package plugin

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/auth"
	"kandev-plugin-azure-devops/internal/azure"
	"kandev-plugin-azure-devops/internal/sourcecontrol"
	"kandev-plugin-azure-devops/internal/store"
	"kandev-plugin-azure-devops/internal/watches"
)

// ID is the manifest id. It prefixes every host route served for this plugin.
const ID = "kandev-plugin-azure-devops"

var errNotReady = errors.New("the Azure DevOps plugin is starting")

// Runtime implements pluginsdk.Plugin and its optional extensions. Handlers
// are unavailable until the Host is injected.
type Runtime struct {
	pluginsdk.UnimplementedPlugin

	newClient ClientFactory
	entra     auth.IdentityProvider
	mu        sync.RWMutex
	services  *services
	cancel    context.CancelFunc
}

type services struct {
	workspaces    WorkspaceLister
	connections   *Connections
	browse        *Browse
	sourceControl *sourcecontrol.Extension
	credentials   *Credentials
	links         *Links
	taskLinks     *TaskLinks
	mentions      *Mentions
	watches       *Watches
}

var (
	_ pluginsdk.Plugin                 = (*Runtime)(nil)
	_ pluginsdk.ActionHandler          = (*Runtime)(nil)
	_ pluginsdk.GitCredentialHandler   = (*Runtime)(nil)
	_ pluginsdk.HostSetter             = (*Runtime)(nil)
	_ pluginsdk.AgentToolPlugin        = (*Runtime)(nil)
	_ pluginsdk.EntityReferenceHandler = (*Runtime)(nil)
)

// MockModeEnv enables an in-process demo Azure DevOps organization and
// Microsoft sign-in; it is for local previews and packaged E2E runs.
const MockModeEnv = "KANDEV_AZURE_DEVOPS_PLUGIN_MOCK"

// NewRuntime returns the plugin runtime served by server/main.go.
func NewRuntime() *Runtime {
	if os.Getenv(MockModeEnv) == "true" {
		if mock, err := azure.NewSeededMockClient(); err == nil {
			return NewRuntimeWith(func(string, azure.Authorizer) Client { return mock }, &auth.MockEntra{})
		}
	}
	return NewRuntimeWith(nil, nil)
}

// NewRuntimeWith returns a runtime with injected Azure DevOps and Microsoft
// identity clients (tests use in-memory fakes); nil selects the real ones.
func NewRuntimeWith(newClient ClientFactory, entra auth.IdentityProvider) *Runtime {
	return &Runtime{newClient: newClient, entra: entra}
}

// SetHost wires the services and starts the health poller.
func (r *Runtime) SetHost(host pluginsdk.Host) {
	r.UnimplementedPlugin.SetHost(host)
	stateKeys := func(ctx context.Context, workspaceID string) ([]string, error) {
		entries, err := host.ListState(ctx, "workspace", workspaceID)
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(entries))
		for _, entry := range entries {
			keys = append(keys, entry.Key)
		}
		return keys, nil
	}
	stateStore := store.New(host)
	connections := NewConnections(stateStore, ConnectionsOptions{
		NewClient: r.newClient,
		Entra:     r.entra,
		Config:    host.GetConfig,
		StateKeys: stateKeys,
	})
	links := NewLinks(stateStore, stateKeys)
	tasks := func() TaskReader { return host.Tasks() }
	hostRepositories := func() RepositoryLister { return host.Repositories() }
	repositories := NewRepositories(connections)
	built := &services{
		workspaces:  host.Workspaces(),
		connections: connections,
		browse:      NewBrowse(connections),
		sourceControl: &sourcecontrol.Extension{
			ProviderID: ProviderID, Repositories: repositories, RepositoryDetails: repositories,
		},
		credentials: NewCredentials(connections, hostRepositories),
		links:       links,
		taskLinks:   NewTaskLinks(connections, links, tasks, hostRepositories),
		mentions:    NewMentions(connections),
	}
	watchService := watches.NewService(watches.Dependencies{
		Store: watches.NewStore(stateStore, stateKeys),
		Client: func(ctx context.Context, workspaceID string) (watches.Client, error) {
			client, _, err := connections.Client(ctx, workspaceID)
			return client, err
		},
		Tasks:      func() watches.Tasks { return host.Tasks() },
		Workspaces: func(ctx context.Context) ([]string, error) { return listWorkspaceIDs(ctx, host.Workspaces()) },
	})
	built.watches = &Watches{service: watchService, host: host, executorProfiles: func() (pluginsdk.ExecutorProfileReader, bool) {
		return pluginsdk.ExecutorProfiles(host)
	}}
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	previous := r.cancel
	r.services, r.cancel = built, cancel
	r.mu.Unlock()
	if previous != nil {
		previous()
	}
	go NewHealthPoller(connections, host.Workspaces()).Run(ctx)
	go links.reconcileDeletedTasks(ctx, host.Workspaces(), host.Tasks())
	go watchService.Run(ctx)
}

// Stop cancels background work. It is safe to call more than once.
func (r *Runtime) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Runtime) ready() (*services, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.services == nil {
		return nil, unavailable(errNotReady)
	}
	return r.services, nil
}

// HandleAction routes browser and host actions.
func (r *Runtime) HandleAction(ctx context.Context, request *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	if request == nil {
		return actionFailure(invalidArgument(errors.New("an action request is required")))
	}
	services, err := r.ready()
	if err != nil {
		return actionFailure(err)
	}
	var response *pluginsdk.PluginActionResponse
	if strings.HasPrefix(request.ActionKey, "repositories.") {
		response, err = services.sourceControl.HandleAction(ctx, request)
	} else if result, found, linkErr := services.taskLinks.Handle(ctx, request.Context, request.ActionKey, request.Body); found {
		if linkErr != nil {
			return actionFailure(linkErr)
		}
		response, err = jsonAction(result)
	} else {
		response, err = services.handleConnectionAction(ctx, request)
	}
	if err != nil {
		return actionFailure(err)
	}
	return response, nil
}

// ResolveGitCredential implements pluginsdk.GitCredentialResolver.
func (r *Runtime) ResolveGitCredential(ctx context.Context, request *pluginsdk.ResolveGitCredentialRequest) (*pluginsdk.ResolveGitCredentialResponse, error) {
	services, err := r.ready()
	if err != nil {
		return nil, ErrCredentialUnavailable
	}
	return services.credentials.ResolveGitCredential(ctx, request)
}

// GetGitCredentialBinding implements pluginsdk.GitCredentialBinder.
func (r *Runtime) GetGitCredentialBinding(ctx context.Context, request *pluginsdk.GitCredentialBindingRequest) (*pluginsdk.GitCredentialBindingResponse, error) {
	services, err := r.ready()
	if err != nil {
		return nil, ErrCredentialUnavailable
	}
	return services.credentials.GetGitCredentialBinding(ctx, request)
}

// OnEvent removes the links of deleted tasks. A task.deleted event that
// arrives before the Host is injected fails, so Kandev retries it.
func (r *Runtime) OnEvent(ctx context.Context, event *pluginsdk.Event) error {
	if event == nil || event.EventType != TaskDeletedEvent {
		return nil
	}
	services, err := r.ready()
	if err != nil {
		return err
	}
	return services.links.handleTaskDeleted(ctx, event)
}

// InvokeAgentTool implements pluginsdk.AgentToolPlugin.
func (r *Runtime) InvokeAgentTool(ctx context.Context, request *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error) {
	if request == nil || request.Name != CreatePullRequestTool {
		return toolError("unknown tool"), nil
	}
	services, err := r.ready()
	if err != nil {
		return toolError("the Azure DevOps plugin is starting; try again"), nil
	}
	return services.taskLinks.InvokeCreatePullRequest(ctx, request), nil
}

// SearchEntityReferences implements pluginsdk.EntityReferenceSearcher.
func (r *Runtime) SearchEntityReferences(ctx context.Context, request *pluginsdk.SearchEntityReferencesRequest) (*pluginsdk.SearchEntityReferencesResponse, error) {
	services, err := r.ready()
	if err != nil {
		return nil, err
	}
	return services.mentions.Search(ctx, request)
}

// AuthorizeEntityReference implements pluginsdk.EntityReferenceAuthorizer.
func (r *Runtime) AuthorizeEntityReference(ctx context.Context, request *pluginsdk.AuthorizeEntityReferenceRequest) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
	services, err := r.ready()
	if err != nil {
		return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false, Reason: "the Azure DevOps plugin is starting"}, nil
	}
	return services.mentions.Authorize(ctx, request)
}
