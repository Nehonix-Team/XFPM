package utils

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// SignalManager is a centralized OS signal distributor.
// It captures SIGINT and SIGTERM at program startup and
// dispatches them to all registered subscribers.
var SignalManager = newSignalManager()

type signalManager struct {
	mu          sync.Mutex
	subscribers []chan os.Signal
	started     bool
	done        chan struct{} // closed once the signal has been dispatched
}

func newSignalManager() *signalManager {
	return &signalManager{
		done: make(chan struct{}),
	}
}

// Start must be called once at program entry (main.go).
// It registers the signal handler and begins dispatching to subscribers.
// Subsequent calls are no-ops.
func (sm *signalManager) Start() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.started {
		return
	}
	sm.started = true

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-ch
		// Restore terminal cursor if it was hidden by pterm or other UI tools
		os.Stderr.Write([]byte("\033[?25h"))

		// Snapshot + clear subscribers atomically so Unsubscribe
		// on a dispatched channel doesn't touch a stale slice.
		sm.mu.Lock()
		subs := sm.subscribers
		sm.subscribers = nil
		sm.mu.Unlock()

		// Dispatch to all subscribers; close their channel so
		// receivers using range or a second read get EOF cleanly.
		for _, sub := range subs {
			select {
			case sub <- sig:
			default:
				// Subscriber is not reading — signal dropped for this sub,
				// but we still close so they unblock if waiting.
			}
			close(sub)
		}

		// Signal dispatch is complete; unblock anyone waiting on Done().
		close(sm.done)

		// Re-register default handler so a second SIGINT terminates the
		// process forcefully if graceful shutdown hangs.
		signal.Reset(os.Interrupt, syscall.SIGTERM)
	}()
}

// Subscribe registers a channel to receive exactly one shutdown signal.
// The channel is buffered (size 1) and will be closed after dispatch,
// so callers can use either a receive or a range loop safely.
//
// If the signal has already been dispatched, the returned channel is
// immediately closed so callers don't block forever.
func (sm *signalManager) Subscribe() chan os.Signal {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ch := make(chan os.Signal, 1)

	select {
	case <-sm.done:
		// Already fired — return a closed channel so the caller
		// doesn't hang waiting for a signal that will never come.
		close(ch)
		return ch
	default:
	}

	sm.subscribers = append(sm.subscribers, ch)
	return ch
}

// Unsubscribe removes a channel from the subscriber list.
// It is safe to call after the signal has been dispatched (no-op in that case).
// The caller should drain and/or close the channel themselves after unsubscribing
// to avoid goroutine leaks.
func (sm *signalManager) Unsubscribe(ch chan os.Signal) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, sub := range sm.subscribers {
		if sub == ch {
			// Order-preserving removal.
			sm.subscribers = append(sm.subscribers[:i], sm.subscribers[i+1:]...)
			return
		}
	}
}

// Done returns a channel that is closed once the shutdown signal has been
// dispatched to all subscribers. Useful for components that want to react
// to shutdown without needing a dedicated signal channel.
//
//	<-utils.SignalManager.Done() // blocks until shutdown
func (sm *signalManager) Done() <-chan struct{} {
	return sm.done
}