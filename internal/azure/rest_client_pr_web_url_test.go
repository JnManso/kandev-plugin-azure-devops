package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Azure DevOps pull request responses carry the repository's API url but no
// webUrl, so the browser address is built from the organization, project,
// and repository names.
const realPullRequestJSON = `{"pullRequestId":42,"title":"Ship it","status":"active",` +
	`"url":"https://dev.azure.com/acme/project-1/_apis/git/repositories/repo-1/pullRequests/42",` +
	`"repository":{"id":"repo-1","name":"Widget Service","url":"https://dev.azure.com/acme/project-1/_apis/git/repositories/repo-1",` +
	`"project":{"id":"project-1","name":"My Project"}}}`

func TestRESTClientBuildsPullRequestWebURLWithoutRepositoryWebURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/acme/project-1/_apis/git/repositories/repo-1/pullrequests":
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(realPullRequestJSON))
				return
			}
			_, _ = w.Write([]byte(`{"count":1,"value":[` + realPullRequestJSON + `]}`))
		case "/acme/project-1/_apis/git/repositories/repo-1/pullrequests/42":
			_, _ = w.Write([]byte(realPullRequestJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := newTestRESTClient(t, server, "pat")
	const want = "https://dev.azure.com/acme/My%20Project/_git/Widget%20Service/pullrequest/42"

	page, err := client.ListPullRequests(context.Background(), PullRequestFilter{ProjectID: "project-1", RepositoryID: "repo-1"})
	if err != nil || len(page.Items) != 1 || page.Items[0].WebURL != want {
		t.Fatalf("ListPullRequests = %+v, %v; want webUrl %s", page, err, want)
	}
	pr, err := client.GetPullRequest(context.Background(), "project-1", "repo-1", 42)
	if err != nil || pr.WebURL != want {
		t.Fatalf("GetPullRequest = %+v, %v; want webUrl %s", pr, err, want)
	}
	created, err := client.CreatePullRequest(context.Background(), "project-1", "repo-1",
		CreatePullRequestRequest{SourceBranch: "feature", TargetBranch: "main", Title: "Ship it"})
	if err != nil || created.WebURL != want {
		t.Fatalf("CreatePullRequest = %+v, %v; want webUrl %s", created, err, want)
	}
}
