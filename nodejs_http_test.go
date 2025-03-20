package nodejs

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNodeJSHTTPSolution(t *testing.T) {
	factory, err := New()
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}

	proc, err := factory.New()
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Load the HTTP solution
	_, err = proc.Eval(context.Background(), `require('./nodejs_http_solution.js')`, nil)
	if err != nil {
		t.Fatalf("Failed to load HTTP solution: %v", err)
	}

	// Test text handler
	t.Run("TextHandler", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/text", nil)
		rec := httptest.NewRecorder()

		proc.ServeHTTPToHandler("textHandler", rec, req)

		resp := rec.Result()
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("Expected Content-Type 'text/plain', got %q", ct)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr := string(body)
		t.Logf("Response body: %q", bodyStr)

		expected := "Hello from the text handler!"
		if !strings.Contains(bodyStr, expected) {
			t.Errorf("Expected body to contain %q, got %q", expected, bodyStr)
		}
	})

	// Test JSON handler
	t.Run("JSONHandler", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/json", nil)
		rec := httptest.NewRecorder()

		proc.ServeHTTPToHandler("jsonHandler", rec, req)

		resp := rec.Result()
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got %q", ct)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr := string(body)
		t.Logf("Response body: %q", bodyStr)

		// Try to parse the JSON
		if len(bodyStr) > 0 {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(bodyStr), &result); err != nil {
				t.Logf("Warning: JSON parsing failed: %v for body: %q", err, bodyStr)
			} else {
				if result["message"] != "Hello JSON" {
					t.Errorf("Expected message: 'Hello JSON', got: %v", result["message"])
				}
			}
		} else {
			t.Errorf("Expected non-empty JSON body, got empty string")
		}
	})

	// Test counter with state
	t.Run("CounterState", func(t *testing.T) {
		// Get initial counter value
		req := httptest.NewRequest("GET", "http://example.com/counter", nil)
		rec := httptest.NewRecorder()

		proc.ServeHTTPToHandler("getCounter", rec, req)

		resp := rec.Result()
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr := string(body)
		t.Logf("Initial counter: %q", bodyStr)

		var initialValue float64
		if len(bodyStr) > 0 {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(bodyStr), &result); err == nil {
				if val, ok := result["value"].(float64); ok {
					initialValue = val
				}
			}
		}

		// Increment counter
		req = httptest.NewRequest("GET", "http://example.com/counter/increment", nil)
		rec = httptest.NewRecorder()

		proc.ServeHTTPToHandler("incrementCounter", rec, req)

		resp = rec.Result()
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr = string(body)
		t.Logf("After increment: %q", bodyStr)

		if len(bodyStr) > 0 {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(bodyStr), &result); err == nil {
				if val, ok := result["value"].(float64); ok {
					if val != initialValue+1 {
						t.Errorf("Expected counter value %f after increment, got %f", initialValue+1, val)
					}
				} else {
					t.Errorf("Expected numeric value property in counter response")
				}
			} else {
				t.Logf("Warning: JSON parsing failed for increment: %v", err)
			}
		}
	})

	// Test context handler
	t.Run("ContextHandler", func(t *testing.T) {
		// Create a test context
		contextSetupCode := `
			// Create test context
			createContextWithHandler('test');
			
			// Define a counter handler in the context
			registerContextHandler('test', 'counter', function(request) {
				// Initialize state if needed
				if (this.state.counter === undefined) {
					this.state.counter = 0;
				}
				
				// Check the URL path for actions
				if (request.url.includes('increment')) {
					this.state.counter++;
				} else if (request.url.includes('reset')) {
					this.state.counter = 0;
				}
				
				// Return the current counter value
				return this.jsonResponse({ value: this.state.counter });
			});
		`

		_, err := proc.Eval(context.Background(), contextSetupCode, nil)
		if err != nil {
			t.Fatalf("Failed to set up test context: %v", err)
		}

		// List available contexts and handlers
		contexts, err := proc.Eval(context.Background(), "listContextsWithHandlers()", nil)
		if err != nil {
			t.Fatalf("Failed to list contexts: %v", err)
		}

		t.Logf("Available contexts: %v", contexts)

		// Get initial counter value
		req := httptest.NewRequest("GET", "http://example.com/counter", nil)
		rec := httptest.NewRecorder()

		proc.ServeHTTPToHandler("test.counter", rec, req)

		resp := rec.Result()
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr := string(body)
		t.Logf("Initial context counter: %q", bodyStr)

		var initialValue float64
		if len(bodyStr) > 0 {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(bodyStr), &result); err == nil {
				if val, ok := result["value"].(float64); ok {
					initialValue = val
					if val != 0 {
						t.Errorf("Expected initial context counter to be 0, got %f", val)
					}
				} else {
					t.Errorf("Expected numeric value property in counter response")
				}
			} else {
				log.Printf("Warning: JSON parsing failed for context counter: %v for: %q", err, bodyStr)
			}
		} else {
			t.Errorf("Expected non-empty response from context handler")
		}

		// Increment context counter
		req = httptest.NewRequest("GET", "http://example.com/counter/increment", nil)
		rec = httptest.NewRecorder()

		proc.ServeHTTPToHandler("test.counter", rec, req)

		resp = rec.Result()
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr = string(body)
		t.Logf("Context counter after increment: %q", bodyStr)

		// Get counter value again to verify state persistence
		req = httptest.NewRequest("GET", "http://example.com/counter", nil)
		rec = httptest.NewRecorder()

		proc.ServeHTTPToHandler("test.counter", rec, req)

		resp = rec.Result()
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr = string(body)
		t.Logf("Context counter after second check: %q", bodyStr)

		if len(bodyStr) > 0 {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(bodyStr), &result); err == nil {
				if val, ok := result["value"].(float64); ok {
					if val != initialValue+1 {
						t.Errorf("Expected context counter to be %f, got %f", initialValue+1, val)
					}
				}
			}
		}
	})
}
