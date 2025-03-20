package nodejs

import (
	"context"
	"testing"
	"time"
)

func TestContextCreation(t *testing.T) {
	factory, err := New()
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}

	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Basic evaluation in the context
	ctx := context.Background()
	result, err := jsCtx.Eval(ctx, "40 + 2", nil)
	if err != nil {
		t.Fatalf("Failed to evaluate code: %v", err)
	}

	expectedResult := float64(42)
	if result != expectedResult {
		t.Errorf("Expected result to be %v, got %v", expectedResult, result)
	}
}

func TestContextIsolation(t *testing.T) {
	factory, err := New()
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}

	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Create two separate contexts
	ctx1, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create first JavaScript context: %v", err)
	}
	defer ctx1.Close()

	ctx2, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create second JavaScript context: %v", err)
	}
	defer ctx2.Close()

	// Set a variable in the first context
	_, err = ctx1.Eval(context.Background(), "var sharedVar = 'context1'", nil)
	if err != nil {
		t.Fatalf("Failed to set variable in first context: %v", err)
	}

	// Set a different variable in the second context
	_, err = ctx2.Eval(context.Background(), "var sharedVar = 'context2'", nil)
	if err != nil {
		t.Fatalf("Failed to set variable in second context: %v", err)
	}

	// Check that variables are isolated to their respective contexts
	result1, err := ctx1.Eval(context.Background(), "sharedVar", nil)
	if err != nil {
		t.Fatalf("Failed to get variable from first context: %v", err)
	}

	result2, err := ctx2.Eval(context.Background(), "sharedVar", nil)
	if err != nil {
		t.Fatalf("Failed to get variable from second context: %v", err)
	}

	if result1 != "context1" {
		t.Errorf("Expected first context variable to be 'context1', got %v", result1)
	}

	if result2 != "context2" {
		t.Errorf("Expected second context variable to be 'context2', got %v", result2)
	}
}

func TestContextError(t *testing.T) {
	factory, err := New()
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}

	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Test syntax error
	_, err = jsCtx.Eval(context.Background(), "function() { return 'invalid syntax'; }", nil)
	if err == nil {
		t.Errorf("Expected syntax error but got nil")
	}

	// Test reference error
	_, err = jsCtx.Eval(context.Background(), "undefinedVariable", nil)
	if err == nil {
		t.Errorf("Expected reference error but got nil")
	}
}

func TestContextCancellation(t *testing.T) {
	factory, err := New()
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}

	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Run in a goroutine and cancel immediately
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Long-running JavaScript that should be cancelled
	_, err = jsCtx.Eval(ctx, "const start = Date.now(); while(Date.now() - start < 5000) {}", nil)
	if err == nil {
		t.Errorf("Expected context cancellation error but got nil")
	}
}

func TestContextClose(t *testing.T) {
	factory, err := New()
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}

	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}

	// Close the context
	err = jsCtx.Close()
	if err != nil {
		t.Fatalf("Failed to close context: %v", err)
	}

	// Attempt to use the closed context
	_, err = jsCtx.Eval(context.Background(), "40 + 2", nil)
	if err == nil {
		t.Errorf("Expected error when using closed context but got nil")
	}
}

func TestContextEvalChannel(t *testing.T) {
	factory, err := New()
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}

	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Simple evaluation using EvalChannel
	resChan, err := jsCtx.EvalChannel("40 + 2", nil)
	if err != nil {
		t.Fatalf("Failed to execute EvalChannel: %v", err)
	}

	// Wait for the result
	select {
	case res := <-resChan:
		// Check for error
		if errMsg, ok := res["error"].(string); ok {
			t.Fatalf("Received error from JavaScript: %s", errMsg)
		}

		// Check result value
		result, ok := res["res"].(float64)
		if !ok {
			t.Fatalf("Expected float64 result, got %T", res["res"])
		}

		if result != 42 {
			t.Errorf("Expected result to be 42, got %v", result)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for result")
	}

	// Test with async JavaScript - don't rely on setTimeout which isn't in VM context by default
	resChan, err = jsCtx.EvalChannel(`
		async function slowAdd(a, b) {
			// Simple async operation using Promise directly
			await Promise.resolve();
			return a + b;
		}
		slowAdd(20, 22)
	`, nil)
	if err != nil {
		t.Fatalf("Failed to execute async EvalChannel: %v", err)
	}

	// Wait for the result
	select {
	case res := <-resChan:
		// Check for error
		if errMsg, ok := res["error"].(string); ok {
			t.Fatalf("Received error from async JavaScript: %s", errMsg)
		}

		// Check result value
		result, ok := res["res"].(float64)
		if !ok {
			t.Fatalf("Expected float64 result from async code, got %T", res["res"])
		}

		if result != 42 {
			t.Errorf("Expected async result to be 42, got %v", result)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for async result")
	}
}
