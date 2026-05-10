package core

import (
	"runtime"
	"sync/atomic"
	"time"
)

type DynamicConfig struct {
	concurrency   atomic.Uint64
	timeoutSecs   atomic.Uint64
	avgLatencyMs  atomic.Uint64
	totalRequests atomic.Uint64
	totalErrors   atomic.Uint64

	// [OPTIM] Pre-computed OS cap — avoids repeated runtime.GOOS checks in hot paths.
	// Windows TCP stack limits effective concurrent HTTP connections before
	// throughput degrades (half-open connection limits, Winsock scheduling).
	hardCap uint64
}

func NewDynamicConfig() *DynamicConfig {
	c := &DynamicConfig{}

	// [OPTIM] Windows has a lower effective HTTP concurrency ceiling.
	// Beyond ~96 simultaneous connections the Winsock scheduler introduces
	// queuing delays that hurt more than the extra parallelism helps.
	if runtime.GOOS == "windows" {
		c.hardCap = 96
	} else {
		c.hardCap = 512
	}

	c.concurrency.Store(32)
	c.timeoutSecs.Store(28)
	c.avgLatencyMs.Store(500)
	return c
}

func (c *DynamicConfig) RecordRequest(duration time.Duration, isError bool) {
	ms := uint64(duration.Milliseconds())
	c.totalRequests.Add(1)

	if isError {
		c.totalErrors.Add(1)
		// [FIX] Replace Load+Store pair with CompareAndSwap loop to eliminate
		// the race window between reading and writing concurrency.
		// Previously two goroutines could both read 64, both store 32 — meaning
		// only one halving happened instead of two. Under heavy Windows parallelism
		// this caused the concurrency to stay too high during error bursts.
		for {
			current := c.concurrency.Load()
			if current <= 8 {
				break
			}
			next := current / 2
			if next < 8 {
				next = 8
			}
			if c.concurrency.CompareAndSwap(current, next) {
				break
			}
			// Another goroutine beat us — re-read and try again
		}
	} else {
		// [OPTIM] EWMA tuned for faster reaction to latency improvements.
		// Old factor 7/8 (alpha=0.125) weighted history too heavily — the system
		// took ~20 requests to reflect a 2x improvement in latency.
		// New factor 3/4 (alpha=0.25) reacts in ~6 requests while still smoothing
		// transient spikes.
		prevAvg := c.avgLatencyMs.Load()
		newAvg := (prevAvg*3 + ms) / 4
		c.avgLatencyMs.Store(newAvg)

		// [FIX] Same CAS loop for the increment path — avoids lost updates when
		// multiple goroutines try to raise concurrency simultaneously.
		for {
			current := c.concurrency.Load()
			if current >= c.hardCap {
				break
			}
			var delta uint64
			if ms < 200 {
				delta = 16
			} else if ms < 1000 {
				delta = 8
			} else {
				delta = 2
			}
			next := current + delta
			if next > c.hardCap {
				next = c.hardCap
			}
			if c.concurrency.CompareAndSwap(current, next) {
				break
			}
		}
	}
}

func (c *DynamicConfig) GetConcurrency() int {
	latency := c.avgLatencyMs.Load()
	current := c.concurrency.Load()

	// [OPTIM] Latency-based ceiling now respects the OS hard cap.
	// Values mirror the original thresholds but are clamped to hardCap,
	// so on Windows the effective ceiling is 96 regardless of latency bucket.
	var latencyCap uint64
	if latency > 5000 {
		latencyCap = 16
	} else if latency > 2000 {
		latencyCap = 32
	} else if latency > 1000 {
		latencyCap = 64
	} else if latency > 500 {
		latencyCap = 96
	} else {
		latencyCap = 128
	}

	// Apply OS hard cap — on Windows this keeps us under the Winsock ceiling
	cap := latencyCap
	if c.hardCap < cap {
		cap = c.hardCap
	}

	res := current
	if res > cap {
		res = cap
	}
	if res < 8 {
		res = 8
	}
	return int(res)
}

func (c *DynamicConfig) GetTimeout(attempt uint32) time.Duration {
	base := c.timeoutSecs.Load()
	patienceFactor := 1 + uint64(attempt*2)
	return time.Duration(base*patienceFactor) * time.Second
}