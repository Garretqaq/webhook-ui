//go:build !windows

package services

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts the child in its own process group.
//
// Without it a script's own children are unreachable: killing the shell leaves
// whatever it spawned running, still holding the output pipes open. The group
// gives the whole tree one handle to signal.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree signals the whole group the child leads. A negative pid
// addresses the group rather than the single process, which is the difference
// between reaping a script's children and orphaning them.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// The group may already be gone, or setpgid may not have taken effect
		// if the process died during Start. Fall back to the process itself.
		cmd.Process.Kill()
	}
}
