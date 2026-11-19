package plugin

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func credentialRequest() *pluginsdk.ResolveGitCredentialRequest {
	return &pluginsdk.ResolveGitCredentialRequest{
		ProviderID: "azure_devops", WorkspaceID: "ws-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repo-1", Host: "dev.azure.com", Path: "/acme/Platform%20Team/_git/api",
	}
}

func credentialHarness(t *testing.T, remoteURL string) *harness {
	t.Helper()
	h := newHarness(t)
	h.connect("ws-1")
	h.host.AddRepository(pluginsdk.Repository{ID: "repo-1", WorkspaceID: "ws-1", ProviderID: "azure_devops", RemoteURL: remoteURL})
	h.host.AddRepository(pluginsdk.Repository{ID: "repo-gh", WorkspaceID: "ws-1", ProviderID: "github", RemoteURL: "https://github.com/acme/api"})
	return h
}

func TestResolveGitCredentialReturnsThePATForAWorkspaceRepository(t *testing.T) {
	h := credentialHarness(t, "https://acme@dev.azure.com/acme/Platform%20Team/_git/api")
	response, err := h.runtime.ResolveGitCredential(context.Background(), credentialRequest())
	if err != nil {
		t.Fatalf("ResolveGitCredential() error = %v", err)
	}
	if response.Username != "kandev" || response.Secret != "good-pat" || response.ExpiresAt == "" {
		t.Fatalf("credential = %+v", response)
	}
}

func TestResolveGitCredentialRejectsEveryOutOfScopeRequest(t *testing.T) {
	h := credentialHarness(t, "https://dev.azure.com/acme/Platform%20Team/_git/api")
	cases := map[string]func(*pluginsdk.ResolveGitCredentialRequest){
		"another provider":           func(r *pluginsdk.ResolveGitCredentialRequest) { r.ProviderID = "github" },
		"another host":               func(r *pluginsdk.ResolveGitCredentialRequest) { r.Host = "evil.example" },
		"another organization":       func(r *pluginsdk.ResolveGitCredentialRequest) { r.Path = "/other/Platform%20Team/_git/api" },
		"another repository path":    func(r *pluginsdk.ResolveGitCredentialRequest) { r.Path = "/acme/Platform%20Team/_git/web" },
		"a repository of no task":    func(r *pluginsdk.ResolveGitCredentialRequest) { r.RepositoryID = "repo-unknown" },
		"a non-Azure repository row": func(r *pluginsdk.ResolveGitCredentialRequest) { r.RepositoryID = "repo-gh" },
		"no session":                 func(r *pluginsdk.ResolveGitCredentialRequest) { r.SessionID = "" },
		"another workspace":          func(r *pluginsdk.ResolveGitCredentialRequest) { r.WorkspaceID = "ws-2" },
	}
	for name, mutate := range cases {
		request := credentialRequest()
		mutate(request)
		if response, err := h.runtime.ResolveGitCredential(context.Background(), request); err == nil {
			t.Fatalf("%s: credential issued: %+v", name, response)
		}
	}
}

func TestResolveGitCredentialFailsClosedWhenReauthIsRequired(t *testing.T) {
	h := credentialHarness(t, "https://dev.azure.com/acme/Platform%20Team/_git/api")
	h.connect("ws-source")
	h.action("ws-1", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-source"})
	h.action("ws-source", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/other", "pat": "good-pat"})
	h.action("ws-1", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-source"})
	if _, err := h.runtime.ResolveGitCredential(context.Background(), credentialRequest()); err == nil {
		t.Fatal("a credential was issued for a connection that requires reauthentication")
	}
}

func TestCredentialBindingChangesWhenThePATChanges(t *testing.T) {
	h := credentialHarness(t, "https://dev.azure.com/acme/Platform%20Team/_git/api")
	bindingRequest := func() *pluginsdk.GitCredentialBindingRequest {
		r := credentialRequest()
		return &pluginsdk.GitCredentialBindingRequest{ProviderID: r.ProviderID, WorkspaceID: r.WorkspaceID, TaskID: r.TaskID, SessionID: r.SessionID, RepositoryID: r.RepositoryID, Host: r.Host, Path: r.Path}
	}
	first, err := h.runtime.GetGitCredentialBinding(context.Background(), bindingRequest())
	if err != nil || first.Binding != "azure-devops-credential:1" {
		t.Fatalf("first binding = %+v, %v", first, err)
	}
	h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "new-pat"})
	second, err := h.runtime.GetGitCredentialBinding(context.Background(), bindingRequest())
	if err != nil || second.Binding != "azure-devops-credential:2" {
		t.Fatalf("second binding = %+v, %v", second, err)
	}
}
