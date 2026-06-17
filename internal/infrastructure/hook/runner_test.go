package hook

import (
	"context"
	"iter"
	"os"
	"path/filepath"
	"testing"
	"time"

	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type mockEventBus struct {
	subscribers []chan *shared.MetadataUpdatedEvent
}

func (m *mockEventBus) SubscribeMetadataUpdated(ctx context.Context) iter.Seq2[*shared.MetadataUpdatedEvent, error] {
	ch := make(chan *shared.MetadataUpdatedEvent, 10)
	m.subscribers = append(m.subscribers, ch)
	return func(yield func(*shared.MetadataUpdatedEvent, error) bool) {
		for {
			select {
			case ev := <-ch:
				if !yield(ev, nil) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

func (m *mockEventBus) Publish(ev *shared.MetadataUpdatedEvent) {
	for _, ch := range m.subscribers {
		ch <- ev
	}
}

func TestRunner_TOML_Parsing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	tomlContent := `
id = "comfyui-test"
name = "ComfyUI 测试"
description = "ComfyUI 测试说明"
command = "python test.py"

[on.post_update_image_metadata]
rating = [4, 5]
label = [""]

[on.image_dispatch]
`
	tomlPath := filepath.Join(hooksDir, "comfyui.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockEventBus{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, "", nil)
	defer runner.Close()

	hooks, err := runner.List(context.Background())
	assert.NoError(t, err)
	assert.Len(t, hooks, 1)

	configs, err := runner.LoadHooks()
	assert.NoError(t, err)
	assert.Len(t, configs, 1)

	h := hooks[0]
	assert.Equal(t, "hk:comfyui-test", h.ID().String())
	assert.Equal(t, "ComfyUI 测试", h.Name())
	assert.True(t, h.CanDispatchByImage())
}

func TestRunner_FilteringAndDebounce(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-exec-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "flag_dir")
	cmdStr := "mkdir " + flagFile

	tomlContent := `
id = "exec-test"
name = "执行测试"
command = '` + cmdStr + `'

[on.post_update_image_metadata]
rating = [4]
label = [""]
`
	tomlPath := filepath.Join(hooksDir, "exec.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockEventBus{}
	logger := zap.NewExample()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, "", nil)
	runner.debouncer.duration = 10 * time.Millisecond // 10ms 防抖
	defer runner.Close()

	// 1. 自动触发：测试发送不符合过滤条件的事件（3星）
	ebus.Publish(&shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(tempDir, "a.png"),
		Rating:    3,
		Label:     "",
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	})

	time.Sleep(50 * time.Millisecond) // 等待防抖
	_, err = os.Stat(flagFile)
	assert.True(t, os.IsNotExist(err), "3星评级不应触发钩子运行")

	// 2. 自动触发：测试发送符合过滤条件的事件（4星）且标签为 ""
	ebus.Publish(&shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(tempDir, "a.png"),
		Rating:    4,
		Label:     "",
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	})

	assert.True(t, waitFlagFile(flagFile, 500*time.Millisecond), "4星评级应触发钩子成功运行")
	time.Sleep(50 * time.Millisecond) // 等待进程执行和日志打印完全收尾

	// 清理标志文件
	err = os.RemoveAll(flagFile)
	assert.NoError(t, err)

	// 3. 自动触发：测试发送符合 4星但标签为 Red（不满足 label = [""] 的过滤）
	ebus.Publish(&shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(tempDir, "a.png"),
		Rating:    4,
		Label:     "Red",
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	})

	time.Sleep(50 * time.Millisecond)
	_, err = os.Stat(flagFile)
	assert.True(t, os.IsNotExist(err), "由于只匹配空标签，标签为 Red 的图片应该不匹配")
}

func waitFlagFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestRunner_Env_Injection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-env-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	tomlContent := `
id = "env-test"
name = "环境变量测试"
command = "python test_env.py"

[env]
TEST_VAR_ONE = "val1"
TEST_VAR_TWO = "val2"
`
	tomlPath := filepath.Join(hooksDir, "env.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockEventBus{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, "", nil)
	defer runner.Close()

	configs, err := runner.LoadHooks()
	assert.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, "val1", configs[0].Env["TEST_VAR_ONE"])
	assert.Equal(t, "val2", configs[0].Env["TEST_VAR_TWO"])
}

func TestRunner_Trigger_SyncAndError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-trigger-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	// 1. 测试成功的外部钩子命令
	successToml := `
id = "success-test"
name = "成功测试"
command = "echo hello"

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "success.toml"), []byte(successToml), 0644)
	assert.NoError(t, err)

	// 2. 测试失败的外部钩子命令（会返回错误，并且 stderr 中有报错）
	failToml := `
id = "fail-test"
name = "失败测试"
command = "echo test_error_out >&2 && exit 42"

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "fail.toml"), []byte(failToml), 0644)
	assert.NoError(t, err)

	ebus := &mockEventBus{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, "", nil)
	defer runner.Close()

	// 触发成功钩子，应该同步等待并返回 nil
	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{"a.png"}, scalar.ToID("hk:success-test"), "image_dispatch")
	assert.NoError(t, err)

	// 触发失败钩子，应该同步等待并返回错误，且错误信息中包含退出码和 stderr 输出
	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{"a.png"}, scalar.ToID("hk:fail-test"), "image_dispatch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test_error_out")
	assert.Contains(t, err.Error(), "exit status 42")
}
