package nodejs_test

import (
	"context"
	"testing"
	"time"

	"github.com/KarpelesLab/nodejs"
)

func TestFactoryCreation(t *testing.T) {
	// Test basic factory creation
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}
	if f == nil {
		t.Fatal("factory is nil despite no error")
	}
}

func TestFactoryNewProcess(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	// Test creating a process with default timeout
	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Test basic JavaScript evaluation
	v, err := proc.Eval(context.Background(), "\"This Is a STRING\".toLowerCase()", map[string]any{"filename": "test.js"})
	if err != nil {
		t.Fatalf("failed to run test: %s", err)
	}
	if v != "this is a string" {
		t.Errorf("expected 'this is a string' but got '%v'", v)
	}
}

func TestFactoryNewWithTimeout(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	// Test creating a process with custom timeout
	proc, err := f.NewWithTimeout(10 * time.Second)
	if err != nil {
		t.Fatalf("failed to get process with custom timeout: %s", err)
	}
	defer proc.Close()

	// Verify the process works
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := proc.Eval(ctx, "21 * 2", nil)
	if err != nil {
		t.Fatalf("failed to evaluate JS: %s", err)
	}
	if result != float64(42) {
		t.Errorf("expected 42 but got %v", result)
	}
}

func TestFactoryNodeVersion(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Check that we can get the NodeJS version
	version := proc.GetVersion("node")
	if version == "" {
		t.Error("could not retrieve NodeJS version")
	}
	t.Logf("NodeJS version: %s", version)
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
			code:     "async function test() { return new Promise(resolve => resolve(42)); }; test()",
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
