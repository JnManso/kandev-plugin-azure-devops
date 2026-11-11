package plugin

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"kandev-plugin-azure-devops/internal/azure"
	"kandev-plugin-azure-devops/internal/sourcecontrol"
)

const (
	// ProviderID is the Kandev repository provider this plugin owns.
	ProviderID = "azure_devops"
	// ProviderHost is the HTTPS origin of every Azure Repos clone URL.
	ProviderHost = "https://" + azure.OrganizationHost

	repositoryListCursor = "after"
)

var errRepositoryNotFound = errors.New("the Azure DevOps repository was not found or is not accessible")

// Repositories adapts the Azure DevOps REST client to the source-control
// boundary. Every call resolves the verified workspace connection first.
type Repositories struct{ connections *Connections }

var (
	_ sourcecontrol.RepositoryLister       = (*Repositories)(nil)
	_ sourcecontrol.RepositoryDetails      = (*Repositories)(nil)
	_ sourcecontrol.RepositoryNameResolver = (*Repositories)(nil)
)

// NewRepositories returns the repository adapter.
func NewRepositories(connections *Connections) *Repositories {
	return &Repositories{connections: connections}
}

// ConnectionScope binds list cursors to the organization and credential
// generation, so a cursor from an older connection is rejected.
func (r *Repositories) ConnectionScope(ctx context.Context, workspaceID string) (string, error) {
	_, connection, err := r.connections.Client(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	organization, err := azure.OrganizationName(connection.OrganizationURL)
	if err != nil {
		return "", unavailable(err)
	}
	return organization + "#" + strconv.FormatInt(connection.Generation, 10), nil
}

// List returns enabled repositories across the organization, filtered by a
// case-insensitive match on project or repository name, ordered by project
// and name, and paged after the last returned repository ID.
func (r *Repositories) List(ctx context.Context, workspaceID, query string, cursor sourcecontrol.RepositoryCursor, limit int) (sourcecontrol.RepositoryPage, error) {
	client, connection, err := r.connections.Client(ctx, workspaceID)
	if err != nil {
		return sourcecontrol.RepositoryPage{}, err
	}
	organization, err := azure.OrganizationName(connection.OrganizationURL)
	if err != nil {
		return sourcecontrol.RepositoryPage{}, unavailable(err)
	}
	all, err := client.ListAllRepositories(ctx)
	if err != nil {
		return sourcecontrol.RepositoryPage{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	matches := make([]azure.Repository, 0, len(all))
	for _, repository := range all {
		if repository.IsDisabled {
			continue
		}
		haystack := strings.ToLower(repository.ProjectName + "/" + repository.Name)
		if needle == "" || strings.Contains(haystack, needle) {
			matches = append(matches, repository)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		left := strings.ToLower(matches[i].ProjectName + "/" + matches[i].Name)
		right := strings.ToLower(matches[j].ProjectName + "/" + matches[j].Name)
		if left != right {
			return left < right
		}
		return matches[i].ID < matches[j].ID
	})
	start := 0
	if cursor.AfterRepositoryID != "" {
		start = -1
		for index, repository := range matches {
			if repository.ID == cursor.AfterRepositoryID {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return sourcecontrol.RepositoryPage{}, invalidArgument(errors.New("the repository cursor is no longer valid"))
		}
	}
	end := min(start+limit, len(matches))
	page := sourcecontrol.RepositoryPage{Repositories: make([]sourcecontrol.Repository, 0, end-start)}
	for _, repository := range matches[start:end] {
		page.Repositories = append(page.Repositories, descriptor(organization, repository))
	}
	if end < len(matches) && end > start {
		page.Next = sourcecontrol.RepositoryCursor{Remote: repositoryListCursor, AfterRepositoryID: matches[end-1].ID}
	}
	return page, nil
}

// Inspect resolves an Azure Repos URL in the connected organization. A URL
// for another host or organization is not a match; a matching URL whose
// repository cannot be read is not found.
func (r *Repositories) Inspect(ctx context.Context, workspaceID, rawURL string) (*sourcecontrol.Repository, error) {
	remote, err := azure.ParseRemoteURL(rawURL)
	if err != nil {
		return nil, nil
	}
	client, connection, err := r.connections.Client(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	organization, err := azure.OrganizationName(connection.OrganizationURL)
	if err != nil {
		return nil, unavailable(err)
	}
	if !strings.EqualFold(remote.Organization, organization) {
		return nil, nil
	}
	repository, err := client.GetRepository(ctx, remote.Project, remote.Repository)
	if err != nil {
		return nil, repositoryReadError(err)
	}
	result := descriptor(organization, *repository)
	return &result, nil
}

// Resolve reads a repository by its immutable ID and checks that it still
// belongs to the recorded organization and project scope.
func (r *Repositories) Resolve(ctx context.Context, workspaceID string, identity sourcecontrol.RepositoryIdentity) (sourcecontrol.Repository, error) {
	client, connection, err := r.connections.Client(ctx, workspaceID)
	if err != nil {
		return sourcecontrol.Repository{}, err
	}
	organization, err := azure.OrganizationName(connection.OrganizationURL)
	if err != nil {
		return sourcecontrol.Repository{}, unavailable(err)
	}
	scopeOrganization, _, found := strings.Cut(identity.ConnectionScope, "/")
	if !found || !strings.EqualFold(scopeOrganization, organization) {
		return sourcecontrol.Repository{}, notFound(errRepositoryNotFound)
	}
	repository, err := client.GetRepository(ctx, "", identity.RepositoryID)
	if err != nil {
		return sourcecontrol.Repository{}, repositoryReadError(err)
	}
	result := descriptor(organization, *repository)
	if !strings.EqualFold(result.ConnectionScope, identity.ConnectionScope) {
		return sourcecontrol.Repository{}, notFound(errRepositoryNotFound)
	}
	return result, nil
}

// ResolveByName reads a repository recorded as project and name.
func (r *Repositories) ResolveByName(ctx context.Context, workspaceID, project, name string) (sourcecontrol.Repository, error) {
	client, connection, err := r.connections.Client(ctx, workspaceID)
	if err != nil {
		return sourcecontrol.Repository{}, err
	}
	organization, err := azure.OrganizationName(connection.OrganizationURL)
	if err != nil {
		return sourcecontrol.Repository{}, unavailable(err)
	}
	repository, err := client.GetRepository(ctx, project, name)
	if err != nil {
		return sourcecontrol.Repository{}, repositoryReadError(err)
	}
	return descriptor(organization, *repository), nil
}

// ListBranches lists branch heads and marks the default branch.
func (r *Repositories) ListBranches(ctx context.Context, workspaceID string, repository sourcecontrol.Repository) ([]sourcecontrol.Branch, error) {
	client, _, err := r.connections.Client(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	project, _ := strings.CutPrefix(repository.ConnectionScope, scopeOrganization(repository.ConnectionScope)+"/")
	branches, err := client.ListBranches(ctx, project, repository.RepositoryID)
	if err != nil {
		return nil, repositoryReadError(err)
	}
	result := make([]sourcecontrol.Branch, 0, len(branches))
	for _, branch := range branches {
		result = append(result, sourcecontrol.Branch{Name: branch.Name, IsDefault: branch.Name == repository.DefaultBranch})
	}
	return result, nil
}

func scopeOrganization(scope string) string {
	organization, _, _ := strings.Cut(scope, "/")
	return organization
}

// descriptor is the credential-free Kandev repository identity. The scope is
// `<organization>/<project>`; the repository ID is Azure's immutable GUID.
func descriptor(organization string, repository azure.Repository) sourcecontrol.Repository {
	return sourcecontrol.Repository{
		ProviderID:      ProviderID,
		ProviderHost:    ProviderHost,
		ConnectionScope: organization + "/" + repository.ProjectName,
		RepositoryID:    repository.ID,
		OwnerOrProject:  repository.ProjectName,
		Name:            repository.Name,
		CloneURL:        azure.CanonicalCloneURL(organization, repository.ProjectName, repository.Name),
		DefaultBranch:   repository.DefaultBranch,
	}
}

func repositoryReadError(err error) error {
	var apiErr *azure.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.StatusCode == 403) {
		return notFound(errRepositoryNotFound)
	}
	if errors.As(err, &apiErr) && apiErr.StatusCode == 401 {
		return failedPrecondition(errors.New("Azure DevOps rejected the stored credentials; update them in settings"))
	}
	return err
}
