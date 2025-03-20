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

	// Set up the proxy pattern for context handlers
	proxySetupCode := `
	// Keep track of contexts and their state
	const contexts = {};
	
	// Create and setup a context
	function setupContext(contextName) {
		console.log("Setting up context:", contextName);
		
		// Create the VM context
		platform.emit('create_context', {ctxid: contextName});
		
		// Initialize the context with required objects
		platform.emit('eval_in_context', {
			ctxid: contextName,
			id: 'setup-' + contextName,
			data: 'this.Response = Response; this.Headers = Headers; this.counter = 0;'
		});
		
		// Define a handler function in the context
		platform.emit('eval_in_context', {
			ctxid: contextName,
			id: 'handler-' + contextName,
			data: 'this.handler = function(request) { this.counter++; return new Response(JSON.stringify({message: "Response from context", counter: this.counter, method: request.method}), {status: 200, headers: {"Content-Type": "application/json"}}); };'
		});
		
		// Register the context
		contexts[contextName] = {
			name: contextName,
			created: Date.now()
		};
		
		console.log("Context setup complete:", contextName);
		return true;
	}
	
	// Create a proxy handler for a context
	function createContextProxy(contextName) {
		// Create the context if it doesn't exist
		if (!contexts[contextName]) {
			setupContext(contextName);
		}
		
		// Define a global handler with the "context.handler" name format
		// This will be called by Go's ServeHTTPToHandler
		global[contextName + '.handler'] = async function(request) {
			console.log("Proxy handler called for context:", contextName);
			
			// Convert the Request to a simpler object for the context
			const reqData = {
				method: request.method,
				url: request.url,
				path: request.path || '',
				headers: {}
			};
			
			// Copy request headers
			for (const [key, value] of Object.entries(request.headers)) {
				reqData.headers[key] = value;
			}
			
			// Get request body if any
			try {
				reqData.body = await request.text();
			} catch (e) {
				console.log("Error reading request body:", e);
				reqData.body = '';
			}
			
			// Call the handler in the context and get the response
			return new Promise((resolve, reject) => {
				const execId = 'exec-' + Date.now();
				
				// One-time listener for the response
				const responseHandler = (event) => {
					if (event.action === 'response' && event.data && event.data.id === execId) {
						// Remove the listener once we get a response
						platform.removeListener('in', responseHandler);
						
						// Check for errors
						if (event.data.error) {
							reject(new Error(event.data.error));
							return;
						}
						
						// Process the result
						try {
							const result = event.data.res;
							
							// Create a Response object
							resolve(new Response(
								result.body || '',
								{
									status: result.status || 200,
									headers: result.headers || {}
								}
							));
						} catch (e) {
							reject(e);
						}
					}
				};
				
				// Listen for the response
				platform.on('in', responseHandler);
				
				// Safety timeout
				const timeout = setTimeout(() => {
					platform.removeListener('in', responseHandler);
					reject(new Error('Timeout waiting for context response'));
				}, 5000);
				
				// Build the code to execute in context by concatenating strings
				// instead of using template literals to avoid escaping issues
				const code = [
					"(async function() {",
					"  try {",
					"    // Make sure handler function exists",
					"    if (typeof this.handler !== 'function') {",
					"      throw new Error('Handler not defined in context');",
					"    }",
					"",
					"    // Create a simple request object",
					"    const req = {",
					"      method: '" + reqData.method.replace(/'/g, "\\'") + "',",
					"      url: '" + reqData.url.replace(/'/g, "\\'") + "',",
					"      path: '" + reqData.path.replace(/'/g, "\\'") + "',",
					"      headers: " + JSON.stringify(reqData.headers) + ",",
					"      body: '" + reqData.body.replace(/'/g, "\\'") + "',",
					"      text: function() { return Promise.resolve('" + reqData.body.replace(/'/g, "\\'") + "'); }",
					"    };",
					"",
					"    // Call the handler",
					"    console.log('Calling handler in context');",
					"    const response = await Promise.resolve(this.handler(req));",
					"",
					"    // Validate response",
					"    if (!response || typeof response !== 'object' || ", 
					"        typeof response.headers !== 'object' || ", 
					"        typeof response.status !== 'number') {",
					"      throw new Error('Handler must return a Response object');",
					"    }",
					"",
					"    // Extract response data",
					"    let body = '';",
					"    try {",
					"      body = await response.text();",
					"    } catch (e) {",
					"      console.log('Error reading response body:', e);",
					"    }",
					"",
					"    // Extract headers",
					"    const headers = {};",
					"    if (typeof response.headers.forEach === 'function') {",
					"      response.headers.forEach((v, k) => { headers[k] = v; });",
					"    } else if (typeof response.headers === 'object') {",
					"      Object.assign(headers, response.headers);",
					"    }",
					"",
					"    // Return structured response",
					"    return {",
					"      status: response.status,",
					"      headers: headers,",
					"      body: body",
					"    };",
					"  } catch (e) {",
					"    console.log('Error in context handler:', e);",
					"    throw e;",
					"  }",
					"})();"
				].join('\n');
				
				// Execute in the context
				platform.emit('eval_in_context', {
					ctxid: contextName,
					id: execId,
					data: code
				});
			});
		};
		
		console.log("Created proxy handler:", contextName + '.handler');
		return contextName + '.handler';
	}
	
	// Export the proxy creator function
	global.createContextProxy = createContextProxy;
	`

	// Evaluate the proxy setup code
	_, err = proc.Eval(context.Background(), proxySetupCode, nil)
	if err != nil {
		t.Fatalf("Failed to evaluate proxy setup: %v", err)
	}

	// Create a context proxy
	ctx := "testContext"
	_, err = proc.Eval(context.Background(), `createContextProxy('`+ctx+`')`, nil)
	if err != nil {
		t.Fatalf("Failed to create context proxy: %v", err)
	}

	// Test the context handler
	t.Run("Context Handler via Proxy", func(t *testing.T) {
		// Create a test HTTP request
		req := httptest.NewRequest(http.MethodGet, "http://localhost/context?param=value", nil)
		req.Header.Set("X-Context-Test", "TestValue")

		// Create a test response recorder
		w := httptest.NewRecorder()

		// Serve the HTTP request to our context handler via proxy
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
