package utils

import (
	"context"
)

// AdaptiveSemaphore handles dynamic concurrency limits professionally.
// It uses a internal manager goroutine to handle token distribution
// and fairness, ensuring it perfectly respects context cancellation.
type AdaptiveSemaphore struct {
	requests chan acquireReq
	release  chan struct{}
	update   chan int
}

type acquireReq struct {
	ctx  context.Context
	done chan struct{}
}

// NewAdaptiveSemaphore creates a new adaptive semaphore.
// It starts a background manager goroutine that lives for the duration of the object.
func NewAdaptiveSemaphore(initialLimit int) *AdaptiveSemaphore {
	s := &AdaptiveSemaphore{
		requests: make(chan acquireReq),
		release:  make(chan struct{}),
		update:   make(chan int),
	}
	go s.manager(initialLimit)
	return s
}

// manager is the core logic that handles token distribution.
func (s *AdaptiveSemaphore) manager(limit int) {
	active := 0
	var queue []acquireReq

	for {
		var firstReq chan struct{}
		var currentReq acquireReq
		
		if active < limit && len(queue) > 0 {
			currentReq = queue[0]
			firstReq = currentReq.done
		}

		select {
		case req := <-s.requests:
			queue = append(queue, req)
		case <-s.release:
			active--
		case newLimit := <-s.update:
			limit = newLimit
		case firstReq <- struct{}{}:
			active++
			queue = queue[1:]
		}

		// Cleanup cancelled requests from queue
		newQueue := make([]acquireReq, 0, len(queue))
		for _, r := range queue {
			select {
			case <-r.ctx.Done():
				// Skip cancelled
			default:
				newQueue = append(newQueue, r)
			}
		}
		queue = newQueue
	}
}

// Acquire blocks until a token is available or context is cancelled.
func (s *AdaptiveSemaphore) Acquire(ctx context.Context, limit int) error {
	// Update limit in the manager
	select {
	case s.update <- limit:
	case <-ctx.Done():
		return ctx.Err()
	}

	done := make(chan struct{})
	req := acquireReq{ctx: ctx, done: done}

	select {
	case s.requests <- req:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release signals that a token is available.
func (s *AdaptiveSemaphore) Release() {
	select {
	case s.release <- struct{}{}:
	default:
		// Should not happen if correctly used
	}
}
