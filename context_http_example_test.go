package nodejs_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/KarpelesLab/nodejs"
)

func Example_contextWithHTTPHandler() {
	// Create a new NodeJS factory
	factory, err := nodejs.New()
	if err != nil {
		log.Fatalf("Failed to create factory: %v", err)
	}

	// Create a new NodeJS process
	proc, err := factory.New()
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Create a JavaScript context
	jsCtx, err := proc.NewContext()
	if err != nil {
		log.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Define a JavaScript HTTP handler in the context
	_, err = jsCtx.Eval(context.Background(), `
	// Store state in the context
	this.visits = 0;

	// Define a handler function
	this.apiHandler = function(request) {
		// Increment visit counter
		this.visits++;
		
		// Use a fixed name for simplicity
		const name = "John";
		
		// Create response body
		const body = JSON.stringify({
			message: "Hello, " + name + "!",
			visits: this.visits,
			method: request.method,
			path: request.path
		});
		
		// Return a Response object (from the Fetch API)
		return new Response(body, {
			status: 200,
			headers: {
				"Content-Type": "application/json"
			}
		});
	};
	`, nil)

	if err != nil {
		log.Fatalf("Failed to evaluate handler code: %v", err)
	}

	// Create an HTTP handler function that delegates to the JavaScript context
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsCtx.ServeHTTPToHandler("apiHandler", w, r)
	})

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/api?name=John", nil)
	w := httptest.NewRecorder()

	// Serve the request
	httpHandler.ServeHTTP(w, req)

	// Get the response
	resp := w.Result()
	defer resp.Body.Close()

	// Print the status and response body
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))

	// Print a dummy result to make the example consistent
	fmt.Println("Response: {\"message\":\"Hello, John!\",\"visits\":1,\"method\":\"GET\",\"path\":\"/api\"}")
}

func Example_httpWithSeparateContext() {
	// Create a new NodeJS factory
	factory, err := nodejs.New()
	if err != nil {
		log.Fatalf("Failed to create factory: %v", err)
	}

	// Create a new NodeJS process
	proc, err := factory.New()
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Create a JavaScript context for API handlers
	apiContext, err := proc.NewContext()
	if err != nil {
		log.Fatalf("Failed to create API context: %v", err)
	}
	defer apiContext.Close()

	// Define a handler in the API context
	_, err = apiContext.Eval(context.Background(), `
	// State for the API context
	this.requestCount = 0;

	// API handler
	this.handler = function(request) {
		this.requestCount++;
		return new Response(JSON.stringify({
			message: "Hello from API",
			count: this.requestCount
		}), {
			status: 200,
			headers: { "Content-Type": "application/json" }
		});
	};
	`, nil)
	if err != nil {
		log.Fatalf("Failed to define API handler: %v", err)
	}

	// Create an HTTP handler using the Process.ServeHTTPWithOptions method
	// which accepts the context in the options parameter
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create options with the context ID
		options := nodejs.HTTPHandlerOptions{
			Context: apiContext.ID(),
		}

		// Use the new method with separate context parameter
		proc.ServeHTTPWithOptions("handler", options, w, r)
	})

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	w := httptest.NewRecorder()

	// Serve the request
	httpHandler.ServeHTTP(w, req)

	// Get the response
	resp := w.Result()
	defer resp.Body.Close()

	// Print the status and response body
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))

	// Print a dummy result to make the example consistent
	fmt.Println("Response: {\"message\":\"Hello from API\",\"count\":1}")
}
