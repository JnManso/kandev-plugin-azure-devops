package watches

import (
	"context"
	"fmt"
	"log"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	taskScanPageSize = 100
	// maxTaskScanPages bounds the metadata lookup of tasks whose create
	// outcome is uncertain.
	maxTaskScanPages = 50
)

// settle brings the watch's reservations up to date and returns how many of
// them hold a task that may still be running. It resolves reservations left
// without a task ID, retries task links, and forgets reservations of earlier
// generations once their task stopped. A task whose state cannot be read
// counts as running, so a transient host error never frees capacity.
func (s *Service) settle(ctx context.Context, watch Watch) (int, error) {
	stored, err := s.deps.Store.AllReservations(ctx, watch)
	if err != nil {
		return 0, err
	}
	lookup := &taskLookup{service: s, watch: watch}
	limited := watch.MaxInflightTasks > 0
	open := 0
	for _, entry := range stored {
		current := entry.Generation == watch.Generation
		reservation, resolved := s.resolvePending(ctx, watch, entry, lookup)
		if !resolved {
			if limited {
				open++
			}
			continue
		}
		if reservation.TaskID == "" {
			continue
		}
		s.retryLink(ctx, watch, entry.Generation, reservation)
		if current && !limited {
			continue
		}
		running, known := s.taskRunning(ctx, reservation.TaskID)
		if !current && known && !running {
			_ = s.deps.Store.releaseReservationIn(ctx, watch, entry.Generation, reservation.ItemKey)
			continue
		}
		if limited && (running || !known) {
			open++
		}
	}
	return open, nil
}

// resolvePending returns the reservation with its task ID filled in, or with
// an empty task ID after releasing a create that never happened. resolved is
// false while the outcome is still unknown.
func (s *Service) resolvePending(ctx context.Context, watch Watch, entry StoredReservation, lookup *taskLookup) (Reservation, bool) {
	reservation := entry.Reservation
	if reservation.TaskID != "" {
		return reservation, true
	}
	taskID, known := lookup.find(ctx, reservation.ItemKey)
	if !known {
		return reservation, false
	}
	if taskID == "" {
		_ = s.deps.Store.releaseReservationIn(ctx, watch, entry.Generation, reservation.ItemKey)
		return reservation, true
	}
	reservation.TaskID = taskID
	if err := s.deps.Store.saveReservationIn(ctx, watch, entry.Generation, reservation); err != nil {
		log.Printf("azure devops: watch %s: record task of %s: %v", watch.ID, reservation.ItemKey, err)
	}
	return reservation, true
}

// retryLink links a task whose link failed when it was created.
func (s *Service) retryLink(ctx context.Context, watch Watch, generation int64, reservation Reservation) {
	if reservation.Linked || reservation.Item == nil || s.deps.Link == nil {
		return
	}
	if !s.link(ctx, watch, reservation.TaskID, *reservation.Item) {
		return
	}
	reservation.Linked = true
	_ = s.deps.Store.saveReservationIn(ctx, watch, generation, reservation)
}

func (s *Service) link(ctx context.Context, watch Watch, taskID string, item ItemLink) bool {
	if s.deps.Link == nil {
		return true
	}
	if err := s.deps.Link(ctx, watch.WorkspaceID, taskID, item); err != nil {
		log.Printf("azure devops: watch %s: link task %s: %v", watch.ID, taskID, err)
		return false
	}
	return true
}

// taskRunning reports whether a task may still be running. known is false
// when the host could not answer.
func (s *Service) taskRunning(ctx context.Context, taskID string) (running, known bool) {
	task, err := s.deps.Tasks().Get(ctx, taskID)
	if err != nil {
		return false, status.Code(err) == codes.NotFound
	}
	if task == nil || task.ArchivedAt != nil || terminalTaskState(task.State) {
		return false, true
	}
	return true, true
}

// taskLookup finds the tasks a watch created by their metadata, reading the
// workspace's tasks at most once per poll.
type taskLookup struct {
	service  *Service
	watch    Watch
	loaded   bool
	complete bool
	byItem   map[string]string
}

// find returns the task created for an item. known is false when the lookup
// failed or stopped before reading every task without finding it.
func (l *taskLookup) find(ctx context.Context, itemKey string) (taskID string, known bool) {
	if !l.loaded {
		l.load(ctx)
	}
	if taskID, ok := l.byItem[itemKey]; ok {
		return taskID, true
	}
	return "", l.complete
}

func (l *taskLookup) load(ctx context.Context) {
	l.loaded, l.byItem = true, map[string]string{}
	filter := pluginsdk.TaskFilter{WorkspaceIDs: []string{l.watch.WorkspaceID}, IncludeArchived: true}
	cursor := ""
	for range maxTaskScanPages {
		tasks, info, err := l.service.deps.Tasks().List(ctx, filter, pluginsdk.Page{Limit: taskScanPageSize, Cursor: cursor})
		if err != nil {
			log.Printf("azure devops: watch %s: look up created tasks: %v", l.watch.ID, err)
			return
		}
		for _, task := range tasks {
			if fmt.Sprint(task.Metadata[MetadataWatchID]) != l.watch.ID {
				continue
			}
			if key, ok := task.Metadata[MetadataItemKey].(string); ok && key != "" {
				l.byItem[key] = task.ID
			}
		}
		if info == nil || !info.HasMore || info.NextCursor == "" || info.NextCursor == cursor {
			l.complete = true
			return
		}
		cursor = info.NextCursor
	}
}

// createTask reserves the item before creating its task, so an item creates
// at most one task per generation, including across restarts.
func (s *Service) createTask(ctx context.Context, watch *Watch, match Match) (bool, error) {
	_, found, err := s.deps.Store.Reservation(ctx, *watch, match.Key)
	if err != nil || found {
		return false, err
	}
	item := match.Link(watch.Organization, watch.ProjectID)
	reservation := Reservation{ItemKey: match.Key, URL: match.URL, Item: &item, ReservedAt: s.deps.Now().UTC()}
	if err := s.deps.Store.SaveReservation(ctx, *watch, reservation); err != nil {
		return false, err
	}
	task, err := s.deps.Tasks().Create(ctx, s.taskInput(*watch, match))
	if err != nil {
		return false, s.createFailed(ctx, watch, match.Key, err)
	}
	reservation.TaskID = task.ID
	reservation.Linked = s.link(ctx, *watch, task.ID, item)
	if err := s.deps.Store.SaveReservation(ctx, *watch, reservation); err != nil {
		// The reservation keeps no task ID; the next poll finds the task by
		// its metadata and counts it as running meanwhile.
		log.Printf("azure devops: watch %s: record task of %s: %v", watch.ID, match.Key, err)
	}
	return true, nil
}

// createFailed releases the item only when the error shows Kandev did not
// create the task. Otherwise the reservation stays until a later poll finds
// the task or confirms it does not exist, so a lost response never starts a
// second agent for the same item.
func (s *Service) createFailed(ctx context.Context, watch *Watch, itemKey string, err error) error {
	if createRejected(err) {
		_ = s.deps.Store.ReleaseReservation(ctx, *watch, itemKey)
	}
	if reason := launchRejection(err); reason != "" {
		watch.Enabled, watch.DisabledReason = false, reason
		log.Printf("azure devops: watch %s disabled: %s", watch.ID, reason)
	}
	return fmt.Errorf("task creation failed: %s", status.Convert(err).Message())
}

// createRejected reports whether Kandev refused a create before making the
// task.
func createRejected(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.PermissionDenied, codes.Unauthenticated,
		codes.NotFound, codes.AlreadyExists, codes.ResourceExhausted, codes.OutOfRange, codes.Unimplemented:
		return true
	default:
		return false
	}
}
