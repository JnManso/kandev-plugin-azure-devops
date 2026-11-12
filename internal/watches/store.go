package watches

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	watchKeyPrefix       = "watch."
	reservationSeparator = ".item."
)

// State is the subset of the plugin state store the watches use.
type State interface {
	LoadState(ctx context.Context, workspaceID, key string, target any) (bool, error)
	SaveState(ctx context.Context, workspaceID, key string, value any) error
	DeleteState(ctx context.Context, workspaceID, key string) error
}

// Reservation records that a watch generation handled one item. TaskID is
// empty until Kandev created the task, and stays empty when the create
// outcome or the write of the task ID is uncertain; a later poll then looks
// the task up by its watch metadata. Linked records that the task holds a
// link to Item.
type Reservation struct {
	ItemKey    string    `json:"itemKey"`
	URL        string    `json:"url,omitempty"`
	TaskID     string    `json:"taskId,omitempty"`
	Item       *ItemLink `json:"item,omitempty"`
	Linked     bool      `json:"linked,omitempty"`
	ReservedAt time.Time `json:"reservedAt"`
}

// StoredReservation is a reservation with the generation that holds it.
type StoredReservation struct {
	Generation int64
	Reservation
}

// Store keeps watches under watch.<id> and their reservations under
// watch.<id>.item.<generation>.<itemKey> in workspace state.
type Store struct {
	state State
	keys  func(ctx context.Context, workspaceID string) ([]string, error)
}

// NewStore returns a watch store.
func NewStore(state State, keys func(ctx context.Context, workspaceID string) ([]string, error)) *Store {
	return &Store{state: state, keys: keys}
}

func watchKey(id string) string { return watchKeyPrefix + id }

func reservationPrefix(id string, generation int64) string {
	return watchKeyPrefix + id + reservationSeparator + strconv.FormatInt(generation, 10) + "."
}

func reservationKey(id string, generation int64, itemKey string) string {
	return reservationPrefix(id, generation) + itemKey
}

// List returns a workspace's watches ordered by creation.
func (s *Store) List(ctx context.Context, workspaceID string) ([]Watch, error) {
	keys, err := s.keys(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	watches := []Watch{}
	for _, key := range keys {
		id, ok := strings.CutPrefix(key, watchKeyPrefix)
		if !ok || id == "" || strings.Contains(id, ".") {
			continue
		}
		var watch Watch
		if found, err := s.state.LoadState(ctx, workspaceID, key, &watch); err != nil {
			return nil, err
		} else if found {
			watches = append(watches, watch)
		}
	}
	sort.SliceStable(watches, func(left, right int) bool {
		return watches[left].CreatedAt.Before(watches[right].CreatedAt)
	})
	return watches, nil
}

// Get loads one watch.
func (s *Store) Get(ctx context.Context, workspaceID, id string) (Watch, error) {
	var watch Watch
	if id == "" || strings.Contains(id, ".") {
		return watch, ErrNotFound
	}
	found, err := s.state.LoadState(ctx, workspaceID, watchKey(id), &watch)
	if err != nil {
		return watch, err
	}
	if !found {
		return watch, ErrNotFound
	}
	return watch, nil
}

// Save writes a watch.
func (s *Store) Save(ctx context.Context, watch Watch) error {
	return s.state.SaveState(ctx, watch.WorkspaceID, watchKey(watch.ID), watch)
}

// Delete removes a watch, then every reservation it holds. The watch goes
// first: a failed reservation cleanup leaves records no watch reads, while a
// watch left without its reservations would create tasks again.
func (s *Store) Delete(ctx context.Context, workspaceID, id string) error {
	if err := s.state.DeleteState(ctx, workspaceID, watchKey(id)); err != nil {
		return err
	}
	return s.deleteReservations(ctx, workspaceID, id)
}

// Reservation returns the reservation of an item, if any.
func (s *Store) Reservation(ctx context.Context, watch Watch, itemKey string) (Reservation, bool, error) {
	var reservation Reservation
	found, err := s.state.LoadState(ctx, watch.WorkspaceID, reservationKey(watch.ID, watch.Generation, itemKey), &reservation)
	return reservation, found, err
}

// SaveReservation writes a reservation of the watch's current generation.
func (s *Store) SaveReservation(ctx context.Context, watch Watch, reservation Reservation) error {
	return s.saveReservationIn(ctx, watch, watch.Generation, reservation)
}

func (s *Store) saveReservationIn(ctx context.Context, watch Watch, generation int64, reservation Reservation) error {
	return s.state.SaveState(ctx, watch.WorkspaceID, reservationKey(watch.ID, generation, reservation.ItemKey), reservation)
}

// ReleaseReservation forgets one item so a later poll can retry it.
func (s *Store) ReleaseReservation(ctx context.Context, watch Watch, itemKey string) error {
	return s.releaseReservationIn(ctx, watch, watch.Generation, itemKey)
}

func (s *Store) releaseReservationIn(ctx context.Context, watch Watch, generation int64, itemKey string) error {
	return s.state.DeleteState(ctx, watch.WorkspaceID, reservationKey(watch.ID, generation, itemKey))
}

// ReservedKeys returns the item keys the watch's current generation handled.
func (s *Store) ReservedKeys(ctx context.Context, watch Watch) (map[string]bool, error) {
	keys, err := s.reservationKeys(ctx, watch.WorkspaceID, watch.ID, watch.Generation)
	if err != nil {
		return nil, err
	}
	prefix := reservationPrefix(watch.ID, watch.Generation)
	reserved := make(map[string]bool, len(keys))
	for _, key := range keys {
		reserved[strings.TrimPrefix(key, prefix)] = true
	}
	return reserved, nil
}

// AllReservations lists the reservations of every generation of a watch.
// Earlier generations stay until their task stops running, so they keep
// counting against the watch's inflight limit after a reset.
func (s *Store) AllReservations(ctx context.Context, watch Watch) ([]StoredReservation, error) {
	keys, err := s.keys(ctx, watch.WorkspaceID)
	if err != nil {
		return nil, err
	}
	prefix := watchKeyPrefix + watch.ID + reservationSeparator
	result := []StoredReservation{}
	for _, key := range keys {
		rest, ok := strings.CutPrefix(key, prefix)
		if !ok {
			continue
		}
		generationText, _, ok := strings.Cut(rest, ".")
		generation, err := strconv.ParseInt(generationText, 10, 64)
		if !ok || err != nil {
			continue
		}
		var reservation Reservation
		if found, err := s.state.LoadState(ctx, watch.WorkspaceID, key, &reservation); err != nil {
			return nil, err
		} else if found {
			result = append(result, StoredReservation{Generation: generation, Reservation: reservation})
		}
	}
	return result, nil
}

// Reservations lists the reservations of the watch's current generation.
func (s *Store) Reservations(ctx context.Context, watch Watch) ([]Reservation, error) {
	keys, err := s.reservationKeys(ctx, watch.WorkspaceID, watch.ID, watch.Generation)
	if err != nil {
		return nil, err
	}
	result := make([]Reservation, 0, len(keys))
	for _, key := range keys {
		var reservation Reservation
		if found, err := s.state.LoadState(ctx, watch.WorkspaceID, key, &reservation); err != nil {
			return nil, err
		} else if found {
			result = append(result, reservation)
		}
	}
	return result, nil
}

// deleteReservations removes the reservations of every generation.
func (s *Store) deleteReservations(ctx context.Context, workspaceID, id string) error {
	keys, err := s.keys(ctx, workspaceID)
	if err != nil {
		return err
	}
	prefix := watchKeyPrefix + id + reservationSeparator
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			if err := s.state.DeleteState(ctx, workspaceID, key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) reservationKeys(ctx context.Context, workspaceID, id string, generation int64) ([]string, error) {
	keys, err := s.keys(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	prefix := reservationPrefix(id, generation)
	result := []string{}
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			result = append(result, key)
		}
	}
	return result, nil
}
