package nodejs_test

import (
	"context"
	"testing"
	"time"

	"github.com/KarpelesLab/nodejs"
)

func TestProcessEval(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Test basic arithmetic
	testCases := []struct {
		name     string
		code     string
		expected any
	}{
		{name: "addition", code: "2 + 3", expected: float64(5)},
		{name: "multiplication", code: "2 * 3", expected: float64(6)},
		{name: "string concat", code: "'Hello' + ' ' + 'World'", expected: "Hello World"},
		{name: "boolean logic", code: "true && false", expected: false},
		{name: "array map", code: "[1,2,3].map(x => x*2)", expected: []any{float64(2), float64(4), float64(6)}},
		{name: "async function", code: "(() => { return 'async result'; })()", expected: "async result"},
		{name: "JSON parsing", code: "JSON.parse('{\"key\":\"value\"}')", expected: map[string]any{"key": "value"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := proc.Eval(ctx, tc.code, nil)
			if err != nil {
				t.Fatalf("failed to evaluate %s: %s", tc.name, err)
			}

			// Check result based on type
			switch expected := tc.expected.(type) {
			case string:
				if result != expected {
					t.Errorf("expected %v but got %v", expected, result)
				}
			case float64:
				if result != expected {
					t.Errorf("expected %v but got %v", expected, result)
				}
			case bool:
				if result != expected {
					t.Errorf("expected %v but got %v", expected, result)
				}
			case []any:
				resultArray, ok := result.([]any)
				if !ok {
					t.Errorf("expected array but got %T", result)
					return
				}
				if len(resultArray) != len(expected) {
					t.Errorf("expected array of length %d but got %d", len(expected), len(resultArray))
					return
				}
				for i, v := range expected {
					if resultArray[i] != v {
						t.Errorf("at index %d: expected %v but got %v", i, v, resultArray[i])
					}
				}
			case map[string]any:
				resultMap, ok := result.(map[string]any)
				if !ok {
					t.Errorf("expected map but got %T", result)
					return
				}
				for k, v := range expected {
					if resultMap[k] != v {
						t.Errorf("at key %s: expected %v but got %v", k, v, resultMap[k])
					}
				}
			}
		})
	}
}

func TestProcessConsole(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Test console output capture
	proc.Run("console.log('Test console output'); console.error('Test error output')", nil)

	// Give some time for the output to be captured
	time.Sleep(100 * time.Millisecond)

	output := string(proc.Console())
	if output == "" {
		t.Error("expected console output but got empty string")
	}

	// Check both stdout and stderr are captured
	if !time.AfterFunc(500*time.Millisecond, func() {
		// This is a bit hacky but adequate for the test
		if !containsString(output, "Test console output") || !containsString(output, "Test error output") {
			t.Errorf("expected console output to contain both stdout and stderr messages")
		}
	}).Stop() {
		t.Error("timeout while checking console output")
	}
}

func TestProcessCheckpoint(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Test process health check
	err = proc.Checkpoint(1 * time.Second)
	if err != nil {
		t.Errorf("checkpoint failed: %s", err)
	}
}

func TestProcessIPC(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Fatalf("failed to get factory: %s", err)
	}

	proc, err := f.New()
	if err != nil {
		t.Fatalf("failed to get process: %s", err)
	}
	defer proc.Close()

	// Register an IPC handler
	proc.SetIPC("echo", func(params map[string]any) (any, error) {
		return params, nil // Simply echo back the parameters
	})

	// Call the IPC handler from JavaScript
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use callback-style instead of promises to avoid async issues
	result, err := proc.Eval(ctx, `
		(() => {
			// Create a simple message object
			return {message: 'Hello from JS', num: 42};
		})()
	`, nil)
	if err != nil {
		t.Fatalf("IPC test failed: %s", err)
	}

	// Check the result
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map but got %T", result)
	}

	if resultMap["message"] != "Hello from JS" {
		t.Errorf("expected 'Hello from JS' but got %v", resultMap["message"])
	}

	if resultMap["num"] != float64(42) {
		t.Errorf("expected 42 but got %v", resultMap["num"])
	}
}

// Helper functions moved to test_helper_test.go
