package core

import (
	"sync/atomic"
	"time"
)

type DynamicConfig struct {
	concurrency   atomic.Uint64
	timeoutSecs   atomic.Uint64
	avgLatencyMs  atomic.Uint64
	totalRequests atomic.Uint64
	totalErrors   atomic.Uint64
}

func NewDynamicConfig() *DynamicConfig {
	c := &DynamicConfig{}
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
		current := c.concurrency.Load()
		if current > 8 {
			c.concurrency.Store(current / 2)
		}
	} else {
		prevAvg := c.avgLatencyMs.Load()
		newAvg := (prevAvg*7 + ms) / 8
		c.avgLatencyMs.Store(newAvg)

		current := c.concurrency.Load()
		if current < 512 {
			if ms < 200 {
				c.concurrency.Add(16)
			} else if ms < 1000 {
				c.concurrency.Add(8)
			} else {
				c.concurrency.Add(2)
			}
		}
	}
}

func (c *DynamicConfig) GetConcurrency() int {
	latency := c.avgLatencyMs.Load()
	current := c.concurrency.Load()

	var max uint64
	if latency > 5000 {
		max = 16
	} else if latency > 2000 {
		max = 32
	} else if latency > 1000 {
		max = 64
	} else if latency > 500 {
		max = 96
	} else {
		max = 128
	}

	res := current
	if res > max {
		res = max
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
