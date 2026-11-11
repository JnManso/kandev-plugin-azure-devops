package watches

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"kandev-plugin-azure-devops/internal/azure"
)

type memoryState struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memoryState) LoadState(_ context.Context, workspaceID, key string, target any) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[workspaceID+"|"+key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal([]byte(value), target)
}

func (m *memoryState) SaveState(_ context.Context, workspaceID, key string, value any) error {
	encoded, err := json.Marshal(value)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[workspaceID+"|"+key] = string(encoded)
	return err
}

func (m *memoryState) DeleteState(_ context.Context, workspaceID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, workspaceID+"|"+key)
	return nil
}

func (m *memoryState) keys(_ context.Context, workspaceID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for key := range m.values {
		if after, ok := strings.CutPrefix(key, workspaceID+"|"); ok {
			keys = append(keys, after)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

type fakeClient struct {
	items []azure.WorkItem
	prs   []azure.PullRequest
	last  azure.PullRequestFilter
}

func (c *fakeClient) QueryWIQL(context.Context, string, string, int) (*azure.WorkItemSearchResult, error) {
	return &azure.WorkItemSearchResult{Items: c.items}, nil
}

func (c *fakeClient) ListPullRequests(_ context.Context, filter azure.PullRequestFilter) (*azure.PullRequestPage, error) {
	c.last = filter
	return &azure.PullRequestPage{Items: c.prs}, nil
}

func (c *fakeClient) GetCurrentIdentity(context.Context) (*azure.Identity, error) {
	return &azure.Identity{ID: "me-guid"}, nil
}

type fakeTasks struct {
	mu      sync.Mutex
	created []pluginsdk.CreateTaskInput
	states  map[string]string
	fail    error
}

func (t *fakeTasks) Get(_ context.Context, id string) (*pluginsdk.Task, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.states[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	return &pluginsdk.Task{ID: id, State: state}, nil
}

func (t *fakeTasks) Create(_ context.Context, in pluginsdk.CreateTaskInput) (*pluginsdk.Task, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fail != nil {
		return nil, t.fail
	}
	if utf8.RuneCountInString(in.Title) > TaskTitleMaxLength {
		return nil, errors.New("task title is too long: task titles must be 60 characters or fewer")
	}
	t.created = append(t.created, in)
	id := "task-" + strconv.Itoa(len(t.created))
	t.states[id] = "IN_PROGRESS"
	return &pluginsdk.Task{ID: id}, nil
}

type fixture struct {
	service *Service
	state   *memoryState
	client  *fakeClient
	tasks   *fakeTasks
	now     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		state:  &memoryState{values: map[string]string{}},
		client: &fakeClient{items: []azure.WorkItem{{ID: 1, Title: "One", Project: "Platform"}, {ID: 2, Title: "Two", Project: "Platform"}}},
		tasks:  &fakeTasks{states: map[string]string{}},
		now:    time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC),
	}
	f.service = NewService(Dependencies{
		Store:      NewStore(f.state, f.state.keys),
		Client:     func(context.Context, string) (Client, error) { return f.client, nil },
		Tasks:      func() Tasks { return f.tasks },
		Workspaces: func(context.Context) ([]string, error) { return []string{"ws-1"}, nil },
		Now:        func() time.Time { return f.now },
	})
	return f
}

func ptr[T any](value T) *T { return &value }

func workItemInput() Input {
	return Input{
		Kind: ptr(KindWorkItem), ProjectID: ptr("Platform"), WIQL: ptr("SELECT [System.Id] FROM WorkItems"),
		WorkflowID: ptr("wf"), WorkflowStepID: ptr("step"), AgentProfileID: ptr("agent"), ExecutorProfileID: ptr("exec"),
		Prompt: ptr("Implement {{work_item.title}} ({{work_item.id}})"), RepositoryID: ptr("repo-1"), BaseBranch: ptr("main"),
	}
}

func TestWatchCreatesExactlyOneTaskPerItemAcrossPollsAndRestarts(t *testing.T) {
	f := newFixture(t)
	watch, err := f.service.Create(context.Background(), "ws-1", workItemInput())
	if err != nil {
		t.Fatal(err)
	}
	created, err := f.service.Trigger(context.Background(), "ws-1", watch.ID)
	if err != nil || created != 2 {
		t.Fatalf("first trigger = %d, %v", created, err)
	}
	first := f.tasks.created[0]
	if first.Title != "[Platform#1] One" || first.Description != "Implement One (1)" || !first.StartAgent ||
		*first.Launch.AgentProfileID != "agent" || *first.WorkflowStepID != "step" ||
		first.Metadata[MetadataWatchID] != watch.ID || first.Metadata[MetadataItemKey] != "wi-1" ||
		first.Repositories[0].RepositoryID != "repo-1" || *first.Repositories[0].BaseBranch != "main" {
		t.Fatalf("task input = %+v", first)
	}
	// A new service over the same state behaves like a plugin restart.
	restarted := newFixture(t)
	restarted.state, restarted.tasks = f.state, f.tasks
	restarted.service.deps.Store = NewStore(f.state, f.state.keys)
	restarted.service.deps.Tasks = func() Tasks { return f.tasks }
	if created, err := restarted.service.Trigger(context.Background(), "ws-1", watch.ID); err != nil || created != 0 {
		t.Fatalf("trigger after restart = %d, %v", created, err)
	}
	if len(f.tasks.created) != 2 {
		t.Fatalf("created %d tasks, want 2", len(f.tasks.created))
	}
}

func TestWatchRespectsTheInflightLimit(t *testing.T) {
	f := newFixture(t)
	input := workItemInput()
	input.MaxInflightTasks = ptr(1)
	watch, _ := f.service.Create(context.Background(), "ws-1", input)
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 1 {
		t.Fatalf("created %d, want 1", created)
	}
	f.tasks.states["task-1"] = "COMPLETED"
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 1 {
		t.Fatalf("after completion created %d, want 1", created)
	}
}

func TestResetMakesItemsEligibleAgain(t *testing.T) {
	f := newFixture(t)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	_, _ = f.service.Trigger(context.Background(), "ws-1", watch.ID)
	if count, err := f.service.PreviewReset(context.Background(), "ws-1", watch.ID); err != nil || count != 2 {
		t.Fatalf("preview = %d, %v", count, err)
	}
	if count, err := f.service.Reset(context.Background(), "ws-1", watch.ID); err != nil || count != 2 {
		t.Fatalf("reset = %d, %v", count, err)
	}
	if count, _ := f.service.PreviewReset(context.Background(), "ws-1", watch.ID); count != 0 {
		t.Fatalf("preview after reset = %d", count)
	}
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 2 {
		t.Fatalf("created after reset = %d", created)
	}
}

func TestUnavailableProfileDisablesTheWatchWithAReason(t *testing.T) {
	f := newFixture(t)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	f.tasks.fail = status.Error(codes.InvalidArgument, "launch.executor_profile_id is not available")
	if _, err := f.service.Trigger(context.Background(), "ws-1", watch.ID); err == nil {
		t.Fatal("expected the rejected launch to fail")
	}
	saved, _ := f.service.deps.Store.Get(context.Background(), "ws-1", watch.ID)
	if saved.Enabled || saved.DisabledReason != ReasonExecutorProfileUnavailable || saved.LastError == "" {
		t.Fatalf("watch = %+v", saved)
	}
	if count, _ := f.service.PreviewReset(context.Background(), "ws-1", watch.ID); count != 0 {
		t.Fatal("a rejected item kept its reservation")
	}
	f.tasks.fail = nil
	updated, err := f.service.Update(context.Background(), "ws-1", watch.ID, Input{Enabled: ptr(true), ExecutorProfileID: ptr("other")})
	if err != nil || updated.DisabledReason != "" || !updated.Enabled {
		t.Fatalf("re-enabled = %+v, %v", updated, err)
	}
}

func TestOtherCreateFailuresReleaseTheItemForRetry(t *testing.T) {
	f := newFixture(t)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	f.tasks.fail = errors.New("transient")
	_, _ = f.service.Trigger(context.Background(), "ws-1", watch.ID)
	saved, _ := f.service.deps.Store.Get(context.Background(), "ws-1", watch.ID)
	if !saved.Enabled || saved.LastError == "" {
		t.Fatalf("watch = %+v", saved)
	}
	f.tasks.fail = nil
	if created, _ := f.service.Trigger(context.Background(), "ws-1", watch.ID); created != 2 {
		t.Fatalf("retry created %d", created)
	}
}

func TestPullRequestWatchResolvesMeAndRendersTheTask(t *testing.T) {
	f := newFixture(t)
	f.client.prs = []azure.PullRequest{{ID: 7, Title: "Fix", ProjectName: "Platform", RepositoryID: "GUID", WebURL: "https://x/7", SourceBranch: "f"}}
	input := Input{
		Kind: ptr(KindPullRequest), ProjectID: ptr("Platform"), ReviewerID: ptr("@me"), WorkflowID: ptr("wf"),
		WorkflowStepID: ptr("step"), AgentProfileID: ptr("agent"), ExecutorProfileID: ptr("exec"), Prompt: ptr("Review {{pull_request.url}} from {{pull_request.source_branch}}"),
	}
	watch, err := f.service.Create(context.Background(), "ws-1", input)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := f.service.Trigger(context.Background(), "ws-1", watch.ID); err != nil || created != 1 {
		t.Fatalf("trigger = %d, %v", created, err)
	}
	if f.client.last.ReviewerID != "me-guid" || f.client.last.Status != "active" {
		t.Fatalf("filter = %+v", f.client.last)
	}
	task := f.tasks.created[0]
	if task.Title != "[Platform!7] Fix" || task.Description != "Review https://x/7 from f" || task.Metadata[MetadataItemKey] != "pr-guid-7" {
		t.Fatalf("task = %+v", task)
	}
}

func TestValidationAndDueness(t *testing.T) {
	f := newFixture(t)
	input := workItemInput()
	input.WIQL = ptr("")
	if _, err := f.service.Create(context.Background(), "ws-1", input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing WIQL err = %v", err)
	}
	if _, err := f.service.Create(context.Background(), "ws-1", Input{Kind: ptr(Kind("other"))}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad kind err = %v", err)
	}
	if _, err := f.service.Update(context.Background(), "ws-1", "missing", Input{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing watch err = %v", err)
	}
	watch := Watch{Enabled: true, PollIntervalSeconds: 10}
	if !watch.Due(f.now) {
		t.Fatal("a never-polled watch is due")
	}
	polled := f.now
	watch.LastPolledAt = &polled
	if watch.Due(f.now.Add(30*time.Second)) || !watch.Due(f.now.Add(time.Minute)) {
		t.Fatal("the minimum interval is 60 seconds")
	}
}

func TestPollOnceRunsDueWatchesAndDeleteRemovesReservations(t *testing.T) {
	f := newFixture(t)
	watch, _ := f.service.Create(context.Background(), "ws-1", workItemInput())
	f.service.PollOnce(context.Background())
	if len(f.tasks.created) != 2 {
		t.Fatalf("poll created %d", len(f.tasks.created))
	}
	f.service.PollOnce(context.Background())
	if len(f.tasks.created) != 2 {
		t.Fatal("a watch polled again before its interval")
	}
	if err := f.service.Delete(context.Background(), "ws-1", watch.ID); err != nil {
		t.Fatal(err)
	}
	if keys, _ := f.state.keys(context.Background(), "ws-1"); len(keys) != 0 {
		t.Fatalf("keys after delete = %v", keys)
	}
}
