package azure

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// ListChildren reads the children of the parents: their links in concurrent
// link queries, the children's fields, and the state categories of their
// work-item types.
func (c *RESTClient) ListChildren(ctx context.Context, projectID string, parentIDs []int) (*BoardChildren, error) {
	links, err := c.readChildLinks(ctx, projectID, parentIDs)
	if err != nil {
		return nil, err
	}
	ids, truncated := ChildIDs(links, maxBoardChildren)
	hydrated, err := c.hydrateWorkItems(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	items := make(map[int]WorkItem, len(hydrated))
	for _, item := range hydrated {
		items[item.ID] = item
	}
	categories, err := c.readStateCategories(ctx, projectID, hydrated)
	if err != nil {
		return nil, err
	}
	return &BoardChildren{
		Children:  BuildBoardChildren(parentIDs, links, items, categories, projectID),
		Truncated: truncated,
	}, nil
}

func (c *RESTClient) readChildLinks(ctx context.Context, projectID string, parentIDs []int) ([]SprintRelation, error) {
	queries := ChildLinkQueries(parentIDs)
	results := make([][]SprintRelation, len(queries))
	reads := make([]func() error, len(queries))
	endpoint := fmt.Sprintf("/%s/_apis/wit/wiql?api-version=%s", pathPart(projectID), restAPIVersion)
	for index, query := range queries {
		reads[index] = func() error {
			var response struct {
				WorkItemRelations []struct {
					Rel    *string `json:"rel"`
					Source *struct {
						ID int `json:"id"`
					} `json:"source"`
					Target struct {
						ID int `json:"id"`
					} `json:"target"`
				} `json:"workItemRelations"`
			}
			if err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]string{"query": query}, &response); err != nil {
				return err
			}
			for _, raw := range response.WorkItemRelations {
				if raw.Rel != nil && *raw.Rel == hierarchyForward && raw.Source != nil && raw.Target.ID > 0 {
					results[index] = append(results[index], SprintRelation{Source: raw.Source.ID, Target: raw.Target.ID})
				}
			}
			return nil
		}
	}
	if err := runConcurrently(reads...); err != nil {
		return nil, err
	}
	var links []SprintRelation
	for _, result := range results {
		links = append(links, result...)
	}
	return links, nil
}

// readStateCategories reads the states of each work-item type of the items,
// once per project and type, concurrently.
func (c *RESTClient) readStateCategories(ctx context.Context, projectID string, items []WorkItem) (StateCategories, error) {
	categories := StateCategories{}
	var mu sync.Mutex
	seen := map[string]bool{}
	var reads []func() error
	for _, item := range items {
		project := item.Project
		if project == "" {
			project = projectID
		}
		key := stateCategoryKey(project, item.Type, "")
		if seen[key] {
			continue
		}
		seen[key] = true
		itemType := item.Type
		reads = append(reads, func() error {
			states, err := c.readTypeStates(ctx, project, itemType)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			for _, state := range states {
				categories.Add(project, itemType, state.Name, state.Category)
			}
			return nil
		})
	}
	return categories, runConcurrently(reads...)
}
