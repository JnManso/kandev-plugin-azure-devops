package plugin

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func TestSearchWorkItemAndPullRequestReferences(t *testing.T) {
	h := linksHarness(t)
	response, err := h.runtime.SearchEntityReferences(context.Background(), &pluginsdk.SearchEntityReferencesRequest{
		Source: WorkItemReferenceSource, WorkspaceID: "ws-1", Query: "token",
	})
	if err != nil || len(response.Candidates) == 0 || response.Candidates[0].ProviderLocalID != "Ops#101" && response.Candidates[0].ProviderLocalID != "Platform Team#101" {
		t.Fatalf("work item search = %+v, %v", response, err)
	}
	response, err = h.runtime.SearchEntityReferences(context.Background(), &pluginsdk.SearchEntityReferencesRequest{
		Source: PullRequestReferenceSource, WorkspaceID: "ws-1", Query: "Pull request 2",
	})
	if err != nil {
		t.Fatalf("pull request search: %v", err)
	}
	for _, candidate := range response.Candidates {
		if candidate.Title != "Pull request 2" {
			t.Fatalf("unexpected candidate %+v", candidate)
		}
	}
	if _, err := h.runtime.SearchEntityReferences(context.Background(), &pluginsdk.SearchEntityReferencesRequest{Source: "other", WorkspaceID: "ws-1", Query: "x"}); err == nil {
		t.Fatal("expected an unknown source to fail")
	}
}

func TestAuthorizeReferencesRereadsTheItem(t *testing.T) {
	h := linksHarness(t)
	authorize := func(source, key string) bool {
		response, err := h.runtime.AuthorizeEntityReference(context.Background(), &pluginsdk.AuthorizeEntityReferenceRequest{
			Source: source, WorkspaceID: "ws-1", Purpose: "submission", Reference: map[string]any{"id": key, "key": key},
		})
		if err != nil {
			t.Fatalf("authorize error = %v", err)
		}
		return response.Allowed
	}
	if !authorize(WorkItemReferenceSource, "Platform Team#101") {
		t.Fatal("work item 101 was denied")
	}
	if authorize(WorkItemReferenceSource, "Platform Team#999") || authorize(WorkItemReferenceSource, "#101") {
		t.Fatal("a missing or project-less work item was allowed")
	}
	if !authorize(PullRequestReferenceSource, "Platform Team/api!42") {
		t.Fatal("pull request 42 was denied")
	}
	if authorize(PullRequestReferenceSource, "Platform Team/api!77") {
		t.Fatal("a missing pull request was allowed")
	}
	response, _ := h.runtime.AuthorizeEntityReference(context.Background(), &pluginsdk.AuthorizeEntityReferenceRequest{
		Source: WorkItemReferenceSource, WorkspaceID: "ws-1", Purpose: "submission", Reference: map[string]any{"id": "P#1", "key": "P#2"},
	})
	if response.Allowed {
		t.Fatal("a mismatched id and key was allowed")
	}
}
