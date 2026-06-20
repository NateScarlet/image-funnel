//go:build !windows

package hook

import (
	"context"
	"os/exec"
)

func newHookCmd(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}