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

// Factory represents a NodeJS instance factory that can create processes.
// It holds the path to the NodeJS executable and its version.
type Factory struct {
	nodePath string // Path to the NodeJS executable
	version  string // Detected NodeJS version
}

// New creates a new NodeJS factory instance.
// It attempts to locate the NodeJS executable in system paths or specific locations.
// On Unix systems, it first checks a predefined path, then falls back to PATH search.
// Returns an error if NodeJS cannot be found or fails initialization checks.
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

	// Fall back to finding node in system PATH
	p, err := exec.LookPath("node")
	if err != nil {
		return nil, err
	}
	factory.nodePath = p
	return factory, nil
}

// New creates a new NodeJS process with the default timeout of 5 seconds.
// It returns a Process that can be used to run JavaScript code.
func (factory *Factory) New() (*Process, error) {
	return startProcess(factory.nodePath, 5*time.Second)
}

// NewWithTimeout creates a new NodeJS process with a custom timeout.
// The timeout controls how long to wait for the NodeJS process to initialize.
func (factory *Factory) NewWithTimeout(timeout time.Duration) (*Process, error) {
	return startProcess(factory.nodePath, timeout)
}

// initialCheck verifies that the NodeJS installation is usable.
// It attempts to start a process, run a checkpoint, and retrieve the NodeJS version.
// Returns an error if any of these steps fail, indicating the NodeJS installation
// cannot be used.
func (factory *Factory) initialCheck() error {
	// Log the path to NodeJS being used
	slog.Debug(fmt.Sprintf("[nodejs] Using nodejs found at %s", factory.nodePath), "event", "nodejs:path")

	// Try to start a NodeJS process with a long timeout
	proc, err := factory.NewWithTimeout(5 * time.Minute)
	if err != nil {
		slog.Error("[nodejs] Nodejs cannot be used, giving up", "event", "nodejs:fail")
		return err
	}
	defer proc.Close()

	// Make sure the process is responsive
	proc.Checkpoint(30 * time.Second)

	// Get and log NodeJS version information
	factory.version = proc.GetVersion("node")
	slog.Debug(fmt.Sprintf("[nodejs] Confirmed nodejs running version %s, latency = %s", factory.version, proc.ping(5)), "event", "nodejs:version", "nodejs.version", factory.version)

	return nil
}
