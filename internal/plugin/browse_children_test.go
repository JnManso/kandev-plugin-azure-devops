package plugin

import (
	"errors"
	"testing"
)

func TestBoardChildrenListsSummarizedChildren(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	if status, _ := h.action("ws-1", "boards.children", map[string]any{"ids": []int{101}}); status != 400 {
		t.Fatalf("missing project status = %d, want 400", status)
	}
	status, body := h.action("ws-1", "boards.children", map[string]any{"projectId": "p1", "ids": []int{101, 102}})
	groups, _ := body["children"].([]any)
	if status != 200 || len(groups) != 2 || body["truncated"] != false {
		t.Fatalf("boards.children status = %d body = %v", status, body)
	}
	child := groups[0].(map[string]any)["items"].([]any)[0]
	if field(groups[0], "parentId") != float64(101) || field(child, "title") != "Rotate the signing key" ||
		field(child, "stateCategory") != "Completed" || field(child, "fields") != nil {
		t.Fatalf("first group = %v", groups[0])
	}
}

func TestBoardChildrenBoundsTheParents(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	status, body := h.action("ws-1", "boards.children", map[string]any{"projectId": "p1", "ids": []int{}})
	if status != 200 || flatten(body["children"]) != `[]` {
		t.Fatalf("no parents status = %d body = %v", status, body)
	}
	ids := make([]int, maxChildParents+1)
	for index := range ids {
		ids[index] = index + 1
	}
	if status, _ := h.action("ws-1", "boards.children", map[string]any{"projectId": "p1", "ids": ids}); status != 400 {
		t.Fatalf("too many parents status = %d, want 400", status)
	}
}

func TestBoardChildrenReportsAzureFailures(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	h.azure.childrenErr = errors.New("boom")
	if status, _ := h.action("ws-1", "boards.children", map[string]any{"projectId": "p1", "ids": []int{101}}); status == 200 {
		t.Fatalf("failed read status = %d", status)
	}
}
