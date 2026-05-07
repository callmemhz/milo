package deploy

import (
	"context"
	"testing"
	"time"
)

func TestLockSerializes(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()
	h1, err := lm.Acquire(ctx, "myapp")
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		h2, _ := lm.Acquire(ctx, "myapp")
		h2.Release()
		close(done)
	}()
	<-started
	select {
	case <-done:
		t.Fatal("second acquire returned before first released")
	case <-time.After(50 * time.Millisecond):
	}
	h1.Release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second acquire never returned")
	}
}

func TestLockSupersedesPredecessor(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()
	h1, _ := lm.Acquire(ctx, "myapp")

	// h1 should not be done yet
	select {
	case <-h1.Done():
		t.Fatal("h1 already done")
	default:
	}

	go func() {
		h2, _ := lm.Acquire(ctx, "myapp")
		defer h2.Release()
	}()

	select {
	case <-h1.Done():
		// good — superseded
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor never cancelled")
	}
	h1.Release()
}

func TestLockCancelHelper(t *testing.T) {
	lm := NewLockManager()
	h, _ := lm.Acquire(context.Background(), "k")
	lm.Cancel("k")
	select {
	case <-h.Done():
	case <-time.After(time.Second):
		t.Fatal("Cancel did not propagate")
	}
	h.Release()
}

func TestLockDifferentKeysIndependent(t *testing.T) {
	lm := NewLockManager()
	h1, _ := lm.Acquire(context.Background(), "a")
	h2, _ := lm.Acquire(context.Background(), "b")
	if h1.Context() == h2.Context() {
		t.Fatal("contexts shared across keys")
	}
	h1.Release()
	h2.Release()
}
