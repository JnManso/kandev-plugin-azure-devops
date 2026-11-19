package sourcecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type namedRepositoryDetailsStub struct {
	repositoryDetailsStub
	gotOwner string
	gotName  string
}

func (s *namedRepositoryDetailsStub) ResolveByName(_ context.Context, workspaceID, owner, name string) (Repository, error) {
	s.gotWorkspace, s.gotOwner, s.gotName = workspaceID, owner, name
	return s.resolved, nil
}

func TestInspectURLWithoutAMatchReturnsTheHostNoMatchEnvelope(t *testing.T) {
	extension := &Extension{ProviderID: "acme", RepositoryDetails: &repositoryDetailsStub{}}
	response, err := extension.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: ActionRepositoriesInspect,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-1"},
		Body:      []byte(`{"url":"https://elsewhere.example/a/b"}`),
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"matched":false}`, string(response.Body))
}

func TestInspectedRepositoryCarriesTheHostRepositoryIDField(t *testing.T) {
	details := &repositoryDetailsStub{inspected: &Repository{RepositoryID: "guid-1", Name: "api"}}
	extension := &Extension{ProviderID: "acme", RepositoryDetails: details}
	response, err := extension.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: ActionRepositoriesInspect,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-1"},
		Body:      []byte(`{"url":"https://code.example/a/b"}`),
	})
	require.NoError(t, err)
	var body struct {
		Repository map[string]any `json:"repository"`
	}
	require.NoError(t, json.Unmarshal(response.Body, &body))
	require.Equal(t, "guid-1", body.Repository["provider_repository_id"])
}

func TestBranchesFallBackToOwnerAndNameForRepositoriesWithoutIdentity(t *testing.T) {
	details := &namedRepositoryDetailsStub{repositoryDetailsStub: repositoryDetailsStub{
		resolved: Repository{RepositoryID: "guid-1"},
		branches: []Branch{{Name: "main", IsDefault: true}},
	}}
	extension := &Extension{ProviderID: "acme", RepositoryDetails: details}
	response, err := extension.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: ActionRepositoriesBranches,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-1"},
		Body:      []byte(`{"repository":{"provider_scope":"","provider_repository_id":"","owner_or_project":"Platform","name":"api"}}`),
	})
	require.NoError(t, err)
	require.Equal(t, "Platform", details.gotOwner)
	require.Equal(t, "api", details.gotName)
	require.Equal(t, Repository{RepositoryID: "guid-1"}, details.gotBranchRepo)
	require.JSONEq(t, `{"branches":[{"name":"main","is_default":true}]}`, string(response.Body))
}

func TestBranchesWithoutAnyIdentityAreAnInvalidArgument(t *testing.T) {
	extension := &Extension{ProviderID: "acme", RepositoryDetails: &repositoryDetailsStub{}}
	_, err := extension.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: ActionRepositoriesBranches,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-1"},
		Body:      []byte(`{"repository":{"owner_or_project":"Platform","name":"api"}}`),
	})
	var actionErr *pluginsdk.ActionError
	require.True(t, errors.As(err, &actionErr))
	require.Equal(t, pluginsdk.ActionErrorInvalidArgument, actionErr.Code)
}
