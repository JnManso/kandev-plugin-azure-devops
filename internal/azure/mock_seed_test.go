package azure

import (
	"context"
	"testing"
)

func TestSeededMockClientServesTheDemoOrganization(t *testing.T) {
	client, err := NewSeededMockClient()
	if err != nil {
		t.Fatalf("NewSeededMockClient() error = %v", err)
	}
	snapshot, err := client.GetBoardSnapshot(context.Background(), "project-1", "team-1", "board-1")
	if err != nil || len(snapshot.Items) != 1 || snapshot.Items[0].Title != "Handle token rotation" {
		t.Fatalf("board = %+v, %v", snapshot, err)
	}
	repository, err := client.GetRepository(context.Background(), "Platform", "api")
	if err != nil || repository.ID != "azure-repo-1" {
		t.Fatalf("repository = %+v, %v", repository, err)
	}
}
