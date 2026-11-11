package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/azure"
	"kandev-plugin-azure-devops/internal/hosttest"
)

// fakeAzure is an in-memory Azure DevOps organization keyed by PAT.
type fakeAzure struct {
	mu           sync.Mutex
	validPAT     string
	repositories []azure.Repository
	branches     map[string][]azure.Branch
	projects     []azure.Project
	created      []string
	validBearers map[string]bool
	browse       browseCalls
	pullRequests map[int]azure.PullRequest
	createdPRs   []azure.CreatePullRequestRequest
	myTeamsErr   error
	childrenErr  error
}

func newFakeAzure() *fakeAzure {
	return &fakeAzure{
		validPAT: "good-pat",
		repositories: []azure.Repository{
			{ID: "guid-api", Name: "api", ProjectID: "p1", ProjectName: "Platform Team", DefaultBranch: "main"},
			{ID: "guid-web", Name: "web", ProjectID: "p1", ProjectName: "Platform Team", DefaultBranch: "develop"},
			{ID: "guid-old", Name: "legacy", ProjectID: "p2", ProjectName: "Archive", DefaultBranch: "main", IsDisabled: true},
			{ID: "guid-ops", Name: "infra", ProjectID: "p3", ProjectName: "Ops", DefaultBranch: "main"},
		},
		branches:     map[string][]azure.Branch{"guid-api": {{Name: "main"}, {Name: "feature/x"}}},
		projects:     []azure.Project{{ID: "p1", Name: "Platform Team"}, {ID: "p3", Name: "Ops"}},
		validBearers: map[string]bool{},
	}
}

func (f *fakeAzure) factory(organizationURL string, auth azure.Authorizer) Client {
	f.mu.Lock()
	f.created = append(f.created, organizationURL)
	f.mu.Unlock()
	return &fakeClient{azure: f, auth: auth}
}

type fakeClient struct {
	azure *fakeAzure
	auth  azure.Authorizer
}

// authorize accepts the valid PAT or any bearer token the fake Entra issued
// and has not revoked, renewing once on rejection like the REST client.
func (c *fakeClient) authorize() error {
	ctx := context.Background()
	header, err := c.auth.Header(ctx)
	if err != nil {
		return err
	}
	if c.azure.accepts(header) {
		return nil
	}
	renewed, err := c.auth.Renew(ctx, header)
	if err != nil {
		return err
	}
	if renewed {
		if header, err = c.auth.Header(ctx); err == nil && c.azure.accepts(header) {
			return nil
		}
	}
	return &azure.APIError{StatusCode: 401, Endpoint: "/_apis/connectionData", Body: "unauthorized"}
}

func (f *fakeAzure) accepts(header string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if header == "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+f.validPAT)) {
		return true
	}
	token, isBearer := strings.CutPrefix(header, azure.BearerPrefix)
	return isBearer && f.validBearers[token]
}

func (c *fakeClient) TestAuth(context.Context) (*azure.TestConnectionResult, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return &azure.TestConnectionResult{OK: true, ID: "user-1", DisplayName: "Ada", Email: "ada@example.com"}, nil
}

func (c *fakeClient) ListProjects(context.Context) ([]azure.Project, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return c.azure.projects, nil
}

func (c *fakeClient) ListAllRepositories(context.Context) ([]azure.Repository, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return c.azure.repositories, nil
}

func (c *fakeClient) GetRepository(_ context.Context, project, idOrName string) (*azure.Repository, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	for _, repository := range c.azure.repositories {
		if repository.ID == idOrName || (repository.ProjectName == project && repository.Name == idOrName) {
			found := repository
			return &found, nil
		}
	}
	return nil, &azure.APIError{StatusCode: 404, Endpoint: "/_apis/git/repositories", Body: "not found"}
}

func (c *fakeClient) ListBranches(_ context.Context, _, repositoryID string) ([]azure.Branch, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return c.azure.branches[repositoryID], nil
}

func (c *fakeClient) BranchExists(ctx context.Context, projectID, repositoryID, branch string) (bool, error) {
	branches, err := c.ListBranches(ctx, projectID, repositoryID)
	for _, candidate := range branches {
		if strings.TrimPrefix(candidate.Name, "refs/heads/") == branch {
			return true, err
		}
	}
	return false, err
}

// harness wires a Runtime to an in-memory Host and fake Azure.
type harness struct {
	t       *testing.T
	host    *hosttest.Host
	azure   *fakeAzure
	entra   *fakeEntra
	runtime *Runtime
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	host := hosttest.New()
	fake := newFakeAzure()
	entra := newFakeEntra(fake)
	runtime := NewRuntimeWith(fake.factory, entra)
	runtime.SetHost(host)
	t.Cleanup(runtime.Stop)
	return &harness{t: t, host: host, azure: fake, entra: entra, runtime: runtime}
}

func (h *harness) action(workspaceID, key string, body any) (int, map[string]any) {
	h.t.Helper()
	return h.actionWith(pluginsdk.VerifiedActionContext{WorkspaceID: workspaceID}, key, body)
}

func (h *harness) actionWith(verified pluginsdk.VerifiedActionContext, key string, body any) (int, map[string]any) {
	h.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		h.t.Fatalf("encode body: %v", err)
	}
	response, err := h.runtime.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: key, Context: verified, Body: encoded,
	})
	if err != nil {
		h.t.Fatalf("HandleAction(%s) error = %v", key, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.Body, &decoded); err != nil {
		h.t.Fatalf("decode %s response %q: %v", key, response.Body, err)
	}
	status := response.Status
	if status == 0 {
		status = 200
	}
	return status, decoded
}

func (h *harness) connect(workspaceID string) {
	h.t.Helper()
	status, body := h.action(workspaceID, "connection.save", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "pat": h.azure.validPAT,
	})
	if status != 200 {
		h.t.Fatalf("connection.save status = %d body = %v", status, body)
	}
}

func field(value any, path ...string) any {
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[key]
	}
	return value
}

func flatten(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
