package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/utils"
)

func TestAdaptiveSemaphoreHighLoad(t *testing.T) {
	sem := utils.NewAdaptiveSemaphore(10)
	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := sem.Acquire(ctx, 10)
			if err != nil {
				t.Errorf("Worker %d failed: %v", id, err)
				return
			}
			time.Sleep(10 * time.Millisecond)
			sem.Release()
		}(i)
	}

	wg.Wait()
}
