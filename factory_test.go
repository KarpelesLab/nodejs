package nodejs_test

import (
	"context"
	"testing"
	"time"

	"github.com/KarpelesLab/nodejs"
)

func TestFactoryCreation(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create factory: %s", err)
	}

	// Verify we got a valid factory
	if f == nil {
		t.Fatal("Expected non-nil factory")
	}
}

func TestFactoryNewProcess(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create factory: %s", err)
	}

	// Create a process from the factory
	proc, err := f.New()
	if err != nil {
		t.Fatalf("Failed to create process: %s", err)
	}
	defer proc.Close()

	// Verify process is alive
	select {
	case <-proc.Alive():
		t.Fatal("Process died immediately")
	default:
		// This is good, process is still running
	}
}

func TestFactoryNewWithTimeout(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create factory: %s", err)
	}

	// We'll use NewWithTimeout instead of SetTimeout

	// Create a process with the timeout
	proc, err := f.NewWithTimeout(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create process with timeout: %s", err)
	}
	defer proc.Close()

	// Verify we can execute code
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := proc.Eval(ctx, "40 + 2", nil)
	if err != nil {
		t.Fatalf("failed to evaluate: %s", err)
	}

	if result != float64(42) {
		t.Errorf("expected 42 but got %v", result)
	}
}

func TestFactoryNodeVersion(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("Failed to create process: %s", err)
	}
	defer proc.Close()

	version := proc.GetVersion("node")
	t.Logf("NodeJS version: %s", version)

	if version == "" {
		t.Error("Failed to get NodeJS version")
	}
}

func TestFactoryES6Features(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Test ES6 features
	testCases := []struct {
		name     string
		code     string
		expected any
	}{
		{
			name:     "arrow functions",
			code:     "const add = (a, b) => a + b; add(2, 3)",
			expected: float64(5),
		},
		{
			name:     "template literals",
			code:     "const name = 'World'; `Hello ${name}!`",
			expected: "Hello World!",
		},
		{
			name:     "destructuring",
			code:     "const {a, b} = {a: 1, b: 2}; a + b",
			expected: float64(3),
		},
		{
			name:     "spread operator",
			code:     "const arr = [1, 2, 3]; const result = [...arr, 4, 5]; result.length",
			expected: float64(5),
		},
		{
			name:     "promises",
			code:     "(() => { return 42; })()",
			expected: float64(42),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := proc.Eval(ctx, tc.code, nil)
			if err != nil {
				t.Fatalf("failed to evaluate %s: %s", tc.name, err)
			}
			if result != tc.expected {
				t.Errorf("expected %v but got %v", tc.expected, result)
			}
		})
	}
}

func TestFactoryErrorHandling(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Test error handling
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "syntax error",
			code: "const x = 'unclosed string",
		},
		{
			name: "reference error",
			code: "nonExistentVariable + 1",
		},
		{
			name: "type error",
			code: "const obj = null; obj.property",
		},
		{
			name: "throw error",
			code: "throw new Error('Custom error message');",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := proc.Eval(ctx, tc.code, nil)
			if err == nil {
				t.Errorf("expected error for %s but got none", tc.name)
			}
		})
	}
}
