package nodejs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/KarpelesLab/rndstr"
)

// Context represents a JavaScript context within a NodeJS process.
// It provides sandboxed JavaScript execution that is isolated from other contexts.
type Context struct {
	proc   *Process
	id     string
	closed bool
	mu     sync.Mutex
}

// NewContext creates a new JavaScript execution context in the specified NodeJS process.
// It returns a Context interface that can be used to execute JavaScript code in isolation.
func (p *Process) NewContext() (*Context, error) {
	// Generate a random ID for this context
	ctxID := rndstr.Simple(16, rndstr.Alnum)

	// Send create_context command to NodeJS
	id, ch := p.MakeResponse()
	err := p.send(map[string]any{
		"action": "create_context",
		"id":     id,
		"ctxid":  ctxID,
	})
	if err != nil {
		return nil, err
	}

	// Wait for response from NodeJS
	select {
	case res := <-ch:
		if errMsg, ok := res["error"].(string); ok {
			return nil, errors.New(errMsg)
		}
		// Context created successfully
	case <-p.alive:
		return nil, ErrDeadProcess
	}

	// Create and return the Context object
	return &Context{
		proc: p,
		id:   ctxID,
	}, nil
}

// Close frees the JavaScript context in the NodeJS process.
// After closing, the context cannot be used for execution.
func (c *Context) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil // Already closed
	}

	// Mark as closed
	c.closed = true

	// Send free_context command to NodeJS
	id, ch := c.proc.MakeResponse()
	err := c.proc.send(map[string]any{
		"action": "free_context",
		"id":     id,
		"ctxid":  c.id,
	})
	if err != nil {
		return err
	}

	// Wait for response from NodeJS
	select {
	case res := <-ch:
		if errMsg, ok := res["error"].(string); ok {
			return errors.New(errMsg)
		}
		// Context freed successfully
	case <-c.proc.alive:
		return ErrDeadProcess
	}

	return nil
}

// EvalChannel executes JavaScript code within this context and returns a channel
// that will receive the evaluation result when available.
// Unlike Eval, this method doesn't wait for the result and returns immediately.
// The caller is responsible for monitoring the channel for results.
func (c *Context) EvalChannel(code string, opts map[string]any) (chan map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("context is closed")
	}

	if opts == nil {
		opts = map[string]any{}
	}

	// Get a response channel
	id, ch := c.proc.MakeResponse()

	// Send eval_in_context command to NodeJS
	err := c.proc.send(map[string]any{
		"action": "eval_in_context",
		"id":     id,
		"ctxid":  c.id,
		"data":   code,
		"opts":   opts,
	})
	if err != nil {
		return nil, err
	}

	return ch, nil
}

// ServeHTTPToHandler serves an HTTP request to a JavaScript handler function within this context.
// It converts the HTTP request to a JavaScript Fetch API compatible Request, calls the specified handler,
// and streams the Response back to the Go ResponseWriter.
//
// The handlerFunc parameter is the name of the JavaScript handler function to call within this context.
// The handler function should accept a Request object and return a Response object.
//
// Example JavaScript handler:
//
//	// In the context:
//	handler = function(request) {
//	  return new Response(JSON.stringify({hello: 'world'}), {
//	    status: 200,
//	    headers: {'Content-Type': 'application/json'}
//	  });
//	}
func (c *Context) ServeHTTPToHandler(handlerFunc string, w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		http.Error(w, "Context is closed", http.StatusInternalServerError)
		return
	}

	// Create HTTP handler options with this context's ID
	options := HTTPHandlerOptions{
		Context: c.id,
	}

	// Use the ServeHTTPWithOptions method
	c.proc.ServeHTTPWithOptions(handlerFunc, options, w, r)
}

// ID returns the unique identifier for this JavaScript context.
// This ID can be used when calling ServeHTTPWithOptions to specify
// the context in which to execute the handler function.
func (c *Context) ID() string {
	return c.id
}

// Eval executes JavaScript code within this context and returns the result.
// It takes a Go context for timeout/cancellation and options for execution.
func (c *Context) Eval(ctx context.Context, code string, opts map[string]any) (any, error) {
	// Get a channel that will receive the evaluation result
	ch, err := c.EvalChannel(code, opts)
	if err != nil {
		return nil, err
	}

	// Wait for one of: result, process termination, or context cancellation
	select {
	case res := <-ch:
		// Check if the JavaScript code resulted in an error
		if v, ok := res["error"].(string); ok {
			return nil, errors.New(v)
		}
		// Extract and return the result value
		if v, ok := res["res"]; ok {
			return v, nil
		}
		return nil, nil
	case <-c.proc.alive:
		// Process died while waiting for result
		return nil, ErrDeadProcess
	case <-ctx.Done():
		// Context was cancelled or timed out
		return nil, ctx.Err()
	}
}
