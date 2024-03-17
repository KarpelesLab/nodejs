package nodejs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/KarpelesLab/pjson"
	"github.com/KarpelesLab/rndstr"
	"github.com/KarpelesLab/runutil"
)

type IpcFunc func(map[string]any) (any, error)

type Process struct {
	*exec.Cmd

	versions map[string]string
	in       io.WriteCloser
	out      io.ReadCloser
	ready    chan struct{}
	alive    chan struct{}
	ipc      map[string]IpcFunc
	chkpnt   map[string]chan map[string]any
	chkpntLk sync.RWMutex
	console  *bytes.Buffer
	ctx      context.Context
	cleanup  []func()
}

type message struct {
	Action string           `json:"action"`
	Data   pjson.RawMessage `json:"data"`
	Id     int64            `json:"id"`
}

const runArg = `(()=>{let i=process.stdin;let b="";let f=(d)=>{b+=d;if(b.slice(-1)=="\n"){i.off("data",f);(1,eval(b));}};i.on("data",f);})();`

func init() {
	// clear zombies that may remain from a previous running version (pre-update)
	runutil.Reap()
}

func startProcess(exe string) (*Process, error) {
	// start nodejs
	proc := &Process{
		Cmd: &exec.Cmd{
			Path:        exe,
			Args:        []string{exe, "--unhandled-rejections=strict", "--experimental-vm-modules", "-e", runArg},
			Env:         []string{"HOME=/", "NODE_ENV=production"},
			SysProcAttr: getSysProcAttr(),
		},
		ready:   make(chan struct{}),
		alive:   make(chan struct{}),
		chkpnt:  make(map[string]chan map[string]any),
		ipc:     make(map[string]IpcFunc),
		console: &bytes.Buffer{},
	}

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

	go proc.readThread()
	go proc.readStderr(stderr)

	err = proc.Start()
	if err != nil {
		// we're getting some of those errors but shouldn't:
		// 2021/12/11 00:34:09 [server] failed to spawn nodejs: fork/exec : no such file or directory
		return nil, fmt.Errorf("failed to start %s (%s): %w", exe, proc.Cmd.Path, err)
	}
	go proc.doWait()

	// run bootstrap.js
	proc.in.Write(append(append([]byte("(1,eval)("), bootstrapEnc...), ')', '\n'))

	// give it 2 seconds to be ready
	t := time.NewTimer(2 * time.Second)
	defer t.Stop()

	select {
	case <-proc.ready:
	case <-t.C:
		proc.Cmd.Process.Kill()
		return nil, ErrTimeout
	}

	return proc, nil
}

func (p *Process) doWait() {
	// wait for process to die, and perform cleanup
	p.Cmd.Wait()
	close(p.alive)
	for _, f := range p.cleanup {
		f()
	}
}

// Kill will kill the nodejs instance
func (p *Process) Kill() {
	p.Cmd.Process.Kill()
}

// Alive returns a channel that will be closed when the nodejs process ends
func (p *Process) Alive() <-chan struct{} {
	return p.alive
}

// Close closes the nodejs stdin pipe, causing the process to die of natual causes
func (p *Process) Close() error {
	// close stdin
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

// Run executes the provided code in the nodejs instance. options can contain things like filename.
// If filename ends in .mjs, the code will be run as a JS module
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

// Eval is similar to Run, but will return whatever the javascript code evaluated returned
func (p *Process) Eval(ctx context.Context, code string, opts map[string]any) (any, error) {
	if opts == nil {
		opts = map[string]any{}
	}
	id, ch := p.makeResponse()
	err := p.send(map[string]any{"action": "eval", "id": id, "data": code, "opts": opts})
	if err != nil {
		return nil, err
	}

	select {
	case res := <-ch:
		if v, ok := res["error"].(string); ok {
			return nil, errors.New(v)
		}
		if v, ok := res["res"]; ok {
			return v, nil
		}
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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
			if err != io.EOF {
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

// Checkpoint ensures the process is running and returns within the specified time
func (p *Process) Checkpoint(timeout time.Duration) error {
	str := rndstr.Simple(32, rndstr.Alnum)
	ch := p.makeHandle(str)
	p.send(map[string]any{"action": "response", "data": map[string]any{"id": str}})

	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case <-ch:
		return nil
	case <-t.C:
		// timeout error
		return ErrTimeout
	}
}

func (p *Process) makeResponse() (string, chan map[string]any) {
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
