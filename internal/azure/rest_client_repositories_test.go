package azure

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListAllRepositoriesAndGetRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/acme/_apis/git/repositories":
			_, _ = w.Write([]byte(`{"value":[{"id":"r1","name":"api","defaultBranch":"refs/heads/main","isDisabled":true,"project":{"id":"p1","name":"Platform"}}]}`))
		case "/acme/Platform%20Team/_apis/git/repositories/api":
			_, _ = w.Write([]byte(`{"id":"r2","name":"api","defaultBranch":"refs/heads/develop","project":{"id":"p2","name":"Platform Team"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestRESTClient(t, server, "pat")

	all, err := client.ListAllRepositories(t.Context())
	if err != nil {
		t.Fatalf("ListAllRepositories() error = %v", err)
	}
	if len(all) != 1 || all[0].ID != "r1" || !all[0].IsDisabled || all[0].DefaultBranch != "main" {
		t.Fatalf("ListAllRepositories() = %+v", all)
	}
	repository, err := client.GetRepository(t.Context(), "Platform Team", "api")
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}
	if repository.ID != "r2" || repository.ProjectName != "Platform Team" || repository.DefaultBranch != "develop" {
		t.Fatalf("GetRepository() = %+v", repository)
	}
	if _, err := client.GetRepository(t.Context(), "Platform", "missing"); err == nil {
		t.Fatal("GetRepository() for a missing repository returned no error")
	}
}
