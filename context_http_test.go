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

func TestContextServeHTTPToHandler(t *testing.T) {
	// Skip this test if running under -short flag
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}
	// Create a factory for NodeJS processes
	factory, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS factory: %v", err)
	}

	// Create a NodeJS process
	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS process: %v", err)
	}
	defer proc.Close()

	// Create a JavaScript context
	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Define a handler in the context
	handlerCode := `
	// Define a state variable in this context
	this.requestCount = 0;

	// Define an HTTP handler
	this.httpHandler = function(request) {
		// Increment request counter
		this.requestCount++;
		
		// No need to parse URL - just use the 'Guest' name
		const name = 'TestUser';
		
		// Create response data
		const responseData = {
			message: "Hello, " + name + "!",
			count: this.requestCount,
			method: request.method,
			path: request.path
		};
		
		// Return a Response object
		return new Response(
			JSON.stringify(responseData),
			{
				status: 200,
				headers: {
					"Content-Type": "application/json",
					"X-Context-Header": "ContextTest"
				}
			}
		);
	};
	`

	// Evaluate the handler code in the context
	_, err = jsCtx.Eval(context.Background(), handlerCode, nil)
	if err != nil {
		t.Fatalf("Failed to evaluate handler code: %v", err)
	}

	// Test the handler with multiple requests to verify state persistence
	for i := 1; i <= 3; i++ {
		// Create a test HTTP request
		req := httptest.NewRequest(http.MethodGet, "http://localhost/test?name=TestUser", nil)
		req.Header.Set("X-Test-Header", "TestValue")

		// Create a test response recorder
		w := httptest.NewRecorder()

		// Serve the HTTP request to our JavaScript handler in the context
		jsCtx.ServeHTTPToHandler("httpHandler", w, req)

		// Get the response
		resp := w.Result()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Request %d: Expected status 200, got %d", i, resp.StatusCode)
		}

		// Check headers
		if resp.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Request %d: Expected Content-Type: application/json, got %s", i, resp.Header.Get("Content-Type"))
		}

		if resp.Header.Get("X-Context-Header") != "ContextTest" {
			t.Errorf("Request %d: Expected X-Context-Header: ContextTest, got %s", i, resp.Header.Get("X-Context-Header"))
		}

		// Read body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Request %d: Failed to read response body: %v", i, err)
		}

		t.Logf("Request %d response body: %s", i, string(body))

		// Parse JSON
		var respData map[string]interface{}
		if err := json.Unmarshal(body, &respData); err != nil {
			t.Fatalf("Request %d: Failed to parse response JSON: %v", i, err)
		}

		// Check message value
		if msg, ok := respData["message"].(string); !ok || msg != "Hello, TestUser!" {
			t.Errorf("Request %d: Expected message 'Hello, TestUser!', got %v", i, respData["message"])
		}

		// Check counter value
		count, ok := respData["count"].(float64)
		if !ok || int(count) != i {
			t.Errorf("Request %d: Expected count %d, got %v", i, i, respData["count"])
		}
	}
}

func TestContextClosedHTTPHandler(t *testing.T) {
	// Skip this test if running under -short flag
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}
	// Create a factory for NodeJS processes
	factory, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS factory: %v", err)
	}

	// Create a NodeJS process
	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS process: %v", err)
	}
	defer proc.Close()

	// Create a JavaScript context
	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}

	// Define a handler in the context
	_, err = jsCtx.Eval(context.Background(), "this.handler = function(req) { return new Response('OK'); };", nil)
	if err != nil {
		t.Fatalf("Failed to define handler: %v", err)
	}

	// Close the context
	err = jsCtx.Close()
	if err != nil {
		t.Fatalf("Failed to close context: %v", err)
	}

	// Create a test HTTP request
	req := httptest.NewRequest(http.MethodGet, "http://localhost/test", nil)

	// Create a test response recorder
	w := httptest.NewRecorder()

	// Try to serve the HTTP request to the closed context
	jsCtx.ServeHTTPToHandler("handler", w, req)

	// Get the response
	resp := w.Result()
	defer resp.Body.Close()

	// Check status code - should be a server error
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for closed context, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServeHTTPWithOptions(t *testing.T) {
	// Skip this test if running under -short flag
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}
	// Create a factory for NodeJS processes
	factory, err := nodejs.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS factory: %v", err)
	}

	// Create a NodeJS process
	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create NodeJS process: %v", err)
	}
	defer proc.Close()

	// Create a JavaScript context
	jsCtx, err := proc.NewContext()
	if err != nil {
		t.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Define a handler in the context
	handlerCode := `
	// Define a state variable in this context
	this.requestCount = 0;

	// Define an HTTP handler
	this.handler = function(request) {
		// Increment request counter
		this.requestCount++;
		
		// Create response data
		const responseData = {
			message: "Hello from handler",
			count: this.requestCount,
			method: request.method,
			path: request.path
		};
		
		// Return a Response object
		return new Response(
			JSON.stringify(responseData),
			{
				status: 200,
				headers: {
					"Content-Type": "application/json",
					"X-Handler-Header": "OptionsTest"
				}
			}
		);
	};
	`

	// Evaluate the handler code in the context
	_, err = jsCtx.Eval(context.Background(), handlerCode, nil)
	if err != nil {
		t.Fatalf("Failed to evaluate handler code: %v", err)
	}

	// Test the handler with multiple requests to verify state persistence
	for i := 1; i <= 3; i++ {
		// Create a test HTTP request
		req := httptest.NewRequest(http.MethodGet, "http://localhost/test", nil)
		req.Header.Set("X-Test-Header", "TestValue")

		// Create a test response recorder
		w := httptest.NewRecorder()

		// Serve the HTTP request using ServeHTTPWithOptions
		options := nodejs.HTTPHandlerOptions{
			Context: jsCtx.ID(),
		}
		proc.ServeHTTPWithOptions("handler", options, w, req)

		// Get the response
		resp := w.Result()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Request %d: Expected status 200, got %d", i, resp.StatusCode)
		}

		// Check headers
		if resp.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Request %d: Expected Content-Type: application/json, got %s", i, resp.Header.Get("Content-Type"))
		}

		if resp.Header.Get("X-Handler-Header") != "OptionsTest" {
			t.Errorf("Request %d: Expected X-Handler-Header: OptionsTest, got %s", i, resp.Header.Get("X-Handler-Header"))
		}

		// Read body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() // Close the body after reading

		if err != nil {
			t.Fatalf("Request %d: Failed to read response body: %v", i, err)
		}

		t.Logf("Request %d response body: %s", i, string(body))

		// Parse JSON
		var respData map[string]interface{}
		if err := json.Unmarshal(body, &respData); err != nil {
			t.Fatalf("Request %d: Failed to parse response JSON: %v", i, err)
		}

		// Check message value
		if msg, ok := respData["message"].(string); !ok || msg != "Hello from handler" {
			t.Errorf("Request %d: Expected message 'Hello from handler', got %v", i, respData["message"])
		}

		// Check counter value
		count, ok := respData["count"].(float64)
		if !ok || int(count) != i {
			t.Errorf("Request %d: Expected count %d, got %v", i, i, respData["count"])
		}
	}
}
