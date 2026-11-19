package azure

import "context"

// ListChildren serves the seeded parent-to-child links, with state
// categories from the seeded per-type map.
func (c *MockClient) ListChildren(_ context.Context, projectID string, parentIDs []int) (*BoardChildren, error) {
	state := c.snapshot()
	parents := make(map[int]bool, len(parentIDs))
	for _, id := range parentIDs {
		parents[id] = true
	}
	var links []SprintRelation
	for _, link := range state.ChildLinks {
		if parents[link.Source] {
			links = append(links, link)
		}
	}
	ids, truncated := ChildIDs(links, maxBoardChildren)
	known := map[int]WorkItem{}
	for _, item := range mockWorkItems(state) {
		known[item.ID] = item
	}
	items := make(map[int]WorkItem, len(ids))
	categories := StateCategories{}
	for _, id := range ids {
		item, ok := known[id]
		if !ok {
			continue
		}
		items[id] = item
		project := item.Project
		if project == "" {
			project = projectID
		}
		for name, category := range state.StateCategories[item.Type] {
			categories.Add(project, item.Type, name, category)
		}
	}
	return &BoardChildren{Children: BuildBoardChildren(parentIDs, links, items, categories, projectID), Truncated: truncated}, nil
}

// mockWorkItems are every seeded work item: search results, sprint items,
// and board card children.
func mockWorkItems(state MockState) []WorkItem {
	all := append([]WorkItem(nil), state.WorkItems...)
	all = append(all, state.SprintItems...)
	return append(all, state.ChildItems...)
}
