package plugin

import (
	"testing"
)

func repositoryNames(body map[string]any) []string {
	var names []string
	repositories, _ := field(body, "repositories").([]any)
	for _, repository := range repositories {
		names = append(names, field(repository, "owner_or_project").(string)+"/"+field(repository, "name").(string))
	}
	return names
}

func TestRepositoriesListSkipsDisabledSortsAndPages(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	status, body := h.action("ws-1", "repositories.list", map[string]any{"limit": 2})
	if status != 200 {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if got := repositoryNames(body); len(got) != 2 || got[0] != "Ops/infra" || got[1] != "Platform Team/api" {
		t.Fatalf("first page = %v", got)
	}
	first := field(body, "repositories").([]any)[0]
	if field(first, "provider_id") != "azure_devops" || field(first, "provider_host") != "https://dev.azure.com" ||
		field(first, "provider_scope") != "acme/Ops" || field(first, "provider_repository_id") != "guid-ops" ||
		field(first, "clone_url") != "https://dev.azure.com/acme/Ops/_git/infra" {
		t.Fatalf("descriptor = %v", first)
	}
	cursor, _ := field(body, "next_cursor").(string)
	if cursor == "" {
		t.Fatalf("missing next cursor: %v", body)
	}
	_, body = h.action("ws-1", "repositories.list", map[string]any{"limit": 2, "cursor": cursor})
	if got := repositoryNames(body); len(got) != 1 || got[0] != "Platform Team/web" || field(body, "next_cursor") != nil {
		t.Fatalf("second page = %v (%v)", got, body)
	}
}

func TestRepositoriesListFiltersByProjectOrName(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	_, body := h.action("ws-1", "repositories.list", map[string]any{"query": "PLATFORM"})
	if got := repositoryNames(body); len(got) != 2 {
		t.Fatalf("query results = %v", got)
	}
}

func TestRepositoriesListRejectsACursorFromAnOlderConnection(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	_, body := h.action("ws-1", "repositories.list", map[string]any{"limit": 1})
	cursor := field(body, "next_cursor").(string)
	h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "good-pat-2"})
	h.azure.validPAT = "good-pat-2"
	if status, _ := h.action("ws-1", "repositories.list", map[string]any{"limit": 1, "cursor": cursor}); status == 200 {
		t.Fatal("a cursor from the previous credential generation was accepted")
	}
}

func TestRepositoriesInspectResolvesEveryAzureFormInTheConnectedOrganization(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	for _, url := range []string{
		"https://dev.azure.com/acme/Platform%20Team/_git/api",
		"https://acme@dev.azure.com/acme/Platform%20Team/_git/api",
		"git@ssh.dev.azure.com:v3/acme/Platform%20Team/api",
		"https://acme.visualstudio.com/Platform%20Team/_git/api",
	} {
		status, body := h.action("ws-1", "repositories.inspect", map[string]any{"url": url})
		repository := field(body, "repository")
		if status != 200 || field(repository, "provider_repository_id") != "guid-api" || field(repository, "default_branch") != "main" ||
			field(repository, "clone_url") != "https://dev.azure.com/acme/Platform%20Team/_git/api" {
			t.Fatalf("inspect(%s) status = %d body = %v", url, status, body)
		}
	}
}

func TestRepositoriesInspectDistinguishesNoMatchFromNotFound(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	for _, url := range []string{"https://github.com/acme/api", "https://dev.azure.com/other/P/_git/api"} {
		if status, body := h.action("ws-1", "repositories.inspect", map[string]any{"url": url}); status != 200 || field(body, "matched") != false {
			t.Fatalf("inspect(%s) status = %d body = %v, want matched false", url, status, body)
		}
	}
	if status, _ := h.action("ws-1", "repositories.inspect", map[string]any{"url": "https://dev.azure.com/acme/Platform%20Team/_git/missing"}); status != 404 {
		t.Fatalf("missing repository status = %d, want 404", status)
	}
}

func TestRepositoriesBranchesResolveByIdentityOrLegacyProjectAndName(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	for _, repository := range []map[string]any{
		{"provider_scope": "acme/Platform Team", "provider_repository_id": "guid-api"},
		{"owner_or_project": "Platform Team", "name": "api"},
	} {
		status, body := h.action("ws-1", "repositories.branches", map[string]any{"repository": repository})
		branches, _ := field(body, "branches").([]any)
		if status != 200 || len(branches) != 2 || field(branches[0], "name") != "main" || field(branches[0], "is_default") != true {
			t.Fatalf("branches(%v) status = %d body = %v", repository, status, body)
		}
	}
}

func TestRepositoriesBranchesRejectAnIdentityFromAnotherOrganization(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	status, _ := h.action("ws-1", "repositories.branches", map[string]any{"repository": map[string]any{
		"provider_scope": "other/Platform Team", "provider_repository_id": "guid-api",
	}})
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
}
