package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domimage "main/internal/domain/image"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type mockMetadataUpdatedSub struct {
	subscribers []chan *shared.MetadataUpdatedEvent
}

func (m *mockMetadataUpdatedSub) Subscribe(ctx context.Context) iter.Seq2[*shared.MetadataUpdatedEvent, error] {
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

func (m *mockMetadataUpdatedSub) Publish(ctx context.Context, ev *shared.MetadataUpdatedEvent, opts ...pubsub.PublishOption) error {
	for _, ch := range m.subscribers {
		ch <- ev
	}
	return nil
}

type mockFileChangedSub struct{}

func (m *mockFileChangedSub) Subscribe(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error] {
	return func(yield func(*shared.FileChangedEvent, error) bool) {}
}

func (m *mockFileChangedSub) Publish(ctx context.Context, ev *shared.FileChangedEvent, opts ...pubsub.PublishOption) error {
	return nil
}

type mockImageRepository struct {
	images []*domimage.Image
}

func (m *mockImageRepository) Get(ctx context.Context, relPath string) (*domimage.Image, error) {
	for _, img := range m.images {
		if img.RelPath() == relPath {
			return img, nil
		}
	}
	return nil, os.ErrNotExist
}

func (m *mockImageRepository) Find(ctx context.Context, relPath string) iter.Seq2[*domimage.Image, error] {
	return func(yield func(*domimage.Image, error) bool) {
		for _, img := range m.images {
			if !yield(img, nil) {
				return
			}
		}
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

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, nil, nil, nil)
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

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewExample()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, nil, nil, nil)
	runner.debouncer.duration = 10 * time.Millisecond // 10ms 防抖
	defer runner.Close()

	// 1. 自动触发：测试发送不符合过滤条件的事件（3星）
	ebus.Publish(context.Background(), &shared.MetadataUpdatedEvent{
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
	ebus.Publish(context.Background(), &shared.MetadataUpdatedEvent{
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
	ebus.Publish(context.Background(), &shared.MetadataUpdatedEvent{
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

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, nil, nil, nil)
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

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, nil, nil, nil)
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

func TestRunner_NoDirective_PostUpdateNote(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "no-dir-update-note-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "update_flag")
	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `echo "$IMAGE_FUNNEL_NOTE_PATHS" > ` + flagFile
	} else {
		cmdStr = `echo %IMAGE_FUNNEL_NOTE_PATHS% > ` + flagFile
	}

	tomlContent := `
id = "no-dir-update"
name = "无指令更新测试"
command = '` + cmdStr + `'

[on.post_update_note]
`
	err = os.WriteFile(filepath.Join(hooksDir, "no-dir-update.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()

	imgID := scalar.ToID("img:1")
	img := domimage.New(imgID, "test.png", "test.png", scalar.ToID("dir:1"), 100, time.Now(), nil, 0, 0)
	imgRepo := &mockImageRepository{images: []*domimage.Image{img}}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, imgRepo, nil, nil)
	defer runner.Close()

	noteRelPath := "test.png.md"
	noteAbsPath := filepath.Join(tempDir, noteRelPath)
	err = os.WriteFile(noteAbsPath, []byte("Just a normal note with no slash commands"), 0644)
	assert.NoError(t, err)

	runner.handleFileChanged(&shared.FileChangedEvent{
		DirectoryID: scalar.ToID("dir:1"),
		RelPath:     noteRelPath,
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	})

	assert.True(t, waitFlagFile(flagFile, 1500*time.Millisecond), "应该无指令直接触发并创建 flag 文件")
	contentBytes, err := os.ReadFile(flagFile)
	assert.NoError(t, err)

	result := strings.TrimSpace(string(contentBytes))
	var mPaths []string
	err = json.Unmarshal([]byte(result), &mPaths)
	assert.NoError(t, err)
	assert.Len(t, mPaths, 1)
	assert.Equal(t, noteAbsPath, mPaths[0], "环境变量 IMAGE_FUNNEL_NOTE_PATHS 应该注入正确")
}

func TestRunner_NoDirective_PostCommitSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "no-dir-commit-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagPure := filepath.Join(tempDir, "pure_flag")
	flagScan := filepath.Join(tempDir, "scan_flag")

	var cmdPure, cmdScan string
	if filepath.Separator == '/' {
		cmdPure = `echo "pure" > ` + flagPure
		cmdScan = `echo "$IMAGE_FUNNEL_NOTE_PATHS" > ` + flagScan
	} else {
		cmdPure = `echo pure > ` + flagPure
		cmdScan = `echo %IMAGE_FUNNEL_NOTE_PATHS% > ` + flagScan
	}

	tomlPure := `
id = "pure-commit"
name = "纯会话提交测试"
command = '` + cmdPure + `'

[on.post_commit_session]
`
	err = os.WriteFile(filepath.Join(hooksDir, "pure.toml"), []byte(tomlPure), 0644)
	assert.NoError(t, err)

	tomlScan := `
id = "scan-commit"
name = "笔记扫描提交测试"
command = '` + cmdScan + `'

[on.post_commit_session.note_scan]
`
	err = os.WriteFile(filepath.Join(hooksDir, "scan.toml"), []byte(tomlScan), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()

	imgID := scalar.ToID("img:1")
	img := domimage.New(imgID, "test.png", "test.png", scalar.ToID("dir:1"), 100, time.Now(), nil, 0, 0)
	imgRepo := &mockImageRepository{images: []*domimage.Image{img}}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, imgRepo, nil, nil)
	defer runner.Close()

	err = os.WriteFile(filepath.Join(tempDir, "test.png.md"), []byte("Hello world note"), 0644)
	assert.NoError(t, err)

	err = runner.OnCommitSession(context.Background(), scalar.ToID("dir:1"), "")
	assert.NoError(t, err)

	assert.True(t, waitFlagFile(flagPure, 1500*time.Millisecond), "纯提交钩子应成功执行")
	assert.True(t, waitFlagFile(flagScan, 1500*time.Millisecond), "无指令扫描提交钩子应成功执行")

	contentBytes, err := os.ReadFile(flagScan)
	assert.NoError(t, err)
	result := strings.TrimSpace(string(contentBytes))
	var mPaths []string
	err = json.Unmarshal([]byte(result), &mPaths)
	assert.NoError(t, err)
	assert.Len(t, mPaths, 1)
	assert.Equal(t, filepath.Join(tempDir, "test.png.md"), mPaths[0], "无指令笔记扫描钩子的 NotePaths 应正确携带")
}

func TestSplitArgs_Simple(t *testing.T) {
	assert.Equal(t, []string{"arg1", "arg2", "arg3"}, splitArgs("arg1 arg2 arg3"))
}

func TestSplitArgs_SingleArg(t *testing.T) {
	assert.Equal(t, []string{"hello"}, splitArgs("hello"))
}

func TestSplitArgs_Empty(t *testing.T) {
	assert.Empty(t, splitArgs(""))
}

func TestSplitArgs_OnlyWhitespace(t *testing.T) {
	assert.Empty(t, splitArgs("   \t  "))
}

func TestSplitArgs_QuotedSimple(t *testing.T) {
	assert.Equal(t, []string{"hello world"}, splitArgs(`"hello world"`))
}

func TestSplitArgs_QuotedMixed(t *testing.T) {
	assert.Equal(t, []string{"arg1", "hello world", "arg3"}, splitArgs(`arg1 "hello world" arg3`))
}

func TestSplitArgs_MultipleQuoted(t *testing.T) {
	assert.Equal(t, []string{"first", "second"}, splitArgs(`"first" "second"`))
}

func TestSplitArgs_QuotedAtStart(t *testing.T) {
	assert.Equal(t, []string{"hello world", "arg2"}, splitArgs(`"hello world" arg2`))
}

func TestSplitArgs_QuotedAtEnd(t *testing.T) {
	assert.Equal(t, []string{"arg1", "hello world"}, splitArgs(`arg1 "hello world"`))
}

func TestSplitArgs_NestedQuotes(t *testing.T) {
	// 引号内的内容原样保留，不支持转义
	assert.Equal(t, []string{`it's`}, splitArgs(`"it's"`))
}

func TestSplitArgs_TabSeparated(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, splitArgs("a\tb\tc"))
}

func TestSplitArgs_QuotedWithTabInside(t *testing.T) {
	// 引号内的制表符原样保留
	assert.Equal(t, []string{"a\tb"}, splitArgs("\"a\tb\""))
}

func TestRunner_ExecuteNoteDirectives_PartialFailures(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-note-partial-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	// 1. 成功钩子定义，指令为 "fork"
	forkToml := `
id = "fork-test"
name = "fork-test"
command = "echo fork success"

[directive]
name = "fork"
on_success_action = "REMOVE"
on_fail_action = "KEEP"

[on.post_update_note]
`
	err = os.WriteFile(filepath.Join(hooksDir, "fork.toml"), []byte(forkToml), 0644)
	assert.NoError(t, err)

	// 2. 失败钩子定义，指令为 "comfyui"
	comfyuiToml := `
id = "comfyui-test"
name = "comfyui-test"
command = "echo fail_output >&2 && exit 1"

[directive]
name = "comfyui"
on_success_action = "REMOVE"
on_fail_action = "KEEP"

[on.post_update_note]
`
	err = os.WriteFile(filepath.Join(hooksDir, "comfyui.toml"), []byte(comfyuiToml), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, &mockImageRepository{}, nil, nil)
	defer runner.Close()

	// 准备笔记文件
	noteRelPath := "test_note.md"
	noteAbsPath := filepath.Join(tempDir, noteRelPath)
	initialContent := `Some text.
/fork a
/comfyui b
Other text.`
	err = os.WriteFile(noteAbsPath, []byte(initialContent), 0644)
	assert.NoError(t, err)

	// 运行指令处理
	ok, err := runner.executeNoteDirectives(context.Background(), scalar.ToID("dir:1"), "", noteRelPath, initialContent, "post_update_note", scalar.ID{})
	assert.NoError(t, err)
	assert.True(t, ok)

	// 检查同步清理后的内容：
	// fork 成功应被 REMOVE 变成空行（正则替换后可能留空，或者去掉）
	// comfyui 失败应被 KEEP 保留
	contentBytes, err := os.ReadFile(noteAbsPath)
	assert.NoError(t, err)
	content := string(contentBytes)

	// 期望 fork 行被移除，comfyui 行被保留，并且 run-id 标签已擦除
	assert.NotContains(t, content, "/fork")
	assert.Contains(t, content, "/comfyui b")
	assert.NotContains(t, content, "hook-run-id")

	// 3. 测试迟到清理的分支：
	// 写回一个带未清理指令和 run-id 的文本，模拟迟到写入
	runID := "run_test_12345"
	lateContent := fmt.Sprintf("---\nhook-run-id: %s\n---\nSome text.\n/fork a\n/comfyui b\nOther text.", runID)
	err = os.WriteFile(noteAbsPath, []byte(lateContent), 0644)
	assert.NoError(t, err)

	// 在 runner.activeTasks 中预设这个 runID
	runner.muTasks.Lock()
	runner.activeTasks[runID] = &activeTask{
		phase: phaseAfter3,
		paths: map[string]struct{}{noteAbsPath: {}},
		failedDirectives: map[string]bool{
			"fork":    false, // 成功
			"comfyui": true,  // 失败
		},
	}
	runner.muTasks.Unlock()

	// 获取 hookMap
	hooks, err := runner.LoadHooks()
	assert.NoError(t, err)
	hookMap := make(map[string]HookConfig)
	for _, h := range hooks {
		if h.Directive != nil && h.Directive.Name != "" {
			hookMap[h.Directive.Name] = h
		}
	}

	// 触发 late cleanup
	runner.postProcessNoteDirectives(context.Background(), noteAbsPath, runID, "post_update_note", hookMap, map[string]bool{
		"fork":    false,
		"comfyui": true,
	})

	// 再次检查迟到清理后的文件内容
	contentBytes, err = os.ReadFile(noteAbsPath)
	assert.NoError(t, err)
	content = string(contentBytes)

	assert.NotContains(t, content, "/fork")
	assert.Contains(t, content, "/comfyui b")
	assert.NotContains(t, content, "hook-run-id")
}

func TestRunner_LoadHooks_StrictParsing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-strict-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	// 写一个包含未知 Key（拼写错误）的 TOML 配置文件
	invalidToml := `
id = "strict-test"
name = "strict-test"
command = "echo success"
unknown_key_here = "oops"

[directive]
name = "strict"
`
	err = os.WriteFile(filepath.Join(hooksDir, "invalid.toml"), []byte(invalidToml), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", nil, &mockImageRepository{}, nil, nil)
	defer runner.Close()

	configs, err := runner.LoadHooks()
	assert.NoError(t, err)

	// configs 不应该包含 strict-test，因为解析由于未知字段而报错被跳过
	for _, c := range configs {
		assert.NotEqual(t, "strict-test", c.ID)
	}
}
