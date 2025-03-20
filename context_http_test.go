package nodejs_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestLargeHTTPResponse(t *testing.T) {
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

	// Define a handler in the context that generates a large response
	handlerCode := `
	// Define a handler that returns a large JSON response
	this.largeResponseHandler = function(request) {
		// Create a large array with 10,000 items
		const largeArray = [];
		for (let i = 0; i < 10000; i++) {
			largeArray.push({
				index: i,
				value: "Item " + i,
				timestamp: new Date().toISOString(),
				randomValue: Math.random().toString(36).substring(2),
			});
		}
		
		// Create large object with the array and additional metadata
		const responseData = {
			message: "Large response test",
			method: request.method,
			path: request.path,
			timestamp: new Date().toISOString(),
			items: largeArray
		};
		
		// Return a Response object with the large JSON
		return new Response(
			JSON.stringify(responseData),
			{
				status: 200,
				headers: {
					"Content-Type": "application/json",
					"X-Large-Response": "true"
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

	// Create a test HTTP request
	req := httptest.NewRequest(http.MethodGet, "http://localhost/large-data", nil)
	
	// Create a test response recorder
	w := httptest.NewRecorder()
	
	// Measure response time to ensure streaming works properly
	startTime := time.Now()
	
	// Serve the HTTP request to our JavaScript handler in the context
	jsCtx.ServeHTTPToHandler("largeResponseHandler", w, req)
	
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
	
	if resp.Header.Get("X-Large-Response") != "true" {
		t.Errorf("Expected X-Large-Response: true, got %s", resp.Header.Get("X-Large-Response"))
	}
	
	// Read body in chunks to verify streaming capability
	var totalBytes int64
	buffer := make([]byte, 4096) // 4KB buffer
	
	for {
		n, err := resp.Body.Read(buffer)
		
		if err == io.EOF {
			break
		}
		
		if err != nil {
			t.Fatalf("Error reading response body: %v", err)
		}
		
		totalBytes += int64(n)
	}
	
	// Log response size and timing information
	responseTime := time.Since(startTime)
	t.Logf("Large response size: %d bytes, time: %v", totalBytes, responseTime)
	
	// Verify that we received a substantial amount of data (at least 1MB)
	if totalBytes < 1000000 {
		t.Errorf("Expected large response (>1MB), got only %d bytes", totalBytes)
	}
	
	// Also test reading the entire response at once to compare with chunked reading
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/large-data", nil)
	w2 := httptest.NewRecorder()
	startTime2 := time.Now()
	
	// Serve the request again
	jsCtx.ServeHTTPToHandler("largeResponseHandler", w2, req2)
	resp2 := w2.Result()
	defer resp2.Body.Close()
	
	// Read the entire body at once
	fullBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("Error reading full response body: %v", err)
	}
	
	responseTime2 := time.Since(startTime2)
	t.Logf("Full read - Large response size: %d bytes, time: %v", len(fullBody), responseTime2)
	
	// Verify that we got approximately the same amount of data both ways
	// Allow for a small margin of error (0.1%) due to buffering differences
	sizeDiff := math.Abs(float64(len(fullBody)) - float64(totalBytes))
	diffPercent := (sizeDiff / float64(totalBytes)) * 100
	
	if diffPercent > 0.1 { // Allow 0.1% difference
		t.Errorf("Inconsistent response sizes: chunked=%d bytes, full=%d bytes (diff: %.2f%%)", 
			totalBytes, len(fullBody), diffPercent)
	} else {
		t.Logf("Response size difference is within acceptable range: %.2f%% difference", diffPercent)
	}
	
	// Parse the JSON to verify it's valid
	var respData map[string]interface{}
	if err := json.Unmarshal(fullBody, &respData); err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}
	
	// Check some expected values
	if msg, ok := respData["message"].(string); !ok || msg != "Large response test" {
		t.Errorf("Expected message 'Large response test', got %v", respData["message"])
	}
	
	// Check that the items array exists and has 10,000 elements
	items, ok := respData["items"].([]interface{})
	if !ok {
		t.Fatalf("Expected items to be an array, got %T", respData["items"])
	}
	
	if len(items) != 10000 {
		t.Errorf("Expected 10,000 items, got %d", len(items))
	}
}