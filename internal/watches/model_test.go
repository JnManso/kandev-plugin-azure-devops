package watches

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"kandev-plugin-azure-devops/internal/azure"
)

func TestTaskTitleFitsKandevLimit(t *testing.T) {
	long := "DD Backoffice | Context Configuration | Configure manual verification per Validation"
	watch := &Watch{ProjectID: "Contoso"}
	cases := map[string]struct {
		match  Match
		prefix string
	}{
		"work item":    {WorkItemMatch(azure.WorkItem{ID: 1152692, Project: "Contoso", Title: long}), "[Contoso#1152692] DD Backoffice"},
		"pull request": {PullRequestMatch(azure.PullRequest{ID: 591494, ProjectName: "Contoso", Title: long}), "[Contoso!591494] DD Backoffice"},
	}
	for name, tc := range cases {
		title := watch.TaskTitle(tc.match)
		if utf8.RuneCountInString(title) != TaskTitleMaxLength || !strings.HasSuffix(title, "…") || !strings.HasPrefix(title, tc.prefix) {
			t.Fatalf("%s title = %q (%d characters), want %d characters starting %q and ending with an ellipsis",
				name, title, utf8.RuneCountInString(title), TaskTitleMaxLength, tc.prefix)
		}
	}
	if got := watch.TaskTitle(WorkItemMatch(azure.WorkItem{ID: 1, Project: "Platform", Title: "One"})); got != "[Platform#1] One" {
		t.Fatalf("short title = %q, want it unchanged", got)
	}
}

func TestWatchRunCreatesTaskForLongWorkItemTitle(t *testing.T) {
	f := newFixture(t)
	f.client.items = []azure.WorkItem{{ID: 1152692, Project: "Contoso",
		Title: "DD Backoffice | Context Configuration | Configure manual verification per Validation"}}
	watch, err := f.service.Create(context.Background(), "ws-1", workItemInput())
	if err != nil {
		t.Fatal(err)
	}
	created, err := f.service.Trigger(context.Background(), "ws-1", watch.ID)
	if err != nil || created != 1 {
		t.Fatalf("trigger = %d, %v; want one task", created, err)
	}
	if prompt := f.tasks.created[0].Description; !strings.Contains(prompt, "per Validation") {
		t.Fatalf("prompt = %q, want the full work item title", prompt)
	}
}
