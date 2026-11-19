package watches

import (
	"context"
	"fmt"

	"kandev-plugin-azure-devops/internal/azure"
)

// creator offers a poll's matches to createTask until the watch runs out of
// capacity. Items the current generation already handled are skipped before
// they are read, so each poll reaches matches past the handled ones.
type creator struct {
	service  *Service
	watch    *Watch
	reserved map[string]bool
	capacity int // -1 is unlimited
	created  int
	failure  error
	stopped  error
}

func (s *Service) createForMatches(ctx context.Context, client Client, watch *Watch, capacity int) (int, error) {
	reserved, err := s.deps.Store.ReservedKeys(ctx, *watch)
	if err != nil {
		return 0, err
	}
	c := &creator{service: s, watch: watch, reserved: reserved, capacity: capacity}
	if watch.Kind == KindWorkItem {
		err = c.workItems(ctx, client)
	} else {
		err = c.pullRequests(ctx, client)
	}
	if c.stopped != nil {
		return c.created, c.stopped
	}
	if err != nil {
		return c.created, fmt.Errorf("Azure DevOps query failed: %w", err)
	}
	return c.created, c.failure
}

// done reports whether the poll should stop offering matches.
func (c *creator) done(ctx context.Context) bool {
	return c.capacity == 0 || c.stopped != nil || ctx.Err() != nil
}

func (c *creator) offer(ctx context.Context, match Match) {
	if c.done(ctx) || c.reserved[match.Key] {
		return
	}
	ok, err := c.service.createTask(ctx, c.watch, match)
	if err != nil {
		if !c.watch.Enabled {
			c.stopped = err
		} else {
			c.failure = err
		}
		return
	}
	if ok {
		c.created++
		if c.capacity > 0 {
			c.capacity--
		}
	}
}

// workItems reads the query's IDs, then only the unhandled items, one batch
// at a time.
func (c *creator) workItems(ctx context.Context, client Client) error {
	ids, err := client.QueryWorkItemIDs(ctx, c.watch.ProjectID, c.watch.WIQL, maxWatchCandidates)
	if err != nil {
		return err
	}
	pending := make([]int, 0, len(ids))
	for _, id := range ids {
		if !c.reserved[workItemKey(id)] {
			pending = append(pending, id)
		}
	}
	for start := 0; start < len(pending) && !c.done(ctx); start += maxMatchesPerPoll {
		items, err := client.GetWorkItems(ctx, c.watch.ProjectID, pending[start:min(start+maxMatchesPerPoll, len(pending))])
		if err != nil {
			return err
		}
		for _, item := range items {
			c.offer(ctx, WorkItemMatch(item))
		}
	}
	return nil
}

// pullRequests pages through the matching pull requests.
func (c *creator) pullRequests(ctx context.Context, client Client) error {
	creatorID, err := resolveIdentity(ctx, client, c.watch.CreatorID)
	if err != nil {
		return err
	}
	reviewerID, err := resolveIdentity(ctx, client, c.watch.ReviewerID)
	if err != nil {
		return err
	}
	filter := azure.PullRequestFilter{
		ProjectID: c.watch.ProjectID, RepositoryID: c.watch.AzureRepositoryID, Status: firstNonEmpty(c.watch.Status, "active"),
		CreatorID: creatorID, ReviewerID: reviewerID, Top: maxMatchesPerPoll,
	}
	for filter.Skip = 0; filter.Skip < maxWatchCandidates && !c.done(ctx); filter.Skip += maxMatchesPerPoll {
		page, err := client.ListPullRequests(ctx, filter)
		if err != nil {
			return err
		}
		if page == nil {
			return nil
		}
		for _, pr := range page.Items {
			c.offer(ctx, PullRequestMatch(pr))
		}
		if len(page.Items) < maxMatchesPerPoll {
			return nil
		}
	}
	return nil
}
