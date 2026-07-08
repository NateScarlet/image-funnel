//go:build windows

package hook

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = modkernel32.NewProc("GenerateConsoleCtrlEvent")
)

func generateConsoleCtrlEvent(event uint32, pgid uint32) error {
	r1, _, err := procGenerateConsoleCtrlEvent.Call(uintptr(event), uintptr(pgid))
	if r1 == 0 {
		return err
	}
	return nil
}

const processQueryLimitedInformation = 0x1000

func newHookCmd(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       "cmd.exe /c " + command,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// 1. 友好中止：向进程组发送 CTRL_BREAK_EVENT 信号（类似 Ctrl+C）
		// 对应常数为 1
		_ = generateConsoleCtrlEvent(1, uint32(cmd.Process.Pid))

		// 2. 打开并持有进程句柄。
		// 只要我们不关闭这个句柄，Windows 就绝对不会回收并重新分配当前 PID，从而避免 1 秒后强杀时发生 PID 冲突。
		hProcess, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(cmd.Process.Pid))
		hasHandle := err == nil

		// 3. 异步等待 1 秒优雅退出期，若仍在运行则在后台强杀整个进程树
		pid := cmd.Process.Pid
		go func() {
			defer func() {
				if hasHandle {
					_ = syscall.CloseHandle(hProcess)
				}
			}()

			time.Sleep(1 * time.Second)

			// 检查进程是否仍在运行。若已经优雅退出，则不进行强杀，避免误杀重新分配了相同 PID 的无关新进程
			stillRunning := true
			if hasHandle {
				var exitCode uint32
				if err := syscall.GetExitCodeProcess(hProcess, &exitCode); err == nil {
					// 259 是 STILL_ACTIVE 常量
					if exitCode != 259 {
						stillRunning = false
					}
				}
			}

			if stillRunning {
				killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
				_ = killCmd.Run()
			}
		}()
		return nil
	}
	return cmd
}