package nodejs_test

import (
	"context"
	"testing"
	"time"

	"github.com/KarpelesLab/nodejs"
)

// TestBasicFunctionality is a minimal test that should work in most environments
func TestBasicFunctionality(t *testing.T) {
	// Create factory
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to create factory: %s", err)
	}

	// Create process
	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to create process: %s", err)
	}
	defer proc.Close()

	// Basic evaluation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := proc.Eval(ctx, "2 + 2", nil)
	if err != nil {
		t.Fatalf("failed to evaluate simple expression: %s", err)
	}

	if result != float64(4) {
		t.Errorf("expected 4 but got %v", result)
	}

	logIfVerbose(t, "Basic evaluation successful")
}
