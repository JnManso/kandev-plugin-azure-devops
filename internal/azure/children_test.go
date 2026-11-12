package azure

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestChildLinkQueriesChunkParentIDs(t *testing.T) {
	ids := make([]int, 450)
	for index := range ids {
		ids[index] = index + 1
	}
	queries := ChildLinkQueries(ids)
	if len(queries) != 3 {
		t.Fatalf("queries = %d, want 3", len(queries))
	}
	first := queries[0]
	for _, want := range []string{
		"FROM WorkItemLinks",
		"[Source].[System.Id] IN (1,2,3,",
		",200)",
		"[System.Links.LinkType] = 'System.LinkTypes.Hierarchy-Forward'",
		"MODE (MustContain)",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("first query %q lacks %q", first, want)
		}
	}
	if strings.Contains(first, ",201") || !strings.Contains(queries[2], "IN (401,") || !strings.Contains(queries[2], ",450)") {
		t.Errorf("chunks = %q / %q", first, queries[2])
	}
	if got := ChildLinkQueries(nil); len(got) != 0 {
		t.Errorf("no parents = %v", got)
	}
}

func TestChildIDsKeepsLinkOrderAndBoundsTheCount(t *testing.T) {
	links := []SprintRelation{{Source: 1, Target: 11}, {Source: 2, Target: 21}, {Source: 1, Target: 11}, {Source: 1, Target: 12}}
	ids, truncated := ChildIDs(links, 10)
	if !reflect.DeepEqual(ids, []int{11, 21, 12}) || truncated {
		t.Fatalf("ChildIDs = %v, %v", ids, truncated)
	}
	ids, truncated = ChildIDs(links, 2)
	if !reflect.DeepEqual(ids, []int{11, 21}) || !truncated {
		t.Fatalf("bounded ChildIDs = %v, %v", ids, truncated)
	}
}

func TestBuildBoardChildrenOrdersAndFiltersChildren(t *testing.T) {
	categories := StateCategories{}
	categories.Add("Platform", "Task", "Closed", "Completed")
	categories.Add("Platform", "Task", "Active", "InProgress")
	categories.Add("Platform", "Task", "Removed", "Removed")
	task := func(id int, state string, priority float64) WorkItem {
		item := sprintItem(id, "Task", state, "", priority)
		item.Project = "Platform"
		return item
	}
	items := map[int]WorkItem{
		11: task(11, "Active", 3),
		12: task(12, "closed", 1),
		13: task(13, "Removed", 2),
		21: task(21, "Removed", 1),
	}
	links := []SprintRelation{
		{Source: 1, Target: 11}, {Source: 1, Target: 12}, {Source: 1, Target: 13}, {Source: 1, Target: 14},
		{Source: 2, Target: 21}, {Source: 9, Target: 11},
	}
	got := BuildBoardChildren([]int{3, 2, 1}, links, items, categories, "Platform")
	if len(got) != 1 || got[0].ParentID != 1 {
		t.Fatalf("children = %+v", got)
	}
	var order []string
	for _, child := range got[0].Items {
		order = append(order, fmt.Sprintf("%d:%s", child.ID, child.StateCategory))
	}
	if want := []string{"12:Completed", "11:InProgress"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("children of 1 = %v, want %v", order, want)
	}
}

func TestStateCategoriesFallBackToTheBoardProject(t *testing.T) {
	categories := StateCategories{}
	categories.Add("Platform", "User Story", "Resolved", "Resolved")
	item := WorkItem{Type: "user story", State: "RESOLVED"}
	if got := categories.Of(item, "platform"); got != "Resolved" {
		t.Fatalf("category = %q", got)
	}
	if got := categories.Of(WorkItem{Type: "Bug", State: "New"}, "Platform"); got != "" {
		t.Fatalf("unknown category = %q", got)
	}
}
