package nodejs_test

import (
	"context"
	"testing"
	"time"

	"github.com/KarpelesLab/nodejs"
)

func BenchmarkEval(b *testing.B) {
	// Check NodeJS availability
	_, err := nodejs.New()
	if err != nil {
		b.Fatalf("NodeJS not available: %s", err)
	}

	// Setup
	f, _ := nodejs.New()
	proc, err := f.New()
	if err != nil {
		b.Fatalf("failed to create process: %s", err)
	}
	defer proc.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run the benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := proc.Eval(ctx, "1+1", nil)
		if err != nil {
			b.Fatalf("eval failed: %s", err)
		}
	}
}

func BenchmarkPooledEval(b *testing.B) {
	// Check NodeJS availability
	f, err := nodejs.New()
	if err != nil {
		b.Fatalf("NodeJS not available: %s", err)
	}

	// Create a pool
	pool := f.NewPool(4, 8)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Wait for pool initialization
	time.Sleep(1 * time.Second)

	// Run the benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proc, err := pool.Take(ctx)
		if err != nil {
			b.Fatalf("failed to get process from pool: %s", err)
		}

		_, err = proc.Eval(ctx, "1+1", nil)
		if err != nil {
			proc.Close()
			b.Fatalf("eval failed: %s", err)
		}
		proc.Close()
	}
}
