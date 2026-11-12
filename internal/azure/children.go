package azure

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// maxBoardChildren bounds the children read for one board so the action
// response stays small.
const maxBoardChildren = 1000

// childLinkChunk is the most parent IDs one link query names.
const childLinkChunk = 200

// stateCategoryRemoved is the state category of removed work items, which
// are neither counted nor listed as children.
const stateCategoryRemoved = "Removed"

// ChildCard is a child work item with the category of its state (Proposed,
// InProgress, Resolved, Completed).
type ChildCard struct {
	WorkItem
	StateCategory string `json:"stateCategory"`
}

// CardChildren are the children of one board card, in backlog order.
type CardChildren struct {
	ParentID int         `json:"parentId"`
	Items    []ChildCard `json:"items"`
}

// BoardChildren are the children of a board's cards. Truncated reports that
// more children exist than were read.
type BoardChildren struct {
	Children  []CardChildren `json:"children"`
	Truncated bool           `json:"truncated"`
}

// StateCategories maps a project, work-item type, and state to the state's
// category, ignoring case.
type StateCategories map[string]string

func stateCategoryKey(project, itemType, state string) string {
	return strings.ToLower(project) + "\x00" + strings.ToLower(itemType) + "\x00" + strings.ToLower(state)
}

// Add records the category of one state of a project's work-item type.
func (c StateCategories) Add(project, itemType, state, category string) {
	c[stateCategoryKey(project, itemType, state)] = category
}

// Of is the category of the item's state, read from the item's project, else
// from the given project.
func (c StateCategories) Of(item WorkItem, fallbackProject string) string {
	project := item.Project
	if project == "" {
		project = fallbackProject
	}
	return c[stateCategoryKey(project, item.Type, item.State)]
}

// ChildLinkQueries are the WIQL link queries that list the parent-to-child
// links of the parents, at most childLinkChunk parents per query.
func ChildLinkQueries(parentIDs []int) []string {
	queries := make([]string, 0, (len(parentIDs)+childLinkChunk-1)/childLinkChunk)
	for start := 0; start < len(parentIDs); start += childLinkChunk {
		chunk := parentIDs[start:min(start+childLinkChunk, len(parentIDs))]
		ids := make([]string, len(chunk))
		for index, id := range chunk {
			ids[index] = strconv.Itoa(id)
		}
		queries = append(queries, fmt.Sprintf(
			"SELECT [System.Id] FROM WorkItemLinks WHERE [Source].[System.Id] IN (%s) AND [System.Links.LinkType] = '%s' MODE (MustContain)",
			strings.Join(ids, ","), hierarchyForward,
		))
	}
	return queries
}

// ChildIDs are the distinct link targets in link order, at most limit, and
// whether more existed.
func ChildIDs(links []SprintRelation, limit int) ([]int, bool) {
	ids := make([]int, 0, min(len(links), limit))
	seen := map[int]bool{}
	for _, link := range links {
		if seen[link.Target] {
			continue
		}
		if len(ids) == limit {
			return ids, true
		}
		seen[link.Target] = true
		ids = append(ids, link.Target)
	}
	return ids, false
}

// BuildBoardChildren groups the read children under the parents, in the
// parents' order, sorting each group in backlog order and dropping removed
// children and parents left without children.
func BuildBoardChildren(parentIDs []int, links []SprintRelation, items map[int]WorkItem, categories StateCategories, project string) []CardChildren {
	byParent := map[int][]ChildCard{}
	seen := map[[2]int]bool{}
	for _, link := range links {
		item, ok := items[link.Target]
		key := [2]int{link.Source, link.Target}
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		category := categories.Of(item, project)
		if strings.EqualFold(category, stateCategoryRemoved) {
			continue
		}
		byParent[link.Source] = append(byParent[link.Source], ChildCard{WorkItem: item, StateCategory: category})
	}
	result := []CardChildren{}
	for _, parentID := range parentIDs {
		children := byParent[parentID]
		if len(children) == 0 {
			continue
		}
		sort.SliceStable(children, func(i, j int) bool { return backlogBefore(children[i].WorkItem, children[j].WorkItem) })
		result = append(result, CardChildren{ParentID: parentID, Items: children})
		delete(byParent, parentID)
	}
	return result
}
