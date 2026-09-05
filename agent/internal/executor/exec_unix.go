//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
)

// prepareCmd puts the child in its own process group and makes context
// cancellation SIGKILL the whole group, so orphaned grandchildren cannot keep
// the stdout/stderr pipes open past the deadline.
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return nil
	}
}
