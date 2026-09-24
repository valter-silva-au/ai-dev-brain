//go:build windows

package cli

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachProcess configures the command to run as a detached background process.
// DETACHED_PROCESS gives the child no console; CREATE_NEW_PROCESS_GROUP ensures
// the daemon survives after the parent shell exits and does not receive Ctrl+C
// from the parent console.
func detachProcess(cmd *exec.Cmd) {
	const (
		detachedProcess       = 0x00000008
		createNewProcessGroup = 0x00000200
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
}

// stopProcess asks the process to terminate. Windows has no SIGTERM equivalent
// for arbitrary processes, so we fall back to a hard kill via TerminateProcess.
// The daemon does not get a chance to run shutdown hooks.
func stopProcess(p *os.Process) error {
	return p.Kill()
}

// processAlive reports whether the given process is still running.
// On Windows, os.FindProcess returns a usable handle whether the process
// exists or not, so we query OpenProcess + GetExitCodeProcess.
func processAlive(p *os.Process) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return code == stillActive
}
