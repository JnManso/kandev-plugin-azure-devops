package plugin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/azure"
	"kandev-plugin-azure-devops/internal/store"
)

const (
	gitUsername            = "kandev"
	gitCredentialLifetime  = 10 * time.Minute
	credentialBindingLabel = "azure-devops-credential:"
	repositoryPageSize     = 200
)

// ErrCredentialUnavailable is the only error returned to the host, so a
// failed lookup never reveals which check rejected the request.
var ErrCredentialUnavailable = errors.New("Azure DevOps Git credential unavailable")

// RepositoryLister reads the workspace repositories through the Host.
type RepositoryLister interface {
	List(ctx context.Context, workspaceID string, page pluginsdk.Page) ([]pluginsdk.Repository, *pluginsdk.PageInfo, error)
}

// Credentials resolves Git credentials for Azure Repos remotes of a verified
// task session.
type Credentials struct {
	connections  *Connections
	repositories func() RepositoryLister
	now          func() time.Time
}

var _ pluginsdk.GitCredentialHandler = (*Credentials)(nil)

// NewCredentials returns the Git credential handler.
func NewCredentials(connections *Connections, repositories func() RepositoryLister) *Credentials {
	return &Credentials{connections: connections, repositories: repositories, now: time.Now}
}

type credentialScope struct {
	ProviderID, WorkspaceID, TaskID, SessionID, RepositoryID, Host, Path string
}

// ResolveGitCredential returns `kandev` and the workspace PAT when the request
// names an Azure Repos repository of the workspace in the connected
// organization.
func (c *Credentials) ResolveGitCredential(ctx context.Context, request *pluginsdk.ResolveGitCredentialRequest) (*pluginsdk.ResolveGitCredentialResponse, error) {
	if request == nil {
		return nil, ErrCredentialUnavailable
	}
	scope := credentialScope{request.ProviderID, request.WorkspaceID, request.TaskID, request.SessionID, request.RepositoryID, request.Host, request.Path}
	connection, err := c.verify(ctx, scope)
	if err != nil {
		return nil, ErrCredentialUnavailable
	}
	secret, expiresAt, err := c.connections.gitSecret(ctx, scope.WorkspaceID, connection)
	if err != nil || secret == "" || !expiresAt.After(c.now()) {
		return nil, ErrCredentialUnavailable
	}
	return &pluginsdk.ResolveGitCredentialResponse{
		Username:  gitUsername,
		Secret:    secret,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

// GetGitCredentialBinding returns the connection generation for the exact
// scope without reading the token, so a credential change revokes leases.
func (c *Credentials) GetGitCredentialBinding(ctx context.Context, request *pluginsdk.GitCredentialBindingRequest) (*pluginsdk.GitCredentialBindingResponse, error) {
	if request == nil {
		return nil, ErrCredentialUnavailable
	}
	scope := credentialScope{request.ProviderID, request.WorkspaceID, request.TaskID, request.SessionID, request.RepositoryID, request.Host, request.Path}
	connection, err := c.verify(ctx, scope)
	if err != nil {
		return nil, ErrCredentialUnavailable
	}
	return &pluginsdk.GitCredentialBindingResponse{Binding: credentialBindingLabel + strconv.FormatInt(connection.Generation, 10)}, nil
}

// verify checks the request scope, the workspace connection, and that the
// repository is an Azure Repos repository of the connected organization whose
// path matches the requested one. It returns the verified connection.
func (c *Credentials) verify(ctx context.Context, scope credentialScope) (store.Connection, error) {
	if !strings.EqualFold(scope.ProviderID, ProviderID) || scope.WorkspaceID == "" || scope.TaskID == "" ||
		scope.SessionID == "" || scope.RepositoryID == "" || !strings.EqualFold(scope.Host, azure.OrganizationHost) ||
		!strings.HasPrefix(scope.Path, "/") {
		return store.Connection{}, ErrCredentialUnavailable
	}
	_, connection, err := c.connections.Client(ctx, scope.WorkspaceID)
	if err != nil {
		return store.Connection{}, err
	}
	organization, err := azure.OrganizationName(connection.OrganizationURL)
	if err != nil {
		return store.Connection{}, err
	}
	requested, err := azure.ParseRemoteURL("https://" + azure.OrganizationHost + scope.Path)
	if err != nil || !strings.EqualFold(requested.Organization, organization) {
		return store.Connection{}, ErrCredentialUnavailable
	}
	repository, err := c.findRepository(ctx, scope.WorkspaceID, scope.RepositoryID)
	if err != nil {
		return store.Connection{}, err
	}
	if !strings.EqualFold(repository.ProviderID, ProviderID) {
		return store.Connection{}, ErrCredentialUnavailable
	}
	recorded, ok := recordedRemote(repository, organization)
	if !ok || !sameRemote(recorded, requested) {
		return store.Connection{}, ErrCredentialUnavailable
	}
	return connection, nil
}

// recordedRemote is the repository a row names: its clone URL, or, for a row
// Kandev recorded without one, its Azure Repos host, project, and name in the
// connected organization. A row with neither names nothing.
func recordedRemote(repository pluginsdk.Repository, organization string) (azure.RemoteRepository, bool) {
	if repository.RemoteURL != "" {
		remote, err := azure.ParseRemoteURL(repository.RemoteURL)
		return remote, err == nil
	}
	host := strings.TrimSuffix(strings.TrimSpace(repository.ProviderHost), "/")
	if !strings.EqualFold(host, "https://"+azure.OrganizationHost) ||
		repository.OwnerOrProject == "" || repository.ProviderName == "" {
		return azure.RemoteRepository{}, false
	}
	return azure.RemoteRepository{
		Organization: organization,
		Project:      repository.OwnerOrProject,
		Repository:   repository.ProviderName,
	}, true
}

func sameRemote(left, right azure.RemoteRepository) bool {
	return strings.EqualFold(left.Organization, right.Organization) &&
		strings.EqualFold(left.Project, right.Project) &&
		strings.EqualFold(left.Repository, right.Repository)
}

func (c *Credentials) findRepository(ctx context.Context, workspaceID, repositoryID string) (pluginsdk.Repository, error) {
	lister := c.repositories()
	if lister == nil {
		return pluginsdk.Repository{}, ErrCredentialUnavailable
	}
	cursor := ""
	for {
		repositories, info, err := lister.List(ctx, workspaceID, pluginsdk.Page{Limit: repositoryPageSize, Cursor: cursor})
		if err != nil {
			return pluginsdk.Repository{}, err
		}
		for _, repository := range repositories {
			if repository.ID == repositoryID && repository.WorkspaceID == workspaceID {
				return repository, nil
			}
		}
		if info == nil || !info.HasMore || info.NextCursor == "" || info.NextCursor == cursor {
			return pluginsdk.Repository{}, ErrCredentialUnavailable
		}
		cursor = info.NextCursor
	}
}
