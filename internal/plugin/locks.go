package plugin

import "sync"

// workspaceLocks serializes credential mutations and token renewal per
// workspace, so a renewal cannot overwrite a newer sign-in or save.
type workspaceLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (l *workspaceLocks) lock(workspaceID string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*sync.Mutex)
	}
	lock, ok := l.locks[workspaceID]
	if !ok {
		lock = &sync.Mutex{}
		l.locks[workspaceID] = lock
	}
	l.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}
