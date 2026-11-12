package azure

import (
	"reflect"
	"testing"
	"time"
)

var taskboardColumns = []TaskboardColumn{
	{ID: "todo", Name: "To Do", Mappings: []TaskboardMapping{{WorkItemType: "Task", State: "New"}, {WorkItemType: "Bug", State: "New"}}},
	{ID: "doing", Name: "In Progress", Mappings: []TaskboardMapping{{WorkItemType: "Task", State: "Active"}, {WorkItemType: "Bug", State: "Committed"}}},
	{ID: "done", Name: "Done", Mappings: []TaskboardMapping{{WorkItemType: "Task", State: "Closed"}, {WorkItemType: "Bug", State: "Done"}}},
}

func TestTaskboardColumnForFollowsTheColumnMappings(t *testing.T) {
	cases := []struct {
		itemType, state, want string
	}{
		{"Task", "Active", "doing"},
		{"task", "closed", "done"},
		{"Bug", "Committed", "doing"},
		{"Issue", "Closed", "done"},
		{"Task", "Removed", "todo"},
	}
	for _, tc := range cases {
		if got := TaskboardColumnFor(taskboardColumns, tc.itemType, tc.state); got != tc.want {
			t.Errorf("TaskboardColumnFor(%q, %q) = %q, want %q", tc.itemType, tc.state, got, tc.want)
		}
	}
	if got := TaskboardColumnFor(nil, "Task", "New"); got != "" {
		t.Errorf("no columns = %q, want empty", got)
	}
}

func sprintItem(id int, itemType, state, assignee string, priority float64) WorkItem {
	fields := map[string]any{}
	if priority > 0 {
		fields["Microsoft.VSTS.Common.BacklogPriority"] = priority
	}
	return WorkItem{ID: id, Title: itemType, Type: itemType, State: state, AssignedTo: assignee, Fields: fields}
}

func rowSummary(rows []SprintRow) [][]int {
	summary := make([][]int, 0, len(rows))
	for _, row := range rows {
		ids := []int{0}
		if row.Parent != nil {
			ids[0] = row.Parent.ID
		}
		for _, task := range row.Tasks {
			ids = append(ids, task.ID)
		}
		summary = append(summary, ids)
	}
	return summary
}

func TestBuildSprintRowsGroupsTasksUnderTheirBacklogItems(t *testing.T) {
	items := map[int]WorkItem{
		1:  sprintItem(1, "User Story", "Active", "Ana", 20),
		2:  sprintItem(2, "User Story", "New", "Rui", 10),
		3:  sprintItem(3, "User Story", "New", "", 30),
		10: sprintItem(10, "Task", "Active", "Ana", 2),
		11: sprintItem(11, "Task", "New", "", 1),
		12: sprintItem(12, "Task", "Closed", "Rui", 0),
		13: sprintItem(13, "Task", "New", "Ana", 0),
		14: sprintItem(14, "Task", "New", "", 0),
	}
	relations := []SprintRelation{
		{Target: 1}, {Target: 2}, {Target: 3},
		{Source: 1, Target: 10}, {Source: 1, Target: 11},
		{Source: 2, Target: 12},
		{Target: 13},             // a task without a parent
		{Source: 99, Target: 14}, // a task whose parent is outside the sprint
	}
	rows := BuildSprintRows(relations, items, taskboardColumns)
	want := [][]int{{0, 13, 14}, {2, 12}, {1, 11, 10}, {3}}
	if got := rowSummary(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v (unparented first, then backlog order)", got, want)
	}
	if rows[1].Tasks[0].ColumnID != "done" || rows[2].Tasks[1].ColumnID != "doing" {
		t.Fatalf("columns = %+v", rows)
	}
}

func TestBuildSprintRowsOmitsAnEmptyUnparentedRow(t *testing.T) {
	items := map[int]WorkItem{1: sprintItem(1, "User Story", "New", "", 1), 10: sprintItem(10, "Task", "New", "", 1)}
	rows := BuildSprintRows([]SprintRelation{{Target: 1}, {Source: 1, Target: 10}}, items, taskboardColumns)
	if got := rowSummary(rows); !reflect.DeepEqual(got, [][]int{{1, 10}}) {
		t.Fatalf("rows = %v", got)
	}
}

func day(value string) time.Time {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestWorkingDaysLeftCountsTheTeamsWorkingDays(t *testing.T) {
	weekdays := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	start, finish := day("2026-11-09"), day("2026-11-20")
	cases := []struct {
		name    string
		today   time.Time
		daysOff []DayRange
		want    int
	}{
		{"from today to the finish", day("2026-11-11"), nil, 8},
		{"minus a team day off", day("2026-11-11"), []DayRange{{Start: day("2026-11-13"), End: day("2026-11-13")}}, 7},
		{"minus a multi-day absence", day("2026-11-11"), []DayRange{{Start: day("2026-11-16"), End: day("2026-11-17")}}, 6},
		{"before the sprint starts", day("2026-11-02"), nil, 10},
		{"after the sprint ends", day("2026-11-23"), nil, 0},
		{"on a weekend", day("2026-11-15"), nil, 5},
	}
	for _, tc := range cases {
		if got := WorkingDaysLeft(tc.today, start, finish, weekdays, tc.daysOff); got != tc.want {
			t.Errorf("%s: WorkingDaysLeft = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestSprintPeopleListsEveryAssigneeOnce(t *testing.T) {
	parent := sprintItem(1, "User Story", "New", "Rui", 1)
	rows := []SprintRow{
		{Parent: &parent, Tasks: []SprintCard{{WorkItem: sprintItem(10, "Task", "New", "Ana", 0)}, {WorkItem: sprintItem(11, "Task", "New", "", 0)}}},
		{Tasks: []SprintCard{{WorkItem: sprintItem(12, "Task", "New", "ana", 0)}, {WorkItem: sprintItem(13, "Task", "New", "Ana", 0)}}},
	}
	if got := SprintPeople(rows); !reflect.DeepEqual(got, []string{"Ana", "ana", "Rui"}) {
		t.Fatalf("people = %v", got)
	}
}

func TestDefaultTaskboardColumnsFollowTheTaskStates(t *testing.T) {
	states := map[string][]WorkItemState{
		"Task": {{Name: "New", Category: "Proposed"}, {Name: "Active", Category: "InProgress"}, {Name: "Closed", Category: "Completed"}, {Name: "Removed", Category: "Removed"}},
		"Bug":  {{Name: "Proposed", Category: "Proposed"}, {Name: "Active", Category: "InProgress"}, {Name: "Done", Category: "Completed"}},
	}
	columns := DefaultTaskboardColumns("Task", states)
	var names []string
	for _, column := range columns {
		names = append(names, column.Name)
	}
	if want := []string{"New", "Active", "Closed"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("columns = %v, want %v", names, want)
	}
	cases := []struct{ itemType, state, want string }{
		{"Task", "Active", "Active"},
		{"Task", "Closed", "Closed"},
		{"Bug", "Proposed", "New"},
		{"Bug", "Done", "Closed"},
		{"Bug", "Active", "Active"},
	}
	for _, tc := range cases {
		if got := TaskboardColumnFor(columns, tc.itemType, tc.state); got != tc.want {
			t.Errorf("%s %s -> %q, want %q", tc.itemType, tc.state, got, tc.want)
		}
	}
	if got := DefaultTaskboardColumns("Missing", states); len(got) != 0 {
		t.Fatalf("unknown default type = %v", got)
	}
}

func TestBuildSprintRowsWithDefaultColumnsKeepsUnparentedTasksAsTasks(t *testing.T) {
	columns := DefaultTaskboardColumns("Task", map[string][]WorkItemState{"Task": {{Name: "Active", Category: "InProgress"}}})
	items := map[int]WorkItem{1: sprintItem(1, "User Story", "Active", "", 1), 2: sprintItem(2, "Task", "Active", "", 0), 3: sprintItem(3, "Task", "Active", "", 0)}
	rows := BuildSprintRows([]SprintRelation{{Target: 1}, {Source: 1, Target: 2}, {Target: 3}}, items, columns)
	if len(rows) != 2 || rows[0].Parent != nil || rows[0].Tasks[0].ID != 3 || rows[1].Tasks[0].ColumnID != "Active" {
		t.Fatalf("rows = %+v", rows)
	}
}
