package plugin

import (
	"context"
	"errors"
	"strings"
	"time"

	"kandev-plugin-azure-devops/internal/azure"
)

// listTeams lists the project's teams and names those of the connected
// identity. When Azure DevOps cannot list the identity's teams, every team is
// still listed and none is marked as the user's.
func listTeams(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireProject(); err != nil {
		return nil, err
	}
	var teams []azure.Team
	var mine []string
	var teamsErr, mineErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		mine, mineErr = client.ListMyTeamIDs(ctx, in.ProjectID)
	}()
	teams, teamsErr = client.ListTeams(ctx, in.ProjectID)
	<-done
	if teamsErr != nil {
		return nil, teamsErr
	}
	if mineErr != nil {
		mine = nil
	}
	return map[string]any{"teams": nonNil(teams), "mineIds": nonNil(mine)}, nil
}

func (in browseInput) requireTeam() error {
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.TeamID) == "" {
		return invalidArgument(errors.New("a project and a team are required"))
	}
	return nil
}

func getTeamSettings(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireTeam(); err != nil {
		return nil, err
	}
	return client.GetTeamSettings(ctx, in.ProjectID, in.TeamID)
}

func getSprintBoard(ctx context.Context, client Client, in browseInput) (any, error) {
	if err := in.requireTeam(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.IterationID) == "" {
		return nil, invalidArgument(errors.New("a sprint is required"))
	}
	board, err := client.GetSprintBoard(ctx, in.ProjectID, in.TeamID, in.IterationID, time.Now())
	if err != nil {
		return nil, err
	}
	for index := range board.Rows {
		row := &board.Rows[index]
		if row.Parent != nil {
			parent := summary(*row.Parent)
			row.Parent = &parent
		}
		for task := range row.Tasks {
			row.Tasks[task].WorkItem = summary(row.Tasks[task].WorkItem)
		}
	}
	return board, nil
}
