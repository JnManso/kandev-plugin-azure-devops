package watches

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"kandev-plugin-azure-devops/internal/azure"
)

func manyWorkItems(count int) []azure.WorkItem {
	items := make([]azure.WorkItem, 0, count)
	for id := 1; id <= count; id++ {
		items = append(items, azure.WorkItem{ID: id, Title: "Item " + strconv.Itoa(id), Project: "Platform"})
	}
	return items
}

func reserveAll(t *testing.T, f *fixture, watch Watch, keys []string) {
	t.Helper()
	stored, err := f.service.deps.Store.Get(context.Background(), "ws-1", watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if err := f.service.deps.Store.SaveReservation(context.Background(), stored, Reservation{ItemKey: key, TaskID: "handled-" + key, ReservedAt: f.now}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkItemWatchReachesMatchesPastHandledItems(t *testing.T) {
	f := newFixture(t)
	f.client.items = manyWorkItems(150)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	handled := make([]string, 0, 100)
	for id := 1; id <= 100; id++ {
		handled = append(handled, workItemKey(id))
	}
	reserveAll(t, f, watch, handled)
	created, err := f.service.Trigger(context.Background(), "ws-1", watch.ID)
	if err != nil || created != 50 {
		t.Fatalf("created %d, %v; want the 50 items after the handled ones", created, err)
	}
	if f.client.hydrated != 50 {
		t.Fatalf("read %d work items, want only the 50 unhandled ones", f.client.hydrated)
	}
}

func TestPullRequestWatchPagesPastHandledPullRequests(t *testing.T) {
	f := newFixture(t)
	handled := []string{}
	for id := 1; id <= 130; id++ {
		pr := azure.PullRequest{ID: id, Title: "PR", ProjectName: "Platform", RepositoryID: "repo"}
		f.client.prs = append(f.client.prs, pr)
		if id <= 100 {
			handled = append(handled, PullRequestMatch(pr).Key)
		}
	}
	input := Input{
		Kind: ptr(KindPullRequest), ProjectID: ptr("Platform"), WorkflowID: ptr("wf"), WorkflowStepID: ptr("step"),
		AgentProfileID: ptr("agent"), ExecutorProfileID: ptr("exec"), Prompt: ptr("Review"),
	}
	watch, _ := f.service.Create(context.Background(), "ws-1", input)
	reserveAll(t, f, watch, handled)
	if created, err := f.service.Trigger(context.Background(), "ws-1", watch.ID); err != nil || created != 30 {
		t.Fatalf("created %d, %v; want the 30 pull requests on the second page", created, err)
	}
}

func TestAnUnreadableTaskStillCountsAgainstTheInflightLimit(t *testing.T) {
	f := newFixture(t)
	input := workItemInput()
	input.MaxInflightTasks = ptr(1)
	watch, _ := f.service.Create(context.Background(), "ws-1", input)
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 1 {
		t.Fatalf("created %d, want 1", created)
	}
	f.tasks.getFail = status.Error(codes.Unavailable, "host busy")
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 0 {
		t.Fatalf("created %d while the running task could not be read, want 0", created)
	}
}

func TestALostCreateResponseNeverStartsASecondTask(t *testing.T) {
	f := newFixture(t)
	f.client.items = f.client.items[:1]
	input := workItemInput()
	input.MaxInflightTasks = ptr(1)
	watch, _ := f.service.Create(context.Background(), "ws-1", input)
	f.tasks.failAfterCreate = status.Error(codes.Unavailable, "connection reset")
	if _, err := f.service.Trigger(context.Background(), "ws-1", watch.ID); err == nil {
		t.Fatal("expected the lost response to be reported")
	}
	f.tasks.failAfterCreate = nil
	f.tasks.listFail = status.Error(codes.Unavailable, "host busy")
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 0 || len(f.tasks.created) != 1 {
		t.Fatalf("created %d (%d total) while the outcome was unknown, want none", created, len(f.tasks.created))
	}
	f.tasks.listFail = nil
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 0 || len(f.tasks.created) != 1 {
		t.Fatalf("created %d (%d total) after the task was found, want none", created, len(f.tasks.created))
	}
	reservation, found, _ := f.service.deps.Store.Reservation(context.Background(), mustWatch(t, f, watch.ID), "wi-1")
	if !found || reservation.TaskID != "task-1" {
		t.Fatalf("reservation = %+v, found %v; want the task found by its metadata", reservation, found)
	}
}

func TestAFailedTaskIDWriteIsRecoveredFromTheTaskMetadata(t *testing.T) {
	f := newFixture(t)
	f.client.items = f.client.items[:1]
	input := workItemInput()
	input.MaxInflightTasks = ptr(1)
	watch, _ := f.service.Create(context.Background(), "ws-1", input)
	f.state.failSave = func(key string, value any) error {
		if reservation, ok := value.(Reservation); ok && reservation.TaskID != "" {
			return errors.New("state write failed")
		}
		return nil
	}
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 1 {
		t.Fatalf("created %d, want 1", created)
	}
	f.state.failSave = nil
	f.client.items = manyWorkItems(2)
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 0 {
		t.Fatalf("created %d while the untracked task runs, want 0", created)
	}
	reservation, _, _ := f.service.deps.Store.Reservation(context.Background(), mustWatch(t, f, watch.ID), "wi-1")
	if reservation.TaskID != "task-1" {
		t.Fatalf("reservation = %+v, want the recovered task ID", reservation)
	}
}

func TestResetKeepsRunningTasksInTheInflightLimit(t *testing.T) {
	f := newFixture(t)
	f.client.items = f.client.items[:1]
	input := workItemInput()
	input.MaxInflightTasks = ptr(1)
	watch, _ := f.service.Create(context.Background(), "ws-1", input)
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 1 {
		t.Fatalf("created %d, want 1", created)
	}
	if _, err := f.service.Reset(context.Background(), "ws-1", watch.ID); err != nil {
		t.Fatal(err)
	}
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 0 {
		t.Fatalf("created %d while the first task runs, want 0", created)
	}
	f.tasks.states["task-1"] = "COMPLETED"
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 1 {
		t.Fatalf("created %d after the first task completed, want 1", created)
	}
	stored, _ := f.service.deps.Store.AllReservations(context.Background(), mustWatch(t, f, watch.ID))
	if len(stored) != 1 || stored[0].Generation != 2 {
		t.Fatalf("reservations = %+v, want only the new generation's", stored)
	}
}

func TestAnotherOrganizationStartsANewGeneration(t *testing.T) {
	f := newFixture(t)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 2 {
		t.Fatalf("created %d, want 2", created)
	}
	if saved := mustWatch(t, f, watch.ID); saved.Organization != "contoso" {
		t.Fatalf("organization = %q", saved.Organization)
	}
	f.organization = "https://dev.azure.com/Contoso/"
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 0 {
		t.Fatalf("created %d for the same organization, want 0", created)
	}
	f.organization = "https://dev.azure.com/fabrikam"
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 2 {
		t.Fatalf("created %d for another organization's items, want 2", created)
	}
	if saved := mustWatch(t, f, watch.ID); saved.Organization != "fabrikam" || saved.Generation != 2 {
		t.Fatalf("watch = %+v", saved)
	}
}

func TestAScheduledPollSkipsAWatchDisabledAfterItWasListed(t *testing.T) {
	f := newFixture(t)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	listed := mustWatch(t, f, watch.ID)
	if _, err := f.service.Update(context.Background(), "ws-1", watch.ID, Input{Enabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if !listed.Due(f.now) {
		t.Fatal("the listed snapshot should look due")
	}
	now := f.now
	if created, err := f.service.trigger(context.Background(), "ws-1", watch.ID, &now); err != nil || created != 0 {
		t.Fatalf("scheduled poll created %d, %v; want 0", created, err)
	}
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 2 {
		t.Fatalf("Run now created %d, want 2", created)
	}
}

func TestAFailedReservationCleanupLeavesNoPollableWatch(t *testing.T) {
	f := newFixture(t)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	_, _ = f.service.Trigger(context.Background(), "ws-1", watch.ID)
	f.state.failDelete = func(key string) error {
		if strings.Contains(key, reservationSeparator) {
			return errors.New("state delete failed")
		}
		return nil
	}
	if err := f.service.Delete(context.Background(), "ws-1", watch.ID); err == nil {
		t.Fatal("expected the cleanup failure to be reported")
	}
	if watches, _ := f.service.List(context.Background(), "ws-1"); len(watches) != 0 {
		t.Fatalf("watches after a failed delete = %+v", watches)
	}
	f.now = f.now.Add(time.Hour)
	f.service.PollOnce(context.Background())
	if len(f.tasks.created) != 2 {
		t.Fatalf("created %d tasks after the delete, want the original 2", len(f.tasks.created))
	}
}

func TestWatchTasksAreLinkedToTheirItemAndFailedLinksRetry(t *testing.T) {
	f := newFixture(t)
	f.client.items = f.client.items[:1]
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	f.linkFail = errors.New("state unavailable")
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 1 {
		t.Fatalf("created %d, want 1", created)
	}
	f.linkFail = nil
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 0 {
		t.Fatalf("created %d when retrying the link, want 0", created)
	}
	if len(f.linked) != 1 || f.linked[0].WorkItemID != 1 || f.linked[0].Organization != "contoso" || f.linked[0].ProjectID != "Platform" {
		t.Fatalf("linked = %+v", f.linked)
	}
	_, _ = f.service.Trigger(context.Background(), "ws-1", watch.ID)
	if len(f.linked) != 1 {
		t.Fatalf("linked %d times, want once", len(f.linked))
	}
}

func mustWatch(t *testing.T, f *fixture, id string) Watch {
	t.Helper()
	watch, err := f.service.deps.Store.Get(context.Background(), "ws-1", id)
	if err != nil {
		t.Fatal(err)
	}
	return watch
}
