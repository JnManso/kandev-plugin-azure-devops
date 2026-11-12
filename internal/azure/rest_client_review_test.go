package azure

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTClientReadsTheCommentContinuationTokenFromTheBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"comments":[{"id":1,"text":"hi"}],"continuationToken":"next-page"}`))
	}))
	t.Cleanup(server.Close)
	page, err := newTestRESTClient(t, server, "pat").ListWorkItemComments(t.Context(), "project-1", 101, "")
	if err != nil || page.ContinuationToken != "next-page" {
		t.Fatalf("page = %+v, %v", page, err)
	}
}

func TestRESTClientBranchExistsUsesAnExactRefFilter(t *testing.T) {
	var filters []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filters = append(filters, r.URL.Query().Get("filter"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"refs/heads/feature/x-old"},{"name":"refs/heads/feature/x"}]}`))
	}))
	t.Cleanup(server.Close)
	client := newTestRESTClient(t, server, "pat")
	found, err := client.BranchExists(t.Context(), "project-1", "repo-1", "feature/x")
	if err != nil || !found {
		t.Fatalf("BranchExists = %v, %v", found, err)
	}
	found, err = client.BranchExists(t.Context(), "project-1", "repo-1", "feature/x-old2")
	if err != nil || found {
		t.Fatalf("BranchExists(missing) = %v, %v", found, err)
	}
	if len(filters) != 2 || filters[0] != "heads/feature/x" || filters[1] != "heads/feature/x-old2" {
		t.Fatalf("filters = %v", filters)
	}
}
