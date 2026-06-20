//go:build windows

package hook

import (
	"context"
	"os/exec"
	"syscall"
)

func newHookCmd(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: "cmd.exe /c " + command,
	}
	return cmd
}