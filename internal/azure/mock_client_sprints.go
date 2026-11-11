package azure

import (
	"context"
	"strings"
	"time"
)

// MockSprint is the taskboard layout and work-item list of one mock sprint.
type MockSprint struct {
	Columns   []TaskboardColumn `json:"columns"`
	Relations []SprintRelation  `json:"relations"`
}

var mockWorkingDays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}

func mockWIQLValue(value string) string { return strings.ReplaceAll(value, "''", "'") }

func mockAreaMatches(wiql, areaPath string) bool {
	matches := mockWIQLArea.FindAllStringSubmatch(wiql, -1)
	if len(matches) == 0 {
		return true
	}
	for _, match := range matches {
		path := mockWIQLValue(match[2])
		if strings.EqualFold(areaPath, path) ||
			(match[1] == "UNDER" && strings.HasPrefix(strings.ToLower(areaPath), strings.ToLower(path)+`\`)) {
			return true
		}
	}
	return false
}

func (c *MockClient) ListMyTeamIDs(context.Context, string) ([]string, error) {
	return append([]string{}, c.snapshot().MyTeamIDs...), nil
}

func (c *MockClient) GetTeamSettings(_ context.Context, _, teamID string) (*TeamSettings, error) {
	settings, ok := c.snapshot().TeamSettings[teamID]
	if !ok {
		return &TeamSettings{AreaField: "System.AreaPath", Areas: []TeamArea{}, Iterations: []Iteration{}}, nil
	}
	return &settings, nil
}

func (c *MockClient) GetSprintBoard(_ context.Context, _, teamID, iterationID string, today time.Time) (*SprintBoard, error) {
	state := c.snapshot()
	var iteration Iteration
	for _, candidate := range state.TeamSettings[teamID].Iterations {
		if candidate.ID == iterationID {
			iteration = candidate
		}
	}
	if iteration.ID == "" {
		return nil, &APIError{StatusCode: 404, Endpoint: "teamsettings/iterations/" + iterationID, Body: "iteration not found"}
	}
	sprint := state.Sprints[iterationID]
	items := make(map[int]WorkItem, len(state.SprintItems))
	for _, item := range state.SprintItems {
		items[item.ID] = item
	}
	columns := sprint.Columns
	if columns == nil {
		columns = []TaskboardColumn{}
	}
	rows := BuildSprintRows(sprint.Relations, items, columns)
	return &SprintBoard{
		Iteration:       iteration,
		WorkingDaysLeft: workingDaysLeftFor(iteration, today, mockWorkingDays, nil),
		Columns:         columns,
		Rows:            rows,
		People:          SprintPeople(rows),
	}, nil
}
