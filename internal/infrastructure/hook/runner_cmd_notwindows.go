//go:build !windows

package hook

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

func newHookCmd(ctx context.Context, argv []string) *exec.Cmd {
	// 不经过 shell（sh -c）直接以 argv 启动，避免指令参数中的 >、|、& 等
	// 元字符被 shell 解释。
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// 1. 友好中止：向整个进程组发送 SIGINT 信号（类似 Ctrl+C）
		pgid := -cmd.Process.Pid
		_ = syscall.Kill(pgid, syscall.SIGINT)

		// 2. 异步等待 1 秒优雅退出期，若仍在运行则在后台强杀整个进程组
		go func() {
			time.Sleep(1 * time.Second)
			// 发送 0 信号用于检测进程组是否存在。若返回错误说明进程组已自然退场。
			// 仅当进程组仍然存活时，执行 SIGKILL 强杀，从而彻底杜绝误杀重分配了相同 PID 的无关新进程组的隐患。
			if err := syscall.Kill(pgid, 0); err == nil {
				_ = syscall.Kill(pgid, syscall.SIGKILL)
			}
		}()
		return nil
	}
	return cmd
}