package plugin

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

// identityRepository is a provider row Kandev recorded without a clone URL:
// only the host, project, and repository name identify it.
func identityRepository(id, host, project, name string) pluginsdk.Repository {
	return pluginsdk.Repository{
		ID: id, WorkspaceID: "ws-1", ProviderID: "azure_devops",
		ProviderHost: host, OwnerOrProject: project, ProviderName: name,
	}
}

func TestResolveGitCredentialForARepositoryRecordedWithoutACloneURL(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	h.host.AddRepository(identityRepository("repo-1", "https://dev.azure.com", "Platform Team", "api"))
	response, err := h.runtime.ResolveGitCredential(context.Background(), credentialRequest())
	if err != nil {
		t.Fatalf("ResolveGitCredential() error = %v", err)
	}
	if response.Secret != "good-pat" {
		t.Fatalf("credential = %+v", response)
	}
}

func TestResolveGitCredentialRejectsAMismatchedRepositoryIdentity(t *testing.T) {
	cases := map[string]pluginsdk.Repository{
		"no identity":        {ID: "repo-1", WorkspaceID: "ws-1", ProviderID: "azure_devops"},
		"no provider host":   identityRepository("repo-1", "", "Platform Team", "api"),
		"another host":       identityRepository("repo-1", "https://evil.example", "Platform Team", "api"),
		"another project":    identityRepository("repo-1", "https://dev.azure.com", "Other", "api"),
		"another repository": identityRepository("repo-1", "https://dev.azure.com", "Platform Team", "web"),
	}
	for name, repository := range cases {
		h := newHarness(t)
		h.connect("ws-1")
		h.host.AddRepository(repository)
		if response, err := h.runtime.ResolveGitCredential(context.Background(), credentialRequest()); err == nil {
			t.Fatalf("%s: credential issued: %+v", name, response)
		}
	}
}
