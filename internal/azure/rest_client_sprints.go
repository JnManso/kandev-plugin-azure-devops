package azure

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxSprintCards bounds a sprint taskboard so its action response stays small.
const maxSprintCards = 1000

const hierarchyForward = "System.LinkTypes.Hierarchy-Forward"

// ListMyTeamIDs lists the IDs of the project's teams the connected identity
// belongs to.
func (c *RESTClient) ListMyTeamIDs(ctx context.Context, projectID string) ([]string, error) {
	type rawTeam struct {
		ID string `json:"id"`
	}
	endpoint := fmt.Sprintf("/_apis/projects/%s/teams?$mine=true&api-version=%s", pathPart(projectID), restAPIVersion)
	raws, err := listAllPages[rawTeam](ctx, c, endpoint)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(raws))
	for _, raw := range raws {
		ids = append(ids, raw.ID)
	}
	return ids, nil
}

type rawIteration struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Attributes struct {
		StartDate  string `json:"startDate"`
		FinishDate string `json:"finishDate"`
		TimeFrame  string `json:"timeFrame"`
	} `json:"attributes"`
}

func (raw rawIteration) iteration() Iteration {
	return Iteration{
		ID: raw.ID, Name: raw.Name, Path: raw.Path,
		Start: raw.Attributes.StartDate, Finish: raw.Attributes.FinishDate, TimeFrame: raw.Attributes.TimeFrame,
	}
}

func teamWorkEndpoint(projectID, teamID, path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return fmt.Sprintf("/%s/%s/_apis/work/%s%sapi-version=%s", pathPart(projectID), pathPart(teamID), path, separator, restAPIVersion)
}

// GetTeamSettings reads the team's areas and sprints.
func (c *RESTClient) GetTeamSettings(ctx context.Context, projectID, teamID string) (*TeamSettings, error) {
	var areas struct {
		Field struct {
			ReferenceName string `json:"referenceName"`
		} `json:"field"`
		Values []struct {
			Value           string `json:"value"`
			IncludeChildren bool   `json:"includeChildren"`
		} `json:"values"`
	}
	var iterations struct {
		Value []rawIteration `json:"value"`
	}
	err := runConcurrently(
		func() error {
			return c.doJSON(ctx, http.MethodGet, teamWorkEndpoint(projectID, teamID, "teamsettings/teamfieldvalues"), nil, &areas)
		},
		func() error {
			return c.doJSON(ctx, http.MethodGet, teamWorkEndpoint(projectID, teamID, "teamsettings/iterations"), nil, &iterations)
		},
	)
	if err != nil {
		return nil, err
	}
	settings := &TeamSettings{AreaField: areas.Field.ReferenceName, Areas: []TeamArea{}, Iterations: []Iteration{}}
	for _, value := range areas.Values {
		settings.Areas = append(settings.Areas, TeamArea{Path: value.Value, IncludeChildren: value.IncludeChildren})
	}
	for _, raw := range iterations.Value {
		settings.Iterations = append(settings.Iterations, raw.iteration())
	}
	return settings, nil
}

type sprintSources struct {
	iteration   rawIteration
	relations   []SprintRelation
	columns     []TaskboardColumn
	workingDays []time.Weekday
	daysOff     []DayRange
}

// GetSprintBoard reads a sprint and lays it out as the team's taskboard, with
// the working days left from today.
func (c *RESTClient) GetSprintBoard(ctx context.Context, projectID, teamID, iterationID string, today time.Time) (*SprintBoard, error) {
	sources, err := c.readSprintSources(ctx, projectID, teamID, iterationID)
	if err != nil {
		return nil, err
	}
	if len(sources.columns) == 0 {
		if sources.columns, err = c.defaultTaskboardColumns(ctx, projectID, teamID); err != nil {
			return nil, err
		}
	}
	ids := make([]int, 0, len(sources.relations))
	seen := map[int]bool{}
	for _, relation := range sources.relations {
		if !seen[relation.Target] {
			seen[relation.Target] = true
			ids = append(ids, relation.Target)
		}
	}
	truncated := len(ids) > maxSprintCards
	if truncated {
		ids = ids[:maxSprintCards]
	}
	hydrated, err := c.hydrateWorkItems(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	items := make(map[int]WorkItem, len(hydrated))
	for _, item := range hydrated {
		items[item.ID] = item
	}
	rows := BuildSprintRows(sources.relations, items, sources.columns)
	iteration := sources.iteration.iteration()
	return &SprintBoard{
		Iteration:       iteration,
		WorkingDaysLeft: workingDaysLeftFor(iteration, today, sources.workingDays, sources.daysOff),
		Columns:         sources.columns,
		Rows:            rows,
		People:          SprintPeople(rows),
		Truncated:       truncated,
	}, nil
}

// defaultTaskboardColumns builds the columns of a taskboard the team never
// customized, from the states of the task backlog level's work-item types.
func (c *RESTClient) defaultTaskboardColumns(ctx context.Context, projectID, teamID string) ([]TaskboardColumn, error) {
	var backlogs struct {
		Value []struct {
			Type          string `json:"type"`
			WorkItemTypes []struct {
				Name string `json:"name"`
			} `json:"workItemTypes"`
			DefaultWorkItemType struct {
				Name string `json:"name"`
			} `json:"defaultWorkItemType"`
		} `json:"value"`
	}
	if err := c.doJSON(ctx, http.MethodGet, teamWorkEndpoint(projectID, teamID, "backlogs"), nil, &backlogs); err != nil {
		return nil, err
	}
	var types []string
	defaultType := ""
	for _, level := range backlogs.Value {
		if !strings.EqualFold(level.Type, "task") {
			continue
		}
		for _, itemType := range level.WorkItemTypes {
			types = append(types, itemType.Name)
		}
		defaultType = level.DefaultWorkItemType.Name
	}
	if len(types) == 0 {
		return []TaskboardColumn{}, nil
	}
	if defaultType == "" {
		defaultType = types[0]
	}
	states := make([][]WorkItemState, len(types))
	reads := make([]func() error, len(types))
	for index, itemType := range types {
		reads[index] = func() error {
			var err error
			states[index], err = c.readTypeStates(ctx, projectID, itemType)
			return err
		}
	}
	if err := runConcurrently(reads...); err != nil {
		return nil, err
	}
	statesByType := make(map[string][]WorkItemState, len(types))
	for index, itemType := range types {
		statesByType[itemType] = states[index]
	}
	return DefaultTaskboardColumns(defaultType, statesByType), nil
}

// readTypeStates reads a work-item type's states in workflow order.
func (c *RESTClient) readTypeStates(ctx context.Context, project, itemType string) ([]WorkItemState, error) {
	var response struct {
		Value []WorkItemState `json:"value"`
	}
	endpoint := fmt.Sprintf("/%s/_apis/wit/workitemtypes/%s/states?api-version=%s", pathPart(project), pathPart(itemType), restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	return response.Value, nil
}

func workingDaysLeftFor(iteration Iteration, today time.Time, workingDays []time.Weekday, daysOff []DayRange) int {
	start, startErr := time.Parse(time.RFC3339, iteration.Start)
	finish, finishErr := time.Parse(time.RFC3339, iteration.Finish)
	if startErr != nil || finishErr != nil {
		return 0
	}
	return WorkingDaysLeft(today, start, finish, workingDays, daysOff)
}

func (c *RESTClient) readSprintSources(ctx context.Context, projectID, teamID, iterationID string) (*sprintSources, error) {
	iterationPath := "teamsettings/iterations/" + pathPart(iterationID)
	var sources sprintSources
	var relations struct {
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
	var columns struct {
		Columns []TaskboardColumn `json:"columns"`
	}
	var settings struct {
		WorkingDays []string `json:"workingDays"`
	}
	var daysOff struct {
		DaysOff []struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"daysOff"`
	}
	get := func(path string, into any) func() error {
		return func() error {
			return c.doJSON(ctx, http.MethodGet, teamWorkEndpoint(projectID, teamID, path), nil, into)
		}
	}
	if err := runConcurrently(
		get(iterationPath, &sources.iteration),
		get(iterationPath+"/workitems", &relations),
		get("taskboardcolumns", &columns),
		get("teamsettings", &settings),
		get(iterationPath+"/teamdaysoff", &daysOff),
	); err != nil {
		return nil, err
	}
	for _, raw := range relations.WorkItemRelations {
		relation := SprintRelation{Target: raw.Target.ID}
		if raw.Rel != nil && *raw.Rel == hierarchyForward && raw.Source != nil {
			relation.Source = raw.Source.ID
		}
		if relation.Target > 0 {
			sources.relations = append(sources.relations, relation)
		}
	}
	sources.columns = columns.Columns
	if sources.columns == nil {
		sources.columns = []TaskboardColumn{}
	}
	sources.workingDays = parseWeekdays(settings.WorkingDays)
	for _, off := range daysOff.DaysOff {
		start, startErr := time.Parse(time.RFC3339, off.Start)
		end, endErr := time.Parse(time.RFC3339, off.End)
		if startErr == nil && endErr == nil {
			sources.daysOff = append(sources.daysOff, DayRange{Start: start, End: end})
		}
	}
	return &sources, nil
}

func parseWeekdays(names []string) []time.Weekday {
	weekdays := make([]time.Weekday, 0, len(names))
	for _, name := range names {
		for weekday := time.Sunday; weekday <= time.Saturday; weekday++ {
			if strings.EqualFold(weekday.String(), strings.TrimSpace(name)) {
				weekdays = append(weekdays, weekday)
			}
		}
	}
	return weekdays
}

// runConcurrently runs independent reads together and returns the first error.
func runConcurrently(reads ...func() error) error {
	errs := make([]error, len(reads))
	var wg sync.WaitGroup
	for index, read := range reads {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[index] = read()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
