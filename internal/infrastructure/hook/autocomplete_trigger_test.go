package hook

import (
	"context"
	"testing"

	domhook "main/internal/domain/hook"
	"main/internal/scalar"

	"github.com/stretchr/testify/assert"
)

// TestAutocomplete_SingleShot_Cancel 单次执行通道的取消：注入进程原语替身并断言它收到取消，
// 不再 spawn 真实进程、也不用 tasklist / ps 探测操作系统进程表。
func TestAutocomplete_SingleShot_Cancel(t *testing.T) {
	process := newBlockingProcessRunner()
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"autocomplete.toml": hookTOML("auto-cancel-test", "自动补全取消测试", "echo fallback",
				"[directive]\nname = \"test\"\n\n[directive.autocomplete]\ncommand = '''uv run serve.py'''\n"),
		},
		Seam: func(sm *seams) { sm.Process = process },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type outcome struct {
		suggestions []*domhook.AutocompleteSuggestion
		err         error
	}
	ch := make(chan outcome, 1)
	go func() {
		s, err := env.runner.Autocomplete(ctx, scalar.ToID("hk:auto-cancel-test"), "", "/add ", "po")
		ch <- outcome{s, err}
	}()

	// 等脚本确实被启动后再取消，避免与「尚未启动」竞态
	process.awaitCall(t)
	cancel()

	got := <-ch
	assert.ErrorIs(t, got.err, context.Canceled, "取消后应立即返回 context.Canceled")
	assert.Nil(t, got.suggestions)

	// 进程替身观察到取消，即取消被透传给了进程原语
	process.canceled(t)
}
