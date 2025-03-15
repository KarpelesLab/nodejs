package nodejs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/KarpelesLab/pjson"
	"github.com/KarpelesLab/rndstr"
	"github.com/KarpelesLab/runutil"
)

// IpcFunc is a function type for handling IPC calls from JavaScript to Go.
// It receives a map of parameters and returns a response or error.
type IpcFunc func(map[string]any) (any, error)

// Process represents a running NodeJS process instance.
// It wraps exec.Cmd and provides communication channels with the NodeJS process.
type Process struct {
	*exec.Cmd // Embedded command to run NodeJS

	versions map[string]string              // Version information from NodeJS
	in       io.WriteCloser                 // Stdin pipe to the NodeJS process
	out      io.ReadCloser                  // Stdout pipe from the NodeJS process
	ready    chan struct{}                  // Channel closed when NodeJS is ready
	alive    chan struct{}                  // Channel closed when NodeJS process exits
	ipc      map[string]IpcFunc             // Map of registered IPC handlers
	chkpnt   map[string]chan map[string]any // Map of checkpoint handlers
	chkpntLk sync.RWMutex                   // Lock for checkpoint map
	console  *bytes.Buffer                  // Buffer for console output
	ctx      context.Context                // Context for cancellation
	cleanup  []func()                       // Cleanup functions to run on exit
}

// message represents a JSON message exchanged with the NodeJS process.
// These messages are used for communication between Go and NodeJS.
type message struct {
	Action string           `json:"action"` // Action to perform (e.g., "eval", "console.log")
	Data   pjson.RawMessage `json:"data"`   // Message payload as raw JSON
	Id     int64            `json:"id"`     // Message ID for tracking responses
}

const runArg = `(()=>{let i=process.stdin;let b="";let f=(d)=>{b+=d;if(b.slice(-1)=="\n"){i.off("data",f);(1,eval(b));}};i.on("data",f);})();`

// init initializes the NodeJS package by reaping any zombie processes.
// This helps clean up orphaned NodeJS processes from previous runs.
func init() {
	// clear zombies that may remain from a previous running version (pre-update)
	runutil.Reap()
}

// startProcess creates and starts a new NodeJS process with the given executable path and timeout.
// It sets up the communication channels, starts monitoring goroutines, and initializes the NodeJS
// environment with bootstrap.js.
func startProcess(exe string, timeout time.Duration) (*Process, error) {
	// Initialize the process with NodeJS config and environment
	proc := &Process{
		Cmd: &exec.Cmd{
			Path:        exe,
			Args:        []string{exe, "--unhandled-rejections=strict", "--experimental-vm-modules", "-e", runArg},
			Env:         []string{"HOME=/", "NODE_ENV=production"},
			SysProcAttr: getSysProcAttr(),
		},
		ready:   make(chan struct{}), // Channel closed when NodeJS is ready
		alive:   make(chan struct{}), // Channel closed when process exits
		chkpnt:  make(map[string]chan map[string]any),
		ipc:     make(map[string]IpcFunc),
		console: &bytes.Buffer{}, // Buffer for console output
	}

	// Set up stdin and stdout pipes for communication
	var err error
	proc.in, err = proc.StdinPipe()
	if err != nil {
		return nil, err
	}
	proc.out, err = proc.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := proc.StderrPipe()

	// Start goroutines to handle input/output
	go proc.readThread()
	go proc.readStderr(stderr)

	// Start the NodeJS process
	err = proc.Start()
	if err != nil {
		// we're getting some of those errors but shouldn't:
		// 2021/12/11 00:34:09 [server] failed to spawn nodejs: fork/exec : no such file or directory
		return nil, fmt.Errorf("failed to start %s (%s): %w", exe, proc.Cmd.Path, err)
	}
	go proc.doWait()

	// Initialize NodeJS environment with bootstrap.js code
	proc.in.Write(append(append([]byte("(1,eval)("), bootstrapEnc...), ')', '\n'))

	// Wait for the process to be ready or time out
	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case <-proc.ready:
		// Process is ready
	case <-t.C:
		// Timeout reached, kill the process
		proc.Cmd.Process.Kill()
		return nil, ErrTimeout
	}

	return proc, nil
}

// doWait waits for the NodeJS process to exit and performs cleanup.
// It runs as a goroutine and executes all registered cleanup functions
// when the process terminates.
func (p *Process) doWait() {
	// Wait for process to die
	p.Cmd.Wait()
	// Signal that the process is no longer alive
	close(p.alive)
	// Run all cleanup handlers
	for _, f := range p.cleanup {
		f()
	}
}

// Kill forcibly terminates the NodeJS process immediately.
func (p *Process) Kill() {
	p.Cmd.Process.Kill()
}

// Alive returns a channel that will be closed when the NodeJS process ends.
// This can be used to monitor the process state and react to termination.
func (p *Process) Alive() <-chan struct{} {
	return p.alive
}

// Close gracefully shuts down the NodeJS process by closing its stdin pipe.
// This allows the process to perform cleanup operations before exiting.
func (p *Process) Close() error {
	// Close stdin, which will cause the NodeJS process to exit cleanly
	return p.in.Close()
}

// Console returns the console output so far for the nodejs process
func (p *Process) Console() []byte {
	return p.console.Bytes()
}

// Log appends a message to the nodejs console directly so it can be retrieved with Console()
func (p *Process) Log(msg string, args ...any) {
	fmt.Fprintf(p.console, msg+"\n", args...)
}

// Run executes the provided JavaScript code in the NodeJS instance.
// The options map can contain metadata like filename, which determines how the code is executed.
// If the filename ends in .mjs, the code will be executed as an ES module.
// This method does not return any results from the execution.
func (p *Process) Run(code string, opts map[string]any) {
	if opts == nil {
		opts = map[string]any{}
	}
	p.send(map[string]any{"action": "eval", "data": code, "opts": opts})
}

// SetIPC adds an IPC that can be called from nodejs
func (p *Process) SetIPC(name string, f IpcFunc) {
	p.ipc[name] = f
}

// Eval executes JavaScript code and returns the result of the evaluation.
// Unlike Run, this method waits for the code to complete execution and returns the result.
// It takes a context for timeout/cancellation control.
// If the JavaScript code throws an error, it will be returned as a Go error.
func (p *Process) Eval(ctx context.Context, code string, opts map[string]any) (any, error) {
	// Get a channel that will receive the evaluation result
	ch, err := p.EvalChannel(code, opts)
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
	case <-p.alive:
		// Process died while waiting for result
		return nil, ErrDeadProcess
	case <-ctx.Done():
		// Context was cancelled or timed out
		return nil, ctx.Err()
	}
}

// EvalChannel will execute the provided code and return a channel that can be used to read the
// response once it is made available
func (p *Process) EvalChannel(code string, opts map[string]any) (chan map[string]any, error) {
	if opts == nil {
		opts = map[string]any{}
	}
	id, ch := p.MakeResponse()
	err := p.send(map[string]any{"action": "eval", "id": id, "data": code, "opts": opts})
	if err != nil {
		return nil, err
	}

	return ch, nil
}

// Set sets a variable in the javascript global scope of this instance
func (p *Process) Set(v string, val any) {
	p.send(map[string]any{"action": "set", "key": v, "data": val})
}

func (p *Process) readThread() {
	b := bufio.NewReader(p.out)

	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] readThread crashed: %s", e), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:readthread_fail", "category", "go.panic")
			p.Cmd.Process.Kill()
		}
	}()

	for {
		lin, err := b.ReadBytes('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] read failed: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:read_fail")
			}
			return
		}
		var data message
		err = pjson.Unmarshal(lin, &data)
		if err != nil {
			slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] parse failed: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:json_fail")
			continue
		}

		p.handleAction(&data)
	}
}

func (p *Process) readStderr(stderr io.Reader) {
	io.Copy(p.console, stderr)
}

func (p *Process) handleAction(msg *message) {
	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] handleAction crashed: %s", e), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:handleact_fail", "category", "go.panic")
		}
	}()

	switch msg.Action {
	case "console.log":
		var str string
		err := pjson.Unmarshal(msg.Data, &str)
		if err != nil {
			slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] decode error: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:json_fail")
			str = string(msg.Data)
		}
		slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] console.log: %s", str), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:console.log")
		fmt.Fprintf(p.console, "%s\n", str)
	case "ready":
		close(p.ready)
	case "versions":
		err := pjson.Unmarshal(msg.Data, &p.versions)
		if err != nil {
			slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] decode error: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:json_fail")
		}
	case "response":
		var res map[string]any
		err := pjson.Unmarshal(msg.Data, &res)
		if err != nil {
			slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] decode error: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:json_fail")
			return
		}
		p.handleResponse(res)
	case "ipc.req":
		go p.handleIpc(msg.Id, msg.Data)
	default:
		slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] unhandled action %s", msg.Action), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:unhandled_action")
	}
}

// Checkpoint verifies that the NodeJS process is responsive by sending a message and waiting for a response.
// It returns an error if the process does not respond within the specified timeout duration.
// This is useful for health checks and ensuring the NodeJS process hasn't frozen.
func (p *Process) Checkpoint(timeout time.Duration) error {
	// Generate a random ID for this checkpoint
	str := rndstr.Simple(32, rndstr.Alnum)
	// Create a channel to receive the response
	ch := p.makeHandle(str)
	// Send a message that should trigger an immediate response
	p.send(map[string]any{"action": "response", "data": map[string]any{"id": str}})

	// Set timeout timer
	t := time.NewTimer(timeout)
	defer t.Stop()

	// Wait for either response or timeout
	select {
	case <-ch:
		// Got response, process is responsive
		return nil
	case <-t.C:
		// Timeout reached, process is unresponsive
		return ErrTimeout
	}
}

// MakeResponse returns a handler id and a channel that will see data appended to it if triggered
// from the javascript side with a response event to that id. This can be useful for asynchronisous
// events.
func (p *Process) MakeResponse() (string, chan map[string]any) {
	str := rndstr.Simple(32, rndstr.Alnum)
	ch := p.makeHandle(str)
	return str, ch
}

func (p *Process) handleResponse(res map[string]any) {
	id, ok := res["id"].(string)
	if !ok {
		return
	}
	v := p.takeHandle(id)
	if v == nil {
		return
	}
	v <- res
}

func (p *Process) takeHandle(str string) chan map[string]any {
	p.chkpntLk.Lock()
	defer p.chkpntLk.Unlock()

	v, ok := p.chkpnt[str]
	if !ok {
		return nil
	}
	delete(p.chkpnt, str)
	return v
}

func (p *Process) makeHandle(str string) chan map[string]any {
	p.chkpntLk.Lock()
	defer p.chkpntLk.Unlock()

	ch := make(chan map[string]any, 1)
	p.chkpnt[str] = ch

	return ch
}

func (p *Process) handleIpc(id int64, req pjson.RawMessage) {
	var param map[string]any
	err := pjson.Unmarshal(req, &param)
	if err != nil {
		slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] failed to decode RPC: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:rpc_decode_fail")
		p.ipcFailure(id, err.Error())
	}

	ipc, ok := param["ipc"].(string)
	if !ok {
		p.ipcFailure(id, "ipc value missing")
	}

	switch ipc {
	case "test":
		p.ipcSuccess(id, map[string]any{"ok": true})
	case "version":
		p.ipcSuccess(id, map[string]any{"version": runtime.Version()})
	default:
		if f, ok := p.ipc[ipc]; ok {
			res, err := f(param)
			if err != nil {
				err = p.ipcFailure(id, err.Error())
				if err != nil {
					slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] failed to send IPC failure response: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:ipc_send_fail")
				}
			} else {
				err = p.ipcSuccess(id, res)
				if err != nil {
					slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] failed to send IPC response: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:ip_send_fail")
				}
			}
			return
		}
		p.ipcFailure(id, fmt.Sprintf("unknown ipc %s", ipc))
	}
}

func (p *Process) ipcSuccess(id int64, data any) error {
	return p.ipcResult(id, "ipc.success", data)
}

func (p *Process) ipcFailure(id int64, data any) error {
	return p.ipcResult(id, "ipc.failure", data)
}

func (p *Process) ipcResult(id int64, code string, data any) error {
	return p.send(map[string]any{"action": code, "id": id, "data": data})
}

func (p *Process) send(obj any) error {
	buf, err := pjson.Marshal(obj)
	if err != nil {
		return err
	}

	_, err = p.in.Write(append(buf, '\n'))
	if err != nil {
		slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] ERROR in write: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:write_error")
	}
	return err
}

func (p *Process) GetVersion(what string) string {
	v, ok := p.versions[what]
	if !ok {
		return ""
	}
	return v
}

func (p *Process) ping(cnt int) time.Duration {
	var res time.Duration
	success := 0
	for i := 0; i < cnt; i++ {
		start := time.Now()
		if err := p.Checkpoint(1 * time.Second); err != nil {
			slog.ErrorContext(p.getContext(), fmt.Sprintf("[nodejs] failed ping request: %s", err), "platform-fe.module", "nodejs", "event", "platform-fe:nodejs:ping_error")
			continue
		}
		success += 1
		res += time.Now().Sub(start)
	}
	if success == 0 {
		return 0
	}
	return res / time.Duration(success)
}

func (p *Process) SetContext(ctx context.Context) {
	p.ctx = ctx
}

func (p *Process) getContext() context.Context {
	if c := p.ctx; c != nil {
		return c
	}
	return context.Background()
}
