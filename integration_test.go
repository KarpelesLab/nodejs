package nodejs_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/KarpelesLab/nodejs"
)

func TestErrorHandling(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Test cases with error conditions
	testCases := []struct {
		name        string
		code        string
		expectedErr string
	}{
		{
			name:        "syntax error",
			code:        "const x = 'unclosed string",
			expectedErr: "SyntaxError",
		},
		{
			name:        "reference error",
			code:        "nonExistentVariable + 1",
			expectedErr: "ReferenceError",
		},
		{
			name:        "type error",
			code:        "const obj = null; obj.property",
			expectedErr: "TypeError",
		},
		{
			name:        "throw error",
			code:        "throw new Error('Custom error message');",
			expectedErr: "Custom error message",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := proc.Eval(ctx, tc.code, nil)
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s' but got: %s", tc.expectedErr, err)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run code that takes longer than the timeout
	_, err = proc.Eval(ctx, `
		new Promise(resolve => {
			// This will run for 5 seconds, but context will timeout before that
			setTimeout(() => resolve(42), 5000);
		});
	`, nil)

	// Check that we got a context timeout error
	if err == nil {
		t.Fatal("expected context deadline exceeded error, but got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded but got: %s", err)
	}
}

func TestLargeDataHandling(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate a large array in JavaScript
	result, err := proc.Eval(ctx, `
		// Generate an array with 10,000 elements
		const largeArray = Array.from({ length: 10000 }, (_, i) => i);
		// Return the length as a basic check
		largeArray.length;
	`, nil)

	if err != nil {
		t.Fatalf("failed to handle large data: %s", err)
	}

	if result != float64(10000) {
		t.Errorf("expected 10000 but got %v", result)
	}
}

func TestProcessReuse(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test that we can set variables and access them in later calls
	_, err = proc.Eval(ctx, "globalVariable = 'test value'", nil)
	if err != nil {
		t.Fatalf("failed to set global variable: %s", err)
	}

	// Try to access the variable in a subsequent call
	result, err := proc.Eval(ctx, "globalVariable", nil)
	if err != nil {
		t.Fatalf("failed to get global variable: %s", err)
	}

	if result != "test value" {
		t.Errorf("expected 'test value' but got %v", result)
	}

	// Test using Set method to set a variable
	proc.Set("setFromGo", "Go-set value")

	// Verify we can access it
	result, err = proc.Eval(ctx, "setFromGo", nil)
	if err != nil {
		t.Fatalf("failed to get variable set from Go: %s", err)
	}

	if result != "Go-set value" {
		t.Errorf("expected 'Go-set value' but got %v", result)
	}
}
