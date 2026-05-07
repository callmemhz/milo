package deploy

import (
	"context"
	"sync"
)

// LockHandle represents a held per-app deploy lock.
type LockHandle struct {
	ctx     context.Context
	cancel  context.CancelFunc
	release func()
}

// Done returns a channel that is closed when this handle is cancelled or released.
func (h *LockHandle) Done() <-chan struct{} { return h.ctx.Done() }

// Context returns the context associated with this lock handle.
func (h *LockHandle) Context() context.Context { return h.ctx }

// Release cancels the handle's context and removes it from the manager.
func (h *LockHandle) Release() { h.release() }

// LockManager manages per-app cancellable locks with last-write-wins semantics.
type LockManager struct {
	mu       sync.Mutex
	locks    map[string]*LockHandle   // currently held
	released map[string]chan struct{} // closed when previous holder releases
}

// NewLockManager creates a new LockManager.
func NewLockManager() *LockManager {
	return &LockManager{
		locks:    map[string]*LockHandle{},
		released: map[string]chan struct{}{},
	}
}

// Acquire signals any in-flight holder to cancel, waits for that holder to release,
// then installs a new handle keyed by key and returns it.
func (lm *LockManager) Acquire(parent context.Context, key string) (*LockHandle, error) {
	for {
		lm.mu.Lock()
		if old, held := lm.locks[key]; held {
			// Cancel old; wait for its release on the released channel.
			old.cancel()
			ch := lm.released[key]
			lm.mu.Unlock()
			select {
			case <-ch:
			case <-parent.Done():
				return nil, parent.Err()
			}
			continue
		}
		// Free: install a new handle.
		ctx, cancel := context.WithCancel(parent)
		relCh := make(chan struct{})
		h := &LockHandle{ctx: ctx, cancel: cancel}
		h.release = func() {
			cancel()
			lm.mu.Lock()
			delete(lm.locks, key)
			delete(lm.released, key)
			lm.mu.Unlock()
			close(relCh)
		}
		lm.locks[key] = h
		lm.released[key] = relCh
		lm.mu.Unlock()
		return h, nil
	}
}

// Cancel signals the in-flight holder for key (if any) without acquiring.
// Used by HTTP cancel endpoint in M10.
func (lm *LockManager) Cancel(key string) {
	lm.mu.Lock()
	h := lm.locks[key]
	lm.mu.Unlock()
	if h != nil {
		h.cancel()
	}
}
