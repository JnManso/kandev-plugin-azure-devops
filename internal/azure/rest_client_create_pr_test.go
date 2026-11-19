package azure

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTClientCreatePullRequest(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/acme/project-1/_apis/git/repositories/repo-1/pullrequests" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pullRequestId":43,"title":"Add retries","status":"active","isDraft":true,"sourceRefName":"refs/heads/feature/retries","targetRefName":"refs/heads/main","repository":{"id":"repo-1","name":"api","webUrl":"https://dev.azure.com/acme/Platform/_git/api","project":{"id":"project-1","name":"Platform"}}}`))
	}))
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server, "pat")
	pr, err := client.CreatePullRequest(context.Background(), "project-1", "repo-1", CreatePullRequestRequest{
		SourceBranch: "feature/retries", TargetBranch: "refs/heads/main", Title: " Add retries ", Description: "Body",
		IsDraft: true, WorkItemIDs: []int{101},
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.ID != 43 || pr.WebURL != "https://dev.azure.com/acme/Platform/_git/api/pullrequest/43" || pr.SourceBranch != "feature/retries" {
		t.Fatalf("pull request = %+v", pr)
	}
	if got["sourceRefName"] != "refs/heads/feature/retries" || got["targetRefName"] != "refs/heads/main" ||
		got["title"] != "Add retries" || got["isDraft"] != true {
		t.Fatalf("request body = %+v", got)
	}
	refs, _ := got["workItemRefs"].([]any)
	if len(refs) != 1 || refs[0].(map[string]any)["id"] != "101" {
		t.Fatalf("work item refs = %+v", got["workItemRefs"])
	}
}

func TestRESTClientCreatePullRequestRequiresBranchesAndTitle(t *testing.T) {
	client := NewRESTClient("https://dev.azure.com/acme", NewPATAuthorizer("pat"), nil)
	_, err := client.CreatePullRequest(context.Background(), "p", "r", CreatePullRequestRequest{SourceBranch: "a", TargetBranch: "b"})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
}

func TestMockClientCreatePullRequest(t *testing.T) {
	client, err := NewSeededMockClient()
	if err != nil {
		t.Fatal(err)
	}
	pr, err := client.CreatePullRequest(context.Background(), "Platform", "api", CreatePullRequestRequest{
		SourceBranch: "feature/x", TargetBranch: "main", Title: "X",
	})
	if err != nil || pr.ID <= 42 || pr.RepositoryName != "api" || pr.Status != "active" {
		t.Fatalf("CreatePullRequest = %+v, %v", pr, err)
	}
	again, err := client.GetPullRequest(context.Background(), "", "", pr.ID)
	if err != nil || again.Title != "X" {
		t.Fatalf("GetPullRequest = %+v, %v", again, err)
	}
	if _, err := client.CreatePullRequest(context.Background(), "Platform", "missing", CreatePullRequestRequest{}); err == nil {
		t.Fatal("expected an unknown repository to fail")
	}
}
