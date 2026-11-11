package watches

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"kandev-plugin-azure-devops/internal/azure"
)

// PollInterval is how often the poller looks for due watches.
const PollInterval = time.Minute

// MetadataWatchID and MetadataItemKey identify watch-created tasks.
const (
	MetadataWatchID = "azure_devops_watch_id"
	MetadataItemKey = "azure_devops_item_key"
	meToken         = "@me"
)

// Client is the Azure DevOps surface a watch queries.
type Client interface {
	QueryWIQL(ctx context.Context, projectID, wiql string, top int) (*azure.WorkItemSearchResult, error)
	ListPullRequests(ctx context.Context, filter azure.PullRequestFilter) (*azure.PullRequestPage, error)
	GetCurrentIdentity(ctx context.Context) (*azure.Identity, error)
}

// Tasks is the host task API a watch uses.
type Tasks interface {
	Get(ctx context.Context, id string) (*pluginsdk.Task, error)
	Create(ctx context.Context, in pluginsdk.CreateTaskInput) (*pluginsdk.Task, error)
}

// Dependencies wires a Service to the connection and host.
type Dependencies struct {
	Store      *Store
	Client     func(ctx context.Context, workspaceID string) (Client, error)
	Tasks      func() Tasks
	Workspaces func(ctx context.Context) ([]string, error)
	Now        func() time.Time
}

// Service manages watches and runs their polls.
type Service struct {
	deps  Dependencies
	locks sync.Map // workspace/watch ID -> *sync.Mutex
}

// NewService returns a watch service.
func NewService(deps Dependencies) *Service {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{deps: deps}
}

func (s *Service) lock(workspaceID, id string) func() {
	value, _ := s.locks.LoadOrStore(workspaceID+"/"+id, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func newID() string {
	buffer := make([]byte, 12)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

// List returns a workspace's watches.
func (s *Service) List(ctx context.Context, workspaceID string) ([]Watch, error) {
	return s.deps.Store.List(ctx, workspaceID)
}

// Create saves a new enabled watch; the poller runs it within a minute.
func (s *Service) Create(ctx context.Context, workspaceID string, in Input) (Watch, error) {
	if in.Kind == nil {
		return Watch{}, fmt.Errorf("%w: kind is required", ErrInvalid)
	}
	now := s.deps.Now().UTC()
	watch := Watch{
		ID: newID(), Kind: *in.Kind, WorkspaceID: workspaceID, Enabled: true, Generation: 1,
		PollIntervalSeconds: DefaultPollIntervalSeconds, CreatedAt: now, UpdatedAt: now,
	}
	if watch.Kind == KindPullRequest {
		watch.Status = "active"
	}
	in.Enabled = nil
	watch.Apply(in)
	if err := watch.Validate(); err != nil {
		return Watch{}, err
	}
	return watch, s.deps.Store.Save(ctx, watch)
}

// Update changes a watch. Enabling a watch clears its disabled reason.
func (s *Service) Update(ctx context.Context, workspaceID, id string, in Input) (Watch, error) {
	defer s.lock(workspaceID, id)()
	watch, err := s.deps.Store.Get(ctx, workspaceID, id)
	if err != nil {
		return Watch{}, err
	}
	in.Kind = nil
	watch.Apply(in)
	if watch.Enabled {
		watch.DisabledReason = ""
	}
	if err := watch.Validate(); err != nil {
		return Watch{}, err
	}
	watch.UpdatedAt = s.deps.Now().UTC()
	return watch, s.deps.Store.Save(ctx, watch)
}

// Delete removes a watch and its reservations. Created tasks are kept.
func (s *Service) Delete(ctx context.Context, workspaceID, id string) error {
	defer s.lock(workspaceID, id)()
	if _, err := s.deps.Store.Get(ctx, workspaceID, id); err != nil {
		return err
	}
	return s.deps.Store.Delete(ctx, workspaceID, id)
}

// PreviewReset counts the items a reset would make eligible again.
func (s *Service) PreviewReset(ctx context.Context, workspaceID, id string) (int, error) {
	watch, err := s.deps.Store.Get(ctx, workspaceID, id)
	if err != nil {
		return 0, err
	}
	reservations, err := s.deps.Store.Reservations(ctx, watch)
	return len(reservations), err
}

// Reset starts a new generation, so every current match creates a task again
// on the next poll. Existing tasks are kept.
func (s *Service) Reset(ctx context.Context, workspaceID, id string) (int, error) {
	defer s.lock(workspaceID, id)()
	watch, err := s.deps.Store.Get(ctx, workspaceID, id)
	if err != nil {
		return 0, err
	}
	reservations, err := s.deps.Store.Reservations(ctx, watch)
	if err != nil {
		return 0, err
	}
	watch.Generation++
	watch.LastPolledAt, watch.UpdatedAt = nil, s.deps.Now().UTC()
	if err := s.deps.Store.Save(ctx, watch); err != nil {
		return 0, err
	}
	if err := s.deps.Store.deleteReservations(ctx, workspaceID, id, watch.Generation); err != nil {
		return 0, err
	}
	return len(reservations), nil
}

// Trigger polls one watch now and returns how many tasks it created.
func (s *Service) Trigger(ctx context.Context, workspaceID, id string) (int, error) {
	defer s.lock(workspaceID, id)()
	watch, err := s.deps.Store.Get(ctx, workspaceID, id)
	if err != nil {
		return 0, err
	}
	created, runErr := s.run(ctx, &watch)
	now := s.deps.Now().UTC()
	watch.LastPolledAt = &now
	if runErr != nil {
		watch.LastError, watch.LastErrorAt = runErr.Error(), &now
	} else {
		watch.LastError, watch.LastErrorAt = "", nil
	}
	if err := s.deps.Store.Save(ctx, watch); err != nil {
		return created, err
	}
	return created, runErr
}

func (s *Service) run(ctx context.Context, watch *Watch) (int, error) {
	client, err := s.deps.Client(ctx, watch.WorkspaceID)
	if err != nil {
		return 0, fmt.Errorf("Azure DevOps is not connected: %w", err)
	}
	matches, err := s.matches(ctx, client, *watch)
	if err != nil {
		return 0, fmt.Errorf("Azure DevOps query failed: %w", err)
	}
	capacity, err := s.capacity(ctx, *watch)
	if err != nil {
		return 0, err
	}
	created := 0
	var failure error
	for _, match := range matches {
		if capacity == 0 || ctx.Err() != nil {
			break
		}
		ok, err := s.createTask(ctx, watch, match)
		if err != nil {
			if !watch.Enabled {
				return created, err
			}
			failure = err
			continue
		}
		if ok {
			created++
			capacity--
		}
	}
	return created, failure
}

func (s *Service) matches(ctx context.Context, client Client, watch Watch) ([]Match, error) {
	if watch.Kind == KindWorkItem {
		result, err := client.QueryWIQL(ctx, watch.ProjectID, watch.WIQL, maxMatchesPerPoll)
		if err != nil || result == nil {
			return nil, err
		}
		matches := make([]Match, 0, len(result.Items))
		for _, item := range result.Items {
			matches = append(matches, WorkItemMatch(item))
		}
		return matches, nil
	}
	creator, err := resolveIdentity(ctx, client, watch.CreatorID)
	if err != nil {
		return nil, err
	}
	reviewer, err := resolveIdentity(ctx, client, watch.ReviewerID)
	if err != nil {
		return nil, err
	}
	page, err := client.ListPullRequests(ctx, azure.PullRequestFilter{
		ProjectID: watch.ProjectID, RepositoryID: watch.AzureRepositoryID, Status: firstNonEmpty(watch.Status, "active"),
		CreatorID: creator, ReviewerID: reviewer, Top: maxMatchesPerPoll,
	})
	if err != nil || page == nil {
		return nil, err
	}
	matches := make([]Match, 0, len(page.Items))
	for _, pr := range page.Items {
		matches = append(matches, PullRequestMatch(pr))
	}
	return matches, nil
}

func resolveIdentity(ctx context.Context, client Client, value string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(value), meToken) {
		return strings.TrimSpace(value), nil
	}
	identity, err := client.GetCurrentIdentity(ctx)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

// capacity is how many tasks the watch may still create; -1 is unlimited.
func (s *Service) capacity(ctx context.Context, watch Watch) (int, error) {
	if watch.MaxInflightTasks <= 0 {
		return -1, nil
	}
	reservations, err := s.deps.Store.Reservations(ctx, watch)
	if err != nil {
		return 0, err
	}
	open := 0
	for _, reservation := range reservations {
		if reservation.TaskID == "" {
			continue
		}
		task, err := s.deps.Tasks().Get(ctx, reservation.TaskID)
		if err == nil && task != nil && task.ArchivedAt == nil && !terminalTaskState(task.State) {
			open++
		}
	}
	return max(watch.MaxInflightTasks-open, 0), nil
}

func terminalTaskState(state string) bool {
	switch strings.ToUpper(state) {
	case "COMPLETED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
}

// createTask reserves the item before creating its task, so an item creates
// at most one task per generation, including across restarts.
func (s *Service) createTask(ctx context.Context, watch *Watch, match Match) (bool, error) {
	_, found, err := s.deps.Store.Reservation(ctx, *watch, match.Key)
	if err != nil || found {
		return false, err
	}
	reservation := Reservation{ItemKey: match.Key, URL: match.URL, ReservedAt: s.deps.Now().UTC()}
	if err := s.deps.Store.SaveReservation(ctx, *watch, reservation); err != nil {
		return false, err
	}
	task, err := s.deps.Tasks().Create(ctx, s.taskInput(*watch, match))
	if err != nil {
		_ = s.deps.Store.ReleaseReservation(ctx, *watch, match.Key)
		if reason := launchRejection(err); reason != "" {
			watch.Enabled, watch.DisabledReason = false, reason
			log.Printf("azure devops: watch %s disabled: %s", watch.ID, reason)
		}
		return false, fmt.Errorf("task creation failed: %s", status.Convert(err).Message())
	}
	reservation.TaskID = task.ID
	if err := s.deps.Store.SaveReservation(ctx, *watch, reservation); err != nil {
		log.Printf("azure devops: watch %s: record task of %s: %v", watch.ID, match.Key, err)
	}
	return true, nil
}

func (s *Service) taskInput(watch Watch, match Match) pluginsdk.CreateTaskInput {
	prompt := watch.TaskPrompt(match)
	step := watch.WorkflowStepID
	input := pluginsdk.CreateTaskInput{
		WorkspaceID: watch.WorkspaceID, WorkflowID: watch.WorkflowID, WorkflowStepID: &step,
		Title: watch.TaskTitle(match), Description: prompt, StartAgent: true,
		Launch: &pluginsdk.PluginTaskLaunchOptions{
			AgentProfileID: &watch.AgentProfileID, ExecutorProfileID: &watch.ExecutorProfileID, Prompt: &prompt,
		},
		Metadata: map[string]any{MetadataWatchID: watch.ID, MetadataItemKey: match.Key, "azure_devops_item_url": match.URL},
	}
	if watch.RepositoryID != "" {
		repository := pluginsdk.PluginTaskRepository{RepositoryID: watch.RepositoryID}
		if watch.BaseBranch != "" {
			base := watch.BaseBranch
			repository.BaseBranch = &base
		}
		input.Repositories = []pluginsdk.PluginTaskRepository{repository}
	}
	return input
}

// launchRejection maps Kandev's refusal of an unavailable launch profile to a
// disabled reason.
func launchRejection(err error) string {
	converted := status.Convert(err)
	if converted.Code() != codes.InvalidArgument {
		return ""
	}
	switch message := converted.Message(); {
	case strings.Contains(message, "agent_profile_id is not available"):
		return ReasonAgentProfileUnavailable
	case strings.Contains(message, "executor_profile_id is not available"):
		return ReasonExecutorProfileUnavailable
	default:
		return ""
	}
}

// PollOnce triggers every due watch of every workspace.
func (s *Service) PollOnce(ctx context.Context) {
	workspaces, err := s.deps.Workspaces(ctx)
	if err != nil {
		return
	}
	now := s.deps.Now()
	for _, workspaceID := range workspaces {
		watches, err := s.deps.Store.List(ctx, workspaceID)
		if err != nil {
			continue
		}
		for _, watch := range watches {
			if ctx.Err() != nil {
				return
			}
			if !watch.Due(now) {
				continue
			}
			if created, err := s.Trigger(ctx, workspaceID, watch.ID); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("azure devops: watch %s: %v", watch.ID, err)
			} else if created > 0 {
				log.Printf("azure devops: watch %s created %d task(s)", watch.ID, created)
			}
		}
	}
}

// Run polls until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		s.PollOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
