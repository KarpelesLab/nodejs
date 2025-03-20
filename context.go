package nodejs

import (
	"context"
	"errors"
	"fmt"
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

// Eval executes JavaScript code within this context and returns the result.
// It takes a Go context for timeout/cancellation and options for execution.
func (c *Context) Eval(ctx context.Context, code string, opts map[string]any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("context is closed")
	}

	if opts == nil {
		opts = map[string]any{}
	}

	// Send eval_in_context command to NodeJS
	id, ch := c.proc.MakeResponse()
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
