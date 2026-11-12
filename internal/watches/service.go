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
	QueryWorkItemIDs(ctx context.Context, projectID, wiql string, top int) ([]int, error)
	GetWorkItems(ctx context.Context, projectID string, ids []int) ([]azure.WorkItem, error)
	ListPullRequests(ctx context.Context, filter azure.PullRequestFilter) (*azure.PullRequestPage, error)
	GetCurrentIdentity(ctx context.Context) (*azure.Identity, error)
}

// Tasks is the host task API a watch uses.
type Tasks interface {
	List(ctx context.Context, filter pluginsdk.TaskFilter, page pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error)
	Get(ctx context.Context, id string) (*pluginsdk.Task, error)
	Create(ctx context.Context, in pluginsdk.CreateTaskInput) (*pluginsdk.Task, error)
}

// Dependencies wires a Service to the connection and host. Client returns
// the workspace client and its organization URL. Link links a created task
// to its matched item; nil skips linking.
type Dependencies struct {
	Store      *Store
	Client     func(ctx context.Context, workspaceID string) (Client, string, error)
	Tasks      func() Tasks
	Link       func(ctx context.Context, workspaceID, taskID string, item ItemLink) error
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
// on the next poll. Existing tasks are kept, and those still running keep
// counting against the inflight limit until they stop.
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
	return len(reservations), nil
}

// Trigger polls one watch now ("Run now") and returns how many tasks it
// created.
func (s *Service) Trigger(ctx context.Context, workspaceID, id string) (int, error) {
	return s.trigger(ctx, workspaceID, id, nil)
}

// trigger polls one watch under its lock. A scheduled poll passes dueAt and
// re-checks the stored watch, so a watch disabled or polled since the poller
// listed it is skipped.
func (s *Service) trigger(ctx context.Context, workspaceID, id string, dueAt *time.Time) (int, error) {
	defer s.lock(workspaceID, id)()
	watch, err := s.deps.Store.Get(ctx, workspaceID, id)
	if err != nil {
		return 0, err
	}
	if dueAt != nil && !watch.Due(*dueAt) {
		return 0, nil
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
	client, organizationURL, err := s.deps.Client(ctx, watch.WorkspaceID)
	if err != nil {
		return 0, fmt.Errorf("Azure DevOps is not connected: %w", err)
	}
	bindOrganization(watch, organizationURL)
	open, err := s.settle(ctx, *watch)
	if err != nil {
		return 0, err
	}
	capacity := -1
	if watch.MaxInflightTasks > 0 {
		capacity = max(watch.MaxInflightTasks-open, 0)
	}
	if capacity == 0 {
		return 0, nil
	}
	return s.createForMatches(ctx, client, watch, capacity)
}

// bindOrganization starts a new generation when the workspace connection
// moved to another organization, so items of the earlier organization stop
// deduplicating unrelated items that share their number. Replacing the
// credential within the same organization keeps the generation.
func bindOrganization(watch *Watch, organizationURL string) {
	organization, err := azure.OrganizationName(organizationURL)
	if err != nil {
		return
	}
	organization = strings.ToLower(organization)
	if watch.Organization == organization {
		return
	}
	if watch.Organization != "" {
		watch.Generation++
	}
	watch.Organization = organization
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

func terminalTaskState(state string) bool {
	switch strings.ToUpper(state) {
	case "COMPLETED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
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
			if created, err := s.trigger(ctx, workspaceID, watch.ID, &now); err != nil && !errors.Is(err, context.Canceled) {
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
