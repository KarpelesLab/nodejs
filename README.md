[![GoDoc](https://godoc.org/github.com/KarpelesLab/nodejs?status.svg)](https://godoc.org/github.com/KarpelesLab/nodejs)

# nodejs tools for Go

A Go library that provides seamless integration with NodeJS, allowing Go programs to run JavaScript code through managed NodeJS instances.

## Features

- **NodeJS Factory**: Automatic detection and initialization of NodeJS
- **Process Management**: Start, monitor, and gracefully terminate NodeJS processes
- **Process Pooling**: Efficiently manage multiple NodeJS instances with auto-scaling
- **JavaScript Execution**: Run JS code with precise control and error handling
- **IPC Communication**: Bidirectional communication between Go and JavaScript
- **Health Monitoring**: Built-in process health checks and responsiveness verification
- **Isolated Contexts**: Create isolated JavaScript execution contexts within a process
- **HTTP Integration**: Serve HTTP requests directly to JavaScript handlers

## Installation

```bash
go get github.com/KarpelesLab/nodejs
```

## Basic Usage

### Creating a NodeJS Factory

The factory is responsible for finding and initializing NodeJS:

```go
factory, err := nodejs.New()
if err != nil {
    // handle error: nodejs could not be found or didn't run
}
```

### Direct Process Management

Create and manage individual NodeJS processes:

```go
// Create a new process with default timeout (5 seconds)
process, err := factory.New()
if err != nil {
    // handle error
}
defer process.Close()

// Run JavaScript without waiting for result
process.Run("console.log('Hello from NodeJS')", nil)

// Evaluate JavaScript and get result
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
result, err := process.Eval(ctx, "2 + 2", nil)
if err != nil {
    // handle error
}
fmt.Println("Result:", result) // Output: Result: 4
```

### Process Pool

For applications that need to execute JavaScript frequently, using a pool improves performance:

```go
// Create a pool with auto-sized queue (defaults to NumCPU)
pool := factory.NewPool(0, 0)

// Get a process from the pool (waits if none available)
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
process, err := pool.Take(ctx)
if err != nil {
    // handle error: timeout or context cancelled
}
defer process.Close() // Returns process to pool

// Execute JavaScript using the pooled process
result, err := process.Eval(ctx, "(() => { return 'Hello from pooled NodeJS'; })()", nil)
if err != nil {
    // handle error
}
fmt.Println(result)
```

### Bidirectional IPC

Register Go functions that can be called from JavaScript:

```go
process.SetIPC("greet", func(params map[string]any) (any, error) {
    name, _ := params["name"].(string)
    if name == "" {
        name = "Guest"
    }
    return map[string]any{
        "message": fmt.Sprintf("Hello, %s!", name),
    }, nil
})

// In JavaScript, call the Go function:
process.Run(`
    async function testIPC() {
        const result = await ipc('greet', { name: 'World' });
        console.log(result.message); // Outputs: Hello, World!
    }
    testIPC();
`, nil)
```

### Advanced Features

#### Isolated JavaScript Contexts

JavaScript contexts provide isolated execution environments within a single NodeJS process:

```go
// Create a NodeJS process
proc, err := factory.New()
if err != nil {
    // handle error
}
defer proc.Close()

// Create an isolated JavaScript context
jsCtx, err := proc.NewContext()
if err != nil {
    // handle error
}
defer jsCtx.Close()

// Execute JavaScript in the context
ctx := context.Background()
result, err := jsCtx.Eval(ctx, "var counter = 1; counter++;", nil)
if err != nil {
    // handle error
}
fmt.Println("Result:", result) // Output: Result: 2

// Create a second isolated context
jsCtx2, err := proc.NewContext()
if err != nil {
    // handle error
}
defer jsCtx2.Close()

// The second context has its own independent environment
result2, err := jsCtx2.Eval(ctx, "typeof counter", nil)
if err != nil {
    // handle error
}
fmt.Println("Result2:", result2) // Output: Result2: undefined
```

#### HTTP Integration with JavaScript Handlers

You can serve HTTP requests directly to JavaScript handlers:

```go
// Create a JavaScript context
jsCtx, err := proc.NewContext()
if err != nil {
    // handle error
}
defer jsCtx.Close()

// Define a JavaScript HTTP handler
_, err = jsCtx.Eval(context.Background(), `
// Define a handler function
this.apiHandler = function(request) {
    // Create response body
    const body = JSON.stringify({
        message: "Hello, World!",
        method: request.method,
        path: request.url
    });
    
    // Return a Response object (Fetch API compatible)
    return new Response(body, {
        status: 200,
        headers: {
            "Content-Type": "application/json"
        }
    });
};
`, nil)

// Create an HTTP handler that delegates to the JavaScript context
http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
    jsCtx.ServeHTTPToHandler("apiHandler", w, r)
})

// Start the HTTP server
http.ListenAndServe(":8080", nil)
```

#### Custom Timeouts

```go
// Create process with custom initialization timeout
process, err := factory.NewWithTimeout(10 * time.Second)
if err != nil {
    // handle error
}
```

#### Health Checks

```go
// Verify process is responsive
if err := process.Checkpoint(2 * time.Second); err != nil {
    // Process is not responding
    process.Kill() // Force terminate if needed
}
```

#### Module Support

```go
// Execute ES modules
process.Run(`
    import { createHash } from 'crypto';
    const hash = createHash('sha256').update('hello').digest('hex');
    console.log(hash);
`, map[string]any{"filename": "example.mjs"})
```

## License

See the LICENSE file for details.