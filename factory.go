package nodejs

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Factory struct {
	nodePath string
	version  string
}

func New() (*Factory, error) {
	factory := &Factory{}

	if runtime.GOOS != "windows" {
		// unix → check if we have azusa nodejs available
		p := "/pkg/main/net-libs.nodejs.core/bin/node"
		if _, err := os.Stat(p); err == nil {
			p, err = filepath.EvalSymlinks(p)
			if err == nil {
				factory.nodePath = p
				if err = factory.initialCheck(); err != nil {
					return nil, err
				}
				return factory, nil
			}
		}
	}

	p, err := exec.LookPath("node")
	if err != nil {
		return nil, err
	}
	factory.nodePath = p
	return factory, nil
}

func (factory *Factory) New() (*Process, error) {
	return startProcess(factory.nodePath, 5*time.Second)
}

func (factory *Factory) NewWithTimeout(timeout time.Duration) (*Process, error) {
	return startProcess(factory.nodePath, timeout)
}

func (factory *Factory) initialCheck() error {
	// check if usable nodejs
	slog.Debug(fmt.Sprintf("[nodejs] Using nodejs found at %s", factory.nodePath), "event", "nodejs:path")

	proc, err := factory.NewWithTimeout(5 * time.Minute)
	if err != nil {
		slog.Error("[nodejs] Nodejs cannot be used, giving up", "event", "nodejs:fail")
		return err
	}
	defer proc.Close()

	proc.Checkpoint(30 * time.Second)

	factory.version = proc.GetVersion("node")
	slog.Debug(fmt.Sprintf("[nodejs] Confirmed nodejs running version %s, latency = %s", factory.version, proc.ping(5)), "event", "nodejs:version", "nodejs.version", factory.version)

	return nil
}
