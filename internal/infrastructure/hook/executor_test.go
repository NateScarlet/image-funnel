package hook

import (
	"context"
	"testing"

	"main/internal/domain/directory"
	"main/internal/domain/relpath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHookExecutor_CancelPropagatesToProcess 普通单次执行路径的同类取消测试：
// 注入进程替身并断言替身收到取消，不再探测操作系统进程表。
func TestHookExecutor_CancelPropagatesToProcess(t *testing.T) {
	process := newBlockingProcessRunner()
	deps := newIsolatedDeps(t, nil, process, nil)

	done := make(chan hookRunResult, 1)
	go func() {
		done <- deps.exec.Execute(hookExecutionTask{
			HookID:      "cancel-test",
			Command:     "uv run slow.py",
			TriggerName: "image_dispatch",
			Dir:         directory.FromRepository(relpath.Path{}),
		})
	}()
	process.awaitCall(t)

	// 关闭执行器即取消其生命周期，正在运行的钩子进程随之被终止
	deps.exec.Close()
	process.canceled(t)

	assert.ErrorIs(t, (<-done).Err, context.Canceled, "执行器关闭后等待方应以取消错误收尾")
}

// TestHookExecutor_ResolvesActionFromConfig 脚本未写 IMAGE_FUNNEL_ACTION 时，
// 执行结果的操作回退到配置声明的指令成功/失败动作
func TestHookExecutor_ResolvesActionFromConfig(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"resolve.toml": noteHookTOML("resolve-test", shellPrint("echo done"), "t", "KEEP", "COMMENT_OUT", "[on.image_dispatch]\n"),
		},
	})

	hooks, err := env.config.Load()
	require.NoError(t, err)
	require.Len(t, hooks, 1)

	dir := directory.FromRepository(relpath.Path{})

	success := env.runner.exec.Execute(newHookTask(hooks[0], "image_dispatch", nil, "", dir))
	assert.NoError(t, success.Err)
	assert.Equal(t, "KEEP", success.Action, "脚本未覆盖操作时应回退到 on_success_action")

	failing := hooks[0]
	failing.Command = shellPrint("exit 1")
	failed := env.runner.exec.Execute(newHookTask(failing, "image_dispatch", nil, "", dir))
	require.Error(t, failed.Err)
	assert.Equal(t, "COMMENT_OUT", failed.Action, "脚本失败时应回退到 on_fail_action")

	// 脚本写出的 IMAGE_FUNNEL_ACTION 优先于配置声明
	overridden := hooks[0]
	overridden.Command = keepWriterCmd("REMOVE")
	over := env.runner.exec.Execute(newHookTask(overridden, "image_dispatch", nil, "", dir))
	assert.NoError(t, over.Err)
	assert.Equal(t, "REMOVE", over.Action, "脚本覆盖的操作应优先于配置声明")
}
