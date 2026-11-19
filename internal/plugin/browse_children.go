package plugin

import (
	"context"
	"errors"
	"fmt"

	"kandev-plugin-azure-devops/internal/azure"
)

// maxChildParents bounds the cards one boards.children call reads children
// for, which keeps the request within its body limit.
const maxChildParents = 1000

func listBoardChildren(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireProject(); err != nil {
		return nil, err
	}
	if len(in.IDs) > maxChildParents {
		return nil, invalidArgument(fmt.Errorf("at most %d cards are allowed", maxChildParents))
	}
	parents := make([]int, 0, len(in.IDs))
	for _, id := range in.IDs {
		if id <= 0 {
			return nil, invalidArgument(errors.New("card ids must be positive"))
		}
		parents = append(parents, id)
	}
	if len(parents) == 0 {
		return &azure.BoardChildren{Children: []azure.CardChildren{}}, nil
	}
	result, err := client.ListChildren(ctx, in.ProjectID, parents)
	if err != nil {
		return nil, err
	}
	for group := range result.Children {
		for index := range result.Children[group].Items {
			child := &result.Children[group].Items[index]
			child.WorkItem = summary(child.WorkItem)
		}
	}
	return result, nil
}
