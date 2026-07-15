//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// detachProcess configures the command to run as a detached background process.
// On Unix this calls setsid(2) so the child becomes a new session leader,
// disconnected from the parent's controlling terminal.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// stopProcess asks the process to terminate. On Unix this sends SIGTERM, giving
// the daemon a chance to shut down cleanly.
func stopProcess(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// processAlive reports whether the given process is still running.
// On Unix, os.FindProcess always succeeds, so we probe with Signal(0).
func processAlive(p *os.Process) bool {
	return p.Signal(syscall.Signal(0)) == nil
}
