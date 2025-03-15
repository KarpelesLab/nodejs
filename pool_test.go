package nodejs_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/KarpelesLab/nodejs"
)

func TestNewPool(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	// Test default pool size
	pool := f.NewPool(0, 0)
	if pool == nil {
		t.Fatal("failed to create pool with default size")
	}

	// Test explicit pool size
	pool = f.NewPool(2, 4)
	if pool == nil {
		t.Fatal("failed to create pool with explicit size")
	}
}

func TestPoolTake(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	// Create a pool with a small size
	pool := f.NewPool(2, 4)

	// Take a process with context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	proc, err := pool.Take(ctx)
	if err != nil {
		t.Fatalf("failed to take process from pool: %s", err)
	}
	if proc == nil {
		t.Fatal("received nil process from pool")
	}

	// Test the process works
	result, err := proc.Eval(ctx, "40 + 2", nil)
	if err != nil {
		t.Fatalf("failed to evaluate JS: %s", err)
	}
	if result != float64(42) {
		t.Errorf("expected 42 but got %v", result)
	}

	// Return the process to the pool
	proc.Close()
}

func TestPoolTakeIfAvailable(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	// Create a pool
	pool := f.NewPool(1, 2)

	// Give time for the first process to be created
	time.Sleep(1 * time.Second)

	// Take the first available process
	proc1 := pool.TakeIfAvailable()
	if proc1 == nil {
		t.Fatal("TakeIfAvailable returned nil, expected a process")
	}
	defer proc1.Close()

	// Immediately try to take another, which may not be available yet
	proc2 := pool.TakeIfAvailable()
	if proc2 != nil {
		// If we got a second process, make sure to close it
		defer proc2.Close()
	}
	// We don't assert proc2 != nil because it depends on timing
}

func TestPoolTakeTimeout(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	// Create a pool
	pool := f.NewPool(1, 2)

	// Wait for pool to initialize
	time.Sleep(1 * time.Second)

	// Take a process with timeout
	proc, err := pool.TakeTimeout(2 * time.Second)
	if err != nil {
		t.Fatalf("failed to take process with timeout: %s", err)
	}
	if proc == nil {
		t.Fatal("received nil process from pool")
	}
	defer proc.Close()

	// Test intentional timeout
	// First take all available processes
	procs := []*nodejs.Process{}
	for i := 0; i < 2; i++ {
		p := pool.TakeIfAvailable()
		if p != nil {
			procs = append(procs, p)
		}
	}

	// Now try to take another with a very short timeout
	_, err = pool.TakeTimeout(100 * time.Millisecond)
	if err == nil {
		t.Error("expected timeout error but got nil")
	}

	// Cleanup
	for _, p := range procs {
		p.Close()
	}
}

func TestPoolConcurrent(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	// Create a small pool
	pool := f.NewPool(2, 4)

	// Wait for the pool to initialize
	time.Sleep(1 * time.Second)

	// Run concurrent operations
	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)

	// Each worker will take a process, run a simple operation, and return it
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			proc, err := pool.Take(ctx)
			if err != nil {
				t.Errorf("worker %d failed to take process: %s", i, err)
				return
			}
			defer proc.Close()

			// Simple operation
			result, err := proc.Eval(ctx, "42", nil)
			if err != nil {
				t.Errorf("worker %d eval failed: %s", i, err)
				return
			}
			if result != float64(42) {
				t.Errorf("worker %d expected 42 but got %v", i, result)
			}

			// Small delay to simulate work
			time.Sleep(100 * time.Millisecond)
		}(i)
	}

	// Wait for all workers to complete
	wg.Wait()
}
