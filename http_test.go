package nodejs_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	nodejs "github.com/KarpelesLab/nodejs"
)

func TestSimpleHTTPHandler(t *testing.T) {
	// Create a factory for NodeJS processes
	factory, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS factory: %v", err)
	}

	// Create a pool with default configuration
	pool := factory.NewPool(1, 1) // queueSize, maxProcs

	// Get a process from the pool
	proc, err := pool.Take(context.Background())
	if err != nil {
		t.Fatalf("Failed to get process from pool: %v", err)
	}

	// Define a simple HTTP handler function in JavaScript
	handlerCode := `
	// Define a simple handler
	function simpleHandler(request) {
		console.log("Simple handler called for:", request.method, request.url);
		
		// Create response body
		const responseData = {
			message: "Hello from Node.js",
			method: request.method,
			url: request.url,
			path: request.path
		};
		
		// Return a Response object
		return new Response(
			JSON.stringify(responseData),
			{
				status: 200,
				headers: {
					"Content-Type": "application/json",
					"X-Test-Header": "TestValue"
				}
			}
		);
	}
	
	// Export handler globally
	global.simpleHandler = simpleHandler;
	`

	// Evaluate the handler code
	_, err = proc.Eval(context.Background(), handlerCode, nil)
	if err != nil {
		t.Fatalf("Failed to evaluate handler code: %v", err)
	}

	// Test the handler
	t.Run("Simple HTTP handler", func(t *testing.T) {
		// Create a test HTTP request
		req := httptest.NewRequest(http.MethodGet, "http://localhost/test?param=value", nil)
		req.Header.Set("X-Custom-Header", "CustomValue")

		// Create a test response recorder
		w := httptest.NewRecorder()

		// Serve the HTTP request to our JavaScript handler
		proc.ServeHTTPToHandler("simpleHandler", w, req)

		// Get the response
		resp := w.Result()
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Check headers
		if resp.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type: application/json, got %s", resp.Header.Get("Content-Type"))
		}

		// Read body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		t.Logf("Response body: %s", string(body))

		// Parse JSON
		var respData map[string]interface{}
		if err := json.Unmarshal(body, &respData); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		// Check values
		if msg, ok := respData["message"].(string); !ok || msg != "Hello from Node.js" {
			t.Errorf("Expected message 'Hello from Node.js', got %v", respData["message"])
		}
	})
}

func TestContextProxyHandler(t *testing.T) {
	// Create a factory for NodeJS processes
	factory, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS factory: %v", err)
	}

	// Create a pool with default configuration
	pool := factory.NewPool(1, 1)

	// Get a process from the pool
	proc, err := pool.Take(context.Background())
	if err != nil {
		t.Fatalf("Failed to get process from pool: %v", err)
	}

	// Create the context on the Go side
	ctx := "testContext"

	// First, tell nodejs to create a VM context with this name
	_, err = proc.Eval(context.Background(), `platform.emit('create_context', {ctxid: '`+ctx+`'})`, nil)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Initialize the context with required objects and state
	initContextCode := `platform.emit('eval_in_context', {
		ctxid: '` + ctx + `',
		id: 'setup-` + ctx + `',
		data: 'this.counter = 0;'
	});`

	_, err = proc.Eval(context.Background(), initContextCode, nil)
	if err != nil {
		t.Fatalf("Failed to initialize context: %v", err)
	}

	// Define a handler function in the context
	handlerCode := `platform.emit('eval_in_context', {
		ctxid: '` + ctx + `',
		id: 'handler-` + ctx + `',
		data: 'this.handler = function(request) { this.counter++; return new Response(JSON.stringify({message: "Response from context", counter: this.counter, method: request.method}), {status: 200, headers: {"Content-Type": "application/json"}}); };'
	});`

	_, err = proc.Eval(context.Background(), handlerCode, nil)
	if err != nil {
		t.Fatalf("Failed to define handler in context: %v", err)
	}

	// Test the context handler
	t.Run("Context Handler via direct context", func(t *testing.T) {
		// Create a test HTTP request
		req := httptest.NewRequest(http.MethodGet, "http://localhost/context?param=value", nil)
		req.Header.Set("X-Context-Test", "TestValue")

		// Create a test response recorder
		w := httptest.NewRecorder()

		// Serve the HTTP request to our context handler
		// Use the "context.handler" name format
		proc.ServeHTTPToHandler(ctx+".handler", w, req)

		// Get the response
		resp := w.Result()
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		t.Logf("Response body: %s", string(body))

		// Parse JSON
		var respData map[string]interface{}
		if err := json.Unmarshal(body, &respData); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		// Counter should be 1 for first request
		counter, ok := respData["counter"].(float64)
		if !ok || counter != 1 {
			t.Errorf("Expected counter to be 1, got %v", respData["counter"])
		}

		// Make a second request to check if state is maintained
		req2 := httptest.NewRequest(http.MethodGet, "http://localhost/context2", nil)
		w2 := httptest.NewRecorder()
		proc.ServeHTTPToHandler(ctx+".handler", w2, req2)

		// Check the second response
		resp2 := w2.Result()
		defer resp2.Body.Close()

		body2, err := io.ReadAll(resp2.Body)
		if err != nil {
			t.Fatalf("Failed to read second response body: %v", err)
		}

		t.Logf("Second response body: %s", string(body2))

		// Parse second response
		var respData2 map[string]interface{}
		if err := json.Unmarshal(body2, &respData2); err != nil {
			t.Fatalf("Failed to parse second response JSON: %v", err)
		}

		// Counter should be 2 for second request
		counter2, ok := respData2["counter"].(float64)
		if !ok || counter2 != 2 {
			t.Errorf("Expected counter to be 2, got %v", respData2["counter"])
		}
	})
}
