package azure

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestMockListsBoardCardChildren(t *testing.T) {
	client := seededMock(t)
	result, err := client.ListChildren(context.Background(), "project-1", []int{102, 101, 999})
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	var got []string
	for _, group := range result.Children {
		for _, child := range group.Items {
			got = append(got, fmt.Sprintf("%d>%d:%s", group.ParentID, child.ID, child.StateCategory))
		}
	}
	want := []string{"102>304:Proposed", "102>101:InProgress", "101>301:Completed", "101>302:InProgress"}
	if !reflect.DeepEqual(got, want) || result.Truncated {
		t.Fatalf("children = %v (truncated %v), want %v", got, result.Truncated, want)
	}
	if item, err := client.GetWorkItem(context.Background(), "project-1", 301); err != nil || item.Title != "Rotate the signing key" {
		t.Fatalf("child detail = %+v, %v", item, err)
	}
}
