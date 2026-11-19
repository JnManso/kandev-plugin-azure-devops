package azure

import (
	"math"
	"sort"
	"strings"
	"time"
)

// TeamArea is one area path a team owns.
type TeamArea struct {
	Path            string `json:"path"`
	IncludeChildren bool   `json:"includeChildren"`
}

// Iteration is one sprint of a team. Start and Finish are RFC 3339 dates, or
// empty when the sprint has no dates.
type Iteration struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Start     string `json:"start,omitempty"`
	Finish    string `json:"finish,omitempty"`
	TimeFrame string `json:"timeFrame,omitempty"`
}

// TeamSettings are the team's areas, the field they are stored in, and its
// sprints.
type TeamSettings struct {
	AreaField  string      `json:"areaField"`
	Areas      []TeamArea  `json:"areas"`
	Iterations []Iteration `json:"iterations"`
}

// TaskboardMapping places one work-item type in one state in a column.
type TaskboardMapping struct {
	WorkItemType string `json:"workItemType"`
	State        string `json:"state"`
}

// TaskboardColumn is one column of a team's sprint taskboard.
type TaskboardColumn struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Mappings []TaskboardMapping `json:"mappings,omitempty"`
}

// SprintCard is a taskboard task and the column it is in.
type SprintCard struct {
	WorkItem
	ColumnID string `json:"columnId"`
}

// SprintRow is a backlog item with its tasks, or, without a parent, the tasks
// of the sprint that have no backlog item in it.
type SprintRow struct {
	Parent *WorkItem    `json:"parent,omitempty"`
	Tasks  []SprintCard `json:"tasks"`
}

// SprintBoard is a sprint laid out as a taskboard.
type SprintBoard struct {
	Iteration       Iteration         `json:"iteration"`
	WorkingDaysLeft int               `json:"workingDaysLeft"`
	Columns         []TaskboardColumn `json:"columns"`
	Rows            []SprintRow       `json:"rows"`
	People          []string          `json:"people"`
	Truncated       bool              `json:"truncated,omitempty"`
}

// SprintRelation is one entry of a sprint's work-item list: Target is in the
// sprint, and Source, when not zero, is its parent.
type SprintRelation struct {
	Source int `json:"source,omitempty"`
	Target int `json:"target"`
}

// DayRange is an inclusive range of days off.
type DayRange struct {
	Start time.Time
	End   time.Time
}

// TaskboardColumnFor returns the column whose mapping names the item's type
// and state, else the first column mapping the state for any type, else the
// first column.
func TaskboardColumnFor(columns []TaskboardColumn, itemType, state string) string {
	if len(columns) == 0 {
		return ""
	}
	for _, column := range columns {
		for _, mapping := range column.Mappings {
			if strings.EqualFold(mapping.WorkItemType, itemType) && strings.EqualFold(mapping.State, state) {
				return column.ID
			}
		}
	}
	for _, column := range columns {
		for _, mapping := range column.Mappings {
			if strings.EqualFold(mapping.State, state) {
				return column.ID
			}
		}
	}
	return columns[0].ID
}

// WorkItemState is one state of a work-item type and its category
// (Proposed, InProgress, Resolved, Completed, Removed).
type WorkItemState struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// DefaultTaskboardColumns are the columns Azure DevOps shows on a taskboard
// the team never customized: one per state of the task level's default type,
// in workflow order, without removed states. Every task type maps its states
// to the column of the same name, else to the first column of the same
// category.
func DefaultTaskboardColumns(defaultType string, statesByType map[string][]WorkItemState) []TaskboardColumn {
	var defaults []WorkItemState
	for itemType, states := range statesByType {
		if strings.EqualFold(itemType, defaultType) {
			defaults = states
		}
	}
	columns := []TaskboardColumn{}
	categories := map[string]int{}
	for _, state := range defaults {
		if strings.EqualFold(state.Category, stateCategoryRemoved) {
			continue
		}
		if _, ok := categories[strings.ToLower(state.Category)]; !ok {
			categories[strings.ToLower(state.Category)] = len(columns)
		}
		columns = append(columns, TaskboardColumn{ID: state.Name, Name: state.Name})
	}
	types := make([]string, 0, len(statesByType))
	for itemType := range statesByType {
		types = append(types, itemType)
	}
	sort.Strings(types)
	for _, itemType := range types {
		for _, state := range statesByType[itemType] {
			index := -1
			for candidate, column := range columns {
				if strings.EqualFold(column.Name, state.Name) {
					index = candidate
				}
			}
			if index < 0 {
				if byCategory, ok := categories[strings.ToLower(state.Category)]; ok {
					index = byCategory
				}
			}
			if index >= 0 {
				columns[index].Mappings = append(columns[index].Mappings, TaskboardMapping{WorkItemType: itemType, State: state.Name})
			}
		}
	}
	return columns
}

// BuildSprintRows groups the sprint's tasks under their backlog items. A
// work item whose type the taskboard columns map is a task; any other
// top-level item is a backlog item. Tasks without a backlog item in the
// sprint form the first row; backlog items follow in backlog order.
func BuildSprintRows(relations []SprintRelation, items map[int]WorkItem, columns []TaskboardColumn) []SprintRow {
	taskTypes := map[string]bool{}
	for _, column := range columns {
		for _, mapping := range column.Mappings {
			taskTypes[strings.ToLower(mapping.WorkItemType)] = true
		}
	}
	parents := map[int]*SprintRow{}
	var parentOrder []int
	for _, relation := range relations {
		item, ok := items[relation.Target]
		if ok && relation.Source == 0 && !taskTypes[strings.ToLower(item.Type)] {
			parent := item
			parents[item.ID] = &SprintRow{Parent: &parent, Tasks: []SprintCard{}}
			parentOrder = append(parentOrder, item.ID)
		}
	}
	unparented := &SprintRow{Tasks: []SprintCard{}}
	for _, relation := range relations {
		item, ok := items[relation.Target]
		if !ok || parents[item.ID] != nil {
			continue
		}
		card := SprintCard{WorkItem: item, ColumnID: TaskboardColumnFor(columns, item.Type, item.State)}
		if row := parents[relation.Source]; relation.Source != 0 && row != nil {
			row.Tasks = append(row.Tasks, card)
		} else {
			unparented.Tasks = append(unparented.Tasks, card)
		}
	}
	sort.SliceStable(parentOrder, func(i, j int) bool {
		return backlogBefore(*parents[parentOrder[i]].Parent, *parents[parentOrder[j]].Parent)
	})
	rows := make([]SprintRow, 0, len(parentOrder)+1)
	if len(unparented.Tasks) > 0 {
		sortCards(unparented.Tasks)
		rows = append(rows, *unparented)
	}
	for _, id := range parentOrder {
		sortCards(parents[id].Tasks)
		rows = append(rows, *parents[id])
	}
	return rows
}

func sortCards(cards []SprintCard) {
	sort.SliceStable(cards, func(i, j int) bool { return backlogBefore(cards[i].WorkItem, cards[j].WorkItem) })
}

// backlogBefore orders by backlog priority, then stack rank, then ID; an item
// without a rank sorts after ranked ones.
func backlogBefore(left, right WorkItem) bool {
	for _, field := range []string{"Microsoft.VSTS.Common.BacklogPriority", "Microsoft.VSTS.Common.StackRank"} {
		l, r := rankField(left.Fields, field), rankField(right.Fields, field)
		if l != r {
			return l < r
		}
	}
	return left.ID < right.ID
}

func rankField(fields map[string]any, key string) float64 {
	if value, ok := fields[key].(float64); ok {
		return value
	}
	return math.Inf(1)
}

// WorkingDaysLeft counts the team's working days from today, or from the
// start when the sprint has not begun, to the finish, both inclusive, minus
// days off.
func WorkingDaysLeft(today, start, finish time.Time, workingDays []time.Weekday, daysOff []DayRange) int {
	working := map[time.Weekday]bool{}
	for _, weekday := range workingDays {
		working[weekday] = true
	}
	from := dateOf(today)
	if dateOf(start).After(from) {
		from = dateOf(start)
	}
	count := 0
	for current := from; !current.After(dateOf(finish)); current = current.AddDate(0, 0, 1) {
		if working[current.Weekday()] && !isDayOff(current, daysOff) {
			count++
		}
	}
	return count
}

func isDayOff(day time.Time, daysOff []DayRange) bool {
	for _, off := range daysOff {
		if !day.Before(dateOf(off.Start)) && !day.After(dateOf(off.End)) {
			return true
		}
	}
	return false
}

func dateOf(value time.Time) time.Time {
	year, month, dayOfMonth := value.UTC().Date()
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
}

// SprintPeople lists every assignee of a backlog item or task once, by name.
func SprintPeople(rows []SprintRow) []string {
	seen := map[string]bool{}
	people := []string{}
	add := func(name string) {
		if name = strings.TrimSpace(name); name != "" && !seen[name] {
			seen[name] = true
			people = append(people, name)
		}
	}
	for _, row := range rows {
		if row.Parent != nil {
			add(row.Parent.AssignedTo)
		}
		for _, task := range row.Tasks {
			add(task.AssignedTo)
		}
	}
	sort.SliceStable(people, func(i, j int) bool { return lessName(people[i], people[j]) })
	return people
}
