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
