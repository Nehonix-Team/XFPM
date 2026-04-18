package utils

import (
	"context"
	"sync"
	"sync/atomic"
)

// AdaptiveSemaphore handles dynamic concurrency limits professionally.
// It uses a mutex-protected waiter queue to ensure fairness and efficiency,
// eliminating the overhead and potential deadlocks of a manager goroutine.
type AdaptiveSemaphore struct {
	mu      sync.Mutex
	active  int
	limit   atomic.Int64
	waiters []chan struct{}
}

// NewAdaptiveSemaphore creates a new adaptive semaphore.
func NewAdaptiveSemaphore(initialLimit int) *AdaptiveSemaphore {
	s := &AdaptiveSemaphore{}
	s.limit.Store(int64(initialLimit))
	return s
}

// Acquire blocks until a token is available or context is cancelled.
func (s *AdaptiveSemaphore) Acquire(ctx context.Context, limit int) error {
	s.limit.Store(int64(limit))

	s.mu.Lock()
	if s.active < int(s.limit.Load()) {
		s.active++
		s.mu.Unlock()
		return nil
	}

	// Queue up a waiter
	waiter := make(chan struct{}, 1)
	s.waiters = append(s.waiters, waiter)
	s.mu.Unlock()

	select {
	case <-waiter:
		return nil
	case <-ctx.Done():
		// Clean up waiter if cancelled
		s.mu.Lock()
		for i, w := range s.waiters {
			if w == waiter {
				s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		return ctx.Err()
	}
}

// Release signals that a token is available.
func (s *AdaptiveSemaphore) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.waiters) > 0 && s.active <= int(s.limit.Load()) {
		// Wake up next waiter
		waiter := s.waiters[0]
		s.waiters = s.waiters[1:]
		close(waiter)
		// We don't decrement active because the waiter takes the slot
	} else {
		s.active--
		if s.active < 0 { s.active = 0 }
	}
}
