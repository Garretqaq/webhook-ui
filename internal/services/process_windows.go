//go:build windows

package services

import "os/exec"

// setProcessGroup is a no-op on Windows: syscall.SysProcAttr has no Setpgid
// field there, and process trees are managed through job objects instead.
// Running the server itself on Windows is not supported — this file exists so
// the release workflow's windows cross-compile keeps building.
func setProcessGroup(cmd *exec.Cmd) {}

// killProcessTree reaches only the direct child on Windows; descendants it
// spawned survive. See setProcessGroup for why that is acceptable here.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	cmd.Process.Kill()
}
