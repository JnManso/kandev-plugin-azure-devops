package plugin

import (
	"context"
	"time"

	"kandev-plugin-azure-devops/internal/azure"
)

// sprintMock serves the team and sprint reads from the seeded mock
// organization, behind the fake client's authorization.
func sprintMock() *azure.MockClient {
	client, err := azure.NewSeededMockClient()
	if err != nil {
		panic(err)
	}
	return client
}

func (c *fakeClient) ListMyTeamIDs(ctx context.Context, projectID string) ([]string, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	if c.azure.myTeamsErr != nil {
		return nil, c.azure.myTeamsErr
	}
	return []string{"team-1"}, nil
}

func (c *fakeClient) GetTeamSettings(ctx context.Context, projectID, teamID string) (*azure.TeamSettings, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return sprintMock().GetTeamSettings(ctx, projectID, teamID)
}

func (c *fakeClient) GetSprintBoard(ctx context.Context, projectID, teamID, iterationID string, today time.Time) (*azure.SprintBoard, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return sprintMock().GetSprintBoard(ctx, projectID, teamID, iterationID, today)
}

func (c *fakeClient) ListChildren(ctx context.Context, projectID string, parentIDs []int) (*azure.BoardChildren, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	if c.azure.childrenErr != nil {
		return nil, c.azure.childrenErr
	}
	return sprintMock().ListChildren(ctx, projectID, parentIDs)
}

func (c *fakeClient) ListPullRequestPeople(ctx context.Context, projectID, repositoryID string) (*azure.PullRequestPeople, error) {
	if err := c.authorize(); err != nil {
		return nil, err
	}
	return sprintMock().ListPullRequestPeople(ctx, projectID, repositoryID)
}
