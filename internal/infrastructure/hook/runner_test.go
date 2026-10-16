package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestRunner_TOML_Parsing 配置来源逐次读盘解析并映射为领域实体
func TestRunner_TOML_Parsing(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"comfyui.toml": hookTOML("comfyui-test", "ComfyUI 测试", "python test.py",
				"[on.post_update_image_metadata]\nrating = [4, 5]\nlabel = [\"\"]\n\n[on.image_dispatch]\nhotkeys = [\"F5\", \"ctrl+shift+k\"]\n"),
		},
	})

	hooks, err := env.runner.List(context.Background())
	require.NoError(t, err)
	require.Len(t, hooks, 1)

	h := hooks[0]
	assert.Equal(t, "hk:comfyui-test", h.ID().String())
	assert.Equal(t, "ComfyUI 测试", h.Name())
	assert.True(t, h.CanDispatchByImage())
	assert.Equal(t, []string{"F5", "ctrl+shift+k"}, h.ImageDispatchHotkeys())
}

func TestRunner_TOML_Parsing_ImageDispatchWithoutHotkeys(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"no-hotkey.toml": hookTOML("no-hotkey", "无快捷键", "python test.py", "[on.image_dispatch]\n"),
		},
	})

	hooks, err := env.runner.List(context.Background())
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Empty(t, hooks[0].ImageDispatchHotkeys())
}

// TestRunner_ConfigSource_ReloadsOnDemand 验证配置来源不缓存：改 toml 即时生效
func TestRunner_ConfigSource_ReloadsOnDemand(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"reload-test.toml": hookTOML("reload-test", "reload-test", "python a.py", "[on.image_dispatch]\n"),
		},
	})

	hooks, err := env.config.Load()
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Equal(t, "python a.py", hooks[0].Command)

	env.writeHook("reload-test.toml", hookTOML("reload-test", "reload-test", "python b.py", "[on.image_dispatch]\n"))

	hooks, err = env.config.Load()
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Equal(t, "python b.py", hooks[0].Command, "配置来源应逐次读盘，改 toml 即时生效")
}

// TestRunner_FilteringAndDebounce 元数据事件按筛选条件命中后防抖合批执行
func TestRunner_FilteringAndDebounce(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Seam: func(sm *seams) { sm.DebounceDuration = 10 * time.Millisecond },
	})
	flagFile := filepath.Join(env.rootDir, "flag_dir")
	env.writeHook("exec.toml", hookTOML("exec-test", "执行测试", shellRedirect("echo 1", flagFile),
		"[on.post_update_image_metadata]\nrating = [4]\nlabel = [\"\"]\n"))

	metadataSub := env.runner.deps.MetadataUpdatedSub.(*mockMetadataUpdatedSub)

	// 1. 自动触发：测试发送不符合过滤条件的事件（3星）
	metadataSub.Publish(context.Background(), metadataEvent(env.rootDir, 3, ""))

	time.Sleep(50 * time.Millisecond) // 等待防抖
	assertNoFile(t, flagFile, "3星评级不应触发钩子运行")

	// 2. 自动触发：测试发送符合过滤条件的事件（4星）且标签为 ""
	metadataSub.Publish(context.Background(), metadataEvent(env.rootDir, 4, ""))

	assert.True(t, waitFlagFile(flagFile, 15*time.Second), "4星评级应触发钩子成功运行")
	time.Sleep(50 * time.Millisecond) // 等待进程执行和日志打印完全收尾

	require.NoError(t, os.RemoveAll(flagFile))

	// 3. 自动触发：测试发送符合 4星但标签为 Red（不满足 label = [""] 的过滤）
	metadataSub.Publish(context.Background(), metadataEvent(env.rootDir, 4, "Red"))

	time.Sleep(50 * time.Millisecond)
	assertNoFile(t, flagFile, "由于只匹配空标签，标签为 Red 的图片应该不匹配")
}

// metadataEvent 构造一次元数据更新事件
func metadataEvent(rootDir string, rating int, label string) *shared.MetadataUpdatedEvent {
	return &shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(rootDir, "a.png"),
		Rating:    rating,
		Label:     label,
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	}
}

func TestRunner_Env_Injection(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"env.toml": "id = \"env-test\"\nname = \"环境变量测试\"\ncommand = \"python test_env.py\"\n\n[env]\nTEST_VAR_ONE = \"val1\"\nTEST_VAR_TWO = \"val2\"\n",
		},
	})

	configs, err := env.config.Load()
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, "val1", configs[0].Env["TEST_VAR_ONE"])
	assert.Equal(t, "val2", configs[0].Env["TEST_VAR_TWO"])
}

// TestRunner_GraphqlURLEnv_Injection 基础环境变量注入本机 GraphQL 地址与数据目录
func TestRunner_GraphqlURLEnv_Injection(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{GraphQLURL: "http://127.0.0.1:8000/graphql"})

	got, err := env.runner.exec.env.Build(context.Background(), envRequest{
		HookID:      "test-id",
		HookName:    "test-name",
		TriggerName: "test-trigger",
	})
	require.NoError(t, err)

	assert.Equal(t, "http://127.0.0.1:8000/graphql", envValue(got, "IMAGE_FUNNEL_GRAPHQL_URL"))
	assert.Equal(t, env.rootDir, envValue(got, "IMAGE_FUNNEL_DATA_DIR"))
}

// envValue 从环境变量列表中取出指定键的值
func envValue(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

func TestRunner_Trigger_SyncAndError(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"success.toml": hookTOML("success-test", "成功测试", shellPrint("echo hello"), "[on.image_dispatch]\n"),
			"fail.toml":    hookTOML("fail-test", "失败测试", shellPrint("echo test_error_out >&2 && exit 42"), "[on.image_dispatch]\n"),
		},
	})

	// 使用绝对路径，使得目录信息可推导
	absPath := filepath.Join(env.rootDir, "a.png")

	// 触发成功钩子，应该同步等待并返回 nil
	err := env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:success-test"), "image_dispatch")
	assert.NoError(t, err)

	// 触发失败钩子，应该同步等待并返回错误，且错误信息中包含退出码和 stderr 输出
	err = env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:fail-test"), "image_dispatch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test_error_out")
	assert.Contains(t, err.Error(), "exit status 42")
}

func TestRunner_DirectoryEnvVars_Injection(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Seam: func(sm *seams) { sm.DebounceDuration = 10 * time.Millisecond },
	})
	flagFile := filepath.Join(env.rootDir, "dir_env_flag")

	// 使用 | 作为分隔符避免与目录 ID 中的 : 冲突
	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `sh -c 'echo "$IMAGE_FUNNEL_DIRECTORY_ID|$IMAGE_FUNNEL_DIRECTORY_REL_PATH" > ` + flagFile + `'`
	} else {
		cmdStr = `cmd /c "echo %IMAGE_FUNNEL_DIRECTORY_ID%^|%IMAGE_FUNNEL_DIRECTORY_REL_PATH% > ` + flagFile + `"`
	}
	env.writeHook("direnv.toml", hookTOML("direnv-test", "目录环境变量测试", cmdStr,
		"[on.post_update_image_metadata]\nrating = [4]\nlabel = [\"\"]\n"))
	expectedDir := env.dirRepo.dirs[relpath.Path{}]

	metadataSub := env.runner.deps.MetadataUpdatedSub.(*mockMetadataUpdatedSub)
	metadataSub.Publish(context.Background(), metadataEvent(env.rootDir, 4, ""))

	require.True(t, waitFlagFile(flagFile, 500*time.Millisecond), "应触发钩子并写入 flag 文件")

	parts := strings.SplitN(strings.TrimSpace(waitFlagContent(t, flagFile, 500*time.Millisecond)), "|", 2)
	require.Len(t, parts, 2, "输出应为 DIRECTORY_ID|DIRECTORY_REL_PATH 格式")
	assert.Equal(t, expectedDir.ID().String(), parts[0], "DIRECTORY_ID 应与目录仓库返回的 ID 一致")
}

func TestRunner_Trigger_DirectoryEnvVars(t *testing.T) {
	subRel := testutil.Rel(t, "subdir")
	env := newTestHooks(t, testHooksOptions{
		Dirs: map[relpath.Path]*directory.Directory{subRel: directory.FromRepository(subRel)},
	})
	flagFile := filepath.Join(env.rootDir, "trigger_dir_env_flag")

	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `sh -c 'echo "$IMAGE_FUNNEL_DIRECTORY_ID|$IMAGE_FUNNEL_DIRECTORY_REL_PATH" > ` + flagFile + `'`
	} else {
		cmdStr = `cmd /c "echo %IMAGE_FUNNEL_DIRECTORY_ID%^|%IMAGE_FUNNEL_DIRECTORY_REL_PATH% > ` + flagFile + `"`
	}
	env.writeHook("trigger-direnv.toml", hookTOML("trigger-direnv-test", "触发目录环境变量测试", cmdStr, "[on.image_dispatch]\n"))
	expectedDir := env.dirRepo.dirs[subRel]

	// 创建一个位于子目录中的文件，确保目录信息可推导
	subDir := filepath.Join(env.rootDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	absPath := filepath.Join(subDir, "test.png")

	require.NoError(t, env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:trigger-direnv-test"), "image_dispatch"))
	require.True(t, waitFlagFile(flagFile, 500*time.Millisecond), "Trigger 应成功执行钩子并写入 flag 文件")

	parts := strings.SplitN(strings.TrimSpace(waitFlagContent(t, flagFile, 500*time.Millisecond)), "|", 2)
	require.Len(t, parts, 2, "输出应为 DIRECTORY_ID|DIRECTORY_REL_PATH 格式")
	assert.Equal(t, expectedDir.ID().String(), parts[0], "DIRECTORY_ID 应与目录仓库返回的 ID 一致")
	assert.Equal(t, "subdir", parts[1], "DIRECTORY_REL_PATH 应为 subdir")
}

func TestRunner_Trigger_DirectoryResolveFailure(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Dirs: map[relpath.Path]*directory.Directory{},
	})
	flagFile := filepath.Join(env.rootDir, "should_not_exist")
	env.writeHook("dir-fail.toml", hookTOML("dir-fail-test", "目录解析失败测试", shellRedirect("echo should_not_run", flagFile), "[on.image_dispatch]\n"))

	absPath := filepath.Join(env.rootDir, "unknown_subdir", "a.png")
	err := env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:dir-fail-test"), "image_dispatch")
	require.Error(t, err, "目录解析失败时应返回错误")
	assert.Contains(t, err.Error(), "failed to resolve directory")

	assertNoFile(t, flagFile, "目录解析失败时不应执行钩子")
}

func TestRunner_NoDirective_PostUpdateNote(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{ImageRelPaths: []string{"test.png"}})
	flagFile := filepath.Join(env.rootDir, "update_flag")
	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `sh -c 'echo "$IMAGE_FUNNEL_NOTE_PATHS" > ` + flagFile + `'`
	} else {
		cmdStr = `cmd /c "echo %IMAGE_FUNNEL_NOTE_PATHS% > ` + flagFile + `"`
	}
	env.writeHook("no-dir-update.toml", hookTOML("no-dir-update", "无指令更新测试", cmdStr, "[on.post_update_note]\n"))

	noteRelPath := testutil.RelIn(t, env.rootDir, "test.png.md")
	noteAbsPath := noteRelPath.Abs(env.rootDir)
	require.NoError(t, os.WriteFile(noteAbsPath, []byte("Just a normal note with no slash commands"), 0644))

	require.NoError(t, env.runner.noteUpdate.HandleFileChanged(&directory.FileChangedEvent{
		DirectoryID: scalar.ToID("dir:1"),
		RelPath:     noteRelPath,
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	}))

	require.True(t, waitFlagFile(flagFile, 1500*time.Millisecond), "应该无指令直接触发并创建 flag 文件")

	var mPaths []string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(waitFlagContent(t, flagFile, 500*time.Millisecond))), &mPaths))
	require.Len(t, mPaths, 1)
	assert.Equal(t, noteAbsPath, mPaths[0], "环境变量 IMAGE_FUNNEL_NOTE_PATHS 应该注入正确")
}

func TestRunner_NoDirective_PostCommitSession(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{ImageRelPaths: []string{"test.png"}})
	flagPure := filepath.Join(env.rootDir, "pure_flag")
	flagScan := filepath.Join(env.rootDir, "scan_flag")

	var cmdPure, cmdScan string
	if filepath.Separator == '/' {
		cmdPure = shellRedirect("echo pure", flagPure)
		cmdScan = `sh -c 'echo "$IMAGE_FUNNEL_NOTE_PATHS" > ` + flagScan + `'`
	} else {
		cmdPure = shellRedirect("echo pure", flagPure)
		cmdScan = `cmd /c "echo %IMAGE_FUNNEL_NOTE_PATHS% > ` + flagScan + `"`
	}
	env.writeHook("pure.toml", hookTOML("pure-commit", "纯会话提交测试", cmdPure, "[on.post_commit_session]\n"))
	env.writeHook("scan.toml", hookTOML("scan-commit", "笔记扫描提交测试", cmdScan, "[on.post_commit_session.note_scan]\n"))

	require.NoError(t, os.WriteFile(filepath.Join(env.rootDir, "test.png.md"), []byte("Hello world note"), 0644))
	require.NoError(t, env.runner.OnCommitSession(context.Background(), relpath.Path{}))

	require.True(t, waitFlagFile(flagPure, 1500*time.Millisecond), "纯提交钩子应成功执行")
	require.True(t, waitFlagFile(flagScan, 1500*time.Millisecond), "无指令扫描提交钩子应成功执行")

	var mPaths []string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(waitFlagContent(t, flagScan, 500*time.Millisecond))), &mPaths))
	require.Len(t, mPaths, 1)
	assert.Equal(t, filepath.Join(env.rootDir, "test.png.md"), mPaths[0], "无指令笔记扫描钩子的 NotePaths 应正确携带")
}

// TestRunner_PostCommitSession_DirectoryID 验证 post_commit_session 纯提交钩子
// 在没有图片路径和笔记路径时，IMAGE_FUNNEL_DIRECTORY_ID 仍能正确注入
func TestRunner_PostCommitSession_DirectoryID(t *testing.T) {
	subRel := testutil.Rel(t, "subdir")
	expectedDir := directory.FromRepository(subRel)
	env := newTestHooks(t, testHooksOptions{
		Dirs: map[relpath.Path]*directory.Directory{subRel: expectedDir},
	})
	flagFile := filepath.Join(env.rootDir, "dir_id_flag")

	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `sh -c 'echo "$IMAGE_FUNNEL_DIRECTORY_ID" > ` + flagFile + `'`
	} else {
		cmdStr = `cmd /c "echo %IMAGE_FUNNEL_DIRECTORY_ID% > ` + flagFile + `"`
	}
	env.writeHook("dirid.toml", hookTOML("post-commit-dir-id", "提交时目录ID注入测试", cmdStr, "[on.post_commit_session]\n"))

	require.NoError(t, env.runner.OnCommitSession(context.Background(), subRel))
	require.True(t, waitFlagFile(flagFile, 1500*time.Millisecond), "纯提交钩子应成功执行并写入 flag 文件")

	// 验证回退机制：即使没有图片路径或笔记路径，DIRECTORY_ID 也能正确注入
	assert.Equal(t, expectedDir.ID().String(), strings.TrimSpace(waitFlagContent(t, flagFile, 500*time.Millisecond)),
		"post_commit_session 纯提交钩子应正确注入 IMAGE_FUNNEL_DIRECTORY_ID（回退到调用者传入的目录信息）")
}

func TestSplitArgs_Simple(t *testing.T) {
	assert.Equal(t, []string{"arg1", "arg2", "arg3"}, mustSplitArgs(t, "arg1 arg2 arg3"))
}

func TestSplitArgs_SingleArg(t *testing.T) {
	assert.Equal(t, []string{"hello"}, mustSplitArgs(t, "hello"))
}

func TestSplitArgs_Empty(t *testing.T) {
	assert.Empty(t, mustSplitArgs(t, ""))
}

func TestSplitArgs_OnlyWhitespace(t *testing.T) {
	assert.Empty(t, mustSplitArgs(t, "   \t  "))
}

func TestSplitArgs_QuotedSimple(t *testing.T) {
	assert.Equal(t, []string{"hello world"}, mustSplitArgs(t, `"hello world"`))
}

func TestSplitArgs_QuotedMixed(t *testing.T) {
	assert.Equal(t, []string{"arg1", "hello world", "arg3"}, mustSplitArgs(t, `arg1 "hello world" arg3`))
}

func TestSplitArgs_MultipleQuoted(t *testing.T) {
	assert.Equal(t, []string{"first", "second"}, mustSplitArgs(t, `"first" "second"`))
}

func TestSplitArgs_QuotedAtStart(t *testing.T) {
	assert.Equal(t, []string{"hello world", "arg2"}, mustSplitArgs(t, `"hello world" arg2`))
}

func TestSplitArgs_QuotedAtEnd(t *testing.T) {
	assert.Equal(t, []string{"arg1", "hello world"}, mustSplitArgs(t, `arg1 "hello world"`))
}

func TestSplitArgs_NestedQuotes(t *testing.T) {
	// 引号内的内容原样保留，不支持转义
	assert.Equal(t, []string{`it's`}, mustSplitArgs(t, `"it's"`))
}

func TestSplitArgs_TabSeparated(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, mustSplitArgs(t, "a\tb\tc"))
}

func TestSplitArgs_QuotedWithTabInside(t *testing.T) {
	// 引号内的制表符原样保留
	assert.Equal(t, []string{"a\tb"}, mustSplitArgs(t, "\"a\tb\""))
}

// mustSplitArgs 断言 splitArgs 成功并返回结果
func mustSplitArgs(t *testing.T, s string) []string {
	t.Helper()
	args, err := splitArgs(s)
	assert.NoError(t, err)
	return args
}

// TestNoteRewriter_PartialFailures 指令部分失败时按各指令自身的成败动作擦除，
// 迟到的文件写入走按指令名配对的后置擦除
func TestNoteRewriter_PartialFailures(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{})
	env.writeHook("fork.toml", noteHookTOML("fork-test", shellPrint("echo fork success"), "fork", "REMOVE", "KEEP", "[on.post_update_note]\n"))
	env.writeHook("comfyui.toml", noteHookTOML("comfyui-test", shellPrint("echo fail_output >&2 && exit 1"), "comfyui", "REMOVE", "KEEP", "[on.post_update_note]\n"))

	noteRelPath := testutil.RelIn(t, env.rootDir, "test_note.md")
	noteAbsPath := noteRelPath.Abs(env.rootDir)
	initialContent := "Some text.\n/fork a\n/comfyui b\nOther text."
	require.NoError(t, os.WriteFile(noteAbsPath, []byte(initialContent), 0644))

	rewriter := env.runner.rewrite
	ok, err := rewriter.Rewrite(context.Background(), noteRewriteRequest{
		Dir:         directory.FromRepository(relpath.Path{}),
		NoteRelPath: noteRelPath,
		Content:     initialContent,
		TriggerName: "post_update_note",
	})
	require.Error(t, err)
	assert.True(t, ok)

	// fork 成功应被 REMOVE，comfyui 失败应被 KEEP，且 run-id 标签已擦除
	content := readFile(t, noteAbsPath)
	assert.NotContains(t, content, "/fork")
	assert.Contains(t, content, "/comfyui b")
	assert.NotContains(t, content, "hook-run-id")

	// 测试迟到清理的分支：写回一个带未清理指令和 run-id 的文本，模拟迟到写入
	runID := "run_test_12345"
	lateContent := fmt.Sprintf("---\nhook-run-id: %s\n---\nSome text.\n/fork a\n/comfyui b\nOther text.", runID)
	require.NoError(t, os.WriteFile(noteAbsPath, []byte(lateContent), 0644))

	// 迟到路径按指令身份取回结果：fork 成功被 REMOVE，comfyui 失败被 KEEP
	require.NoError(t, rewriter.CleanupLate(noteAbsPath, runID, map[directiveIdentity]directiveResult{
		{line: "/fork a", ordinal: 0}:    {action: "REMOVE"},
		{line: "/comfyui b", ordinal: 0}: {action: "KEEP"},
	}))

	content = readFile(t, noteAbsPath)
	assert.NotContains(t, content, "/fork")
	assert.Contains(t, content, "/comfyui b")
	assert.NotContains(t, content, "hook-run-id")
}

func TestIsFileNotFoundError(t *testing.T) {
	// 直接包装 os.ErrNotExist 应被检测到
	assert.True(t, isFileNotFoundError(os.ErrNotExist))
	assert.True(t, isFileNotFoundError(fmt.Errorf("wrapped: %w", os.ErrNotExist)))
	assert.True(t, isFileNotFoundError(fmt.Errorf("nested: %w", fmt.Errorf("inner: %w", os.ErrNotExist))))

	// 非文件系统错误不应被检测到
	assert.False(t, isFileNotFoundError(fmt.Errorf("some other error")))
	assert.False(t, isFileNotFoundError(fmt.Errorf("exit status 1")))

	// exec.ExitError 不应被检测到（脚本内部错误，非 cmd.Start() 失败）
	assert.False(t, isFileNotFoundError(&exec.ExitError{ProcessState: &os.ProcessState{}}))
}

// TestNoteRewriter_NoteFileDeleted 笔记已被 retention 删除时静默跳过
func TestNoteRewriter_NoteFileDeleted(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"deleted-note.toml": noteHookTOML("deleted-note-test", "echo success", "del-note", "REMOVE", "KEEP", "[on.post_update_note]\n"),
		},
	})

	noteRelPath := testutil.RelIn(t, env.rootDir, "test_note.md")
	noteAbsPath := noteRelPath.Abs(env.rootDir)
	initialContent := "Some text.\n/del-note a\nOther text."

	// 模拟 retention 删除笔记文件：先写入再删除
	require.NoError(t, os.WriteFile(noteAbsPath, []byte(initialContent), 0644))
	require.NoError(t, os.Remove(noteAbsPath))

	ok, err := env.runner.rewrite.Rewrite(context.Background(), noteRewriteRequest{
		Dir:         directory.FromRepository(relpath.Path{}),
		NoteRelPath: noteRelPath,
		Content:     initialContent,
		TriggerName: "post_update_note",
	})
	assert.NoError(t, err)
	assert.False(t, ok)
}

// TestNoteRewriter_OnFailRemove_OtherError_Propagates on_fail_action=REMOVE 但失败原因与文件无关时仍应上报
func TestNoteRewriter_OnFailRemove_OtherError_Propagates(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"remove-other-fail.toml": noteHookTOML("remove-other-fail-test", shellPrint("exit 1"), "remove-other-fail", "KEEP", "REMOVE", "[on.post_update_note]\n"),
		},
	})

	noteRelPath := testutil.RelIn(t, env.rootDir, "test_note.md")
	initialContent := "Some text.\n/remove-other-fail a\nOther text."
	require.NoError(t, os.WriteFile(noteRelPath.Abs(env.rootDir), []byte(initialContent), 0644))

	ok, err := env.runner.rewrite.Rewrite(context.Background(), noteRewriteRequest{
		Dir:         directory.FromRepository(relpath.Path{}),
		NoteRelPath: noteRelPath,
		Content:     initialContent,
		TriggerName: "post_update_note",
	})
	require.Error(t, err)
	assert.True(t, ok)

	assert.NotContains(t, readFile(t, noteRelPath.Abs(env.rootDir)), "/remove-other-fail", "指令行仍应被移除")
}

// TestRunner_loadHooks_StrictParsing 未知字段视为拼写错误，严格解析下整份配置被跳过
func TestRunner_loadHooks_StrictParsing(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"invalid.toml": "id = \"strict-test\"\nname = \"strict-test\"\ncommand = \"echo success\"\nunknown_key_here = \"oops\"\n\n[directive]\nname = \"strict\"\n",
		},
	})

	configs, err := env.config.Load()
	require.NoError(t, err)

	for _, c := range configs {
		assert.NotEqual(t, "strict-test", c.ID)
	}
}

func TestRunner_NotificationOnSuccess(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			// 不使用裸 echo：Windows 上会从 PATH 解析到第三方 echo 实现（如 msys），在受限环境中可能无法启动
			"success.toml": hookTOML("notif-success-test", "通知成功测试", shellPrint("echo hello_success"), "[on.image_dispatch]\n"),
		},
	})

	absPath := filepath.Join(env.rootDir, "a.png")
	require.NoError(t, env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:notif-success-test"), "image_dispatch"))

	notifs := env.notifier.Notifications()
	require.Len(t, notifs, 1)
	assert.Equal(t, "通知成功测试", notifs[0].Channel)
	assert.Equal(t, "", notifs[0].Title)
	assert.Equal(t, shared.NotificationPriorityLow, notifs[0].Opts.Priority())
	assert.Contains(t, notifs[0].Opts.Body(), "hello_success")
}

func TestRunner_NotificationOnFailure(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"fail.toml": hookTOML("notif-fail-test", "通知失败测试", shellPrint("echo stderr_err_msg >&2 && exit 1"), "[on.image_dispatch]\n"),
		},
	})

	absPath := filepath.Join(env.rootDir, "a.png")
	require.Error(t, env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:notif-fail-test"), "image_dispatch"))

	notifs := env.notifier.Notifications()
	require.Len(t, notifs, 1)
	assert.Equal(t, "通知失败测试", notifs[0].Channel)
	assert.Equal(t, "", notifs[0].Title)
	assert.Equal(t, shared.NotificationPriorityHigh, notifs[0].Opts.Priority())
	assert.Contains(t, notifs[0].Opts.Body(), "stderr_err_msg")
}

// keepWriterCmd 返回向 IMAGE_FUNNEL_ACTION 文件写入指定内容的命令（不产生 stdout 输出）
func keepWriterCmd(content string) string {
	if filepath.Separator == '/' {
		return `sh -c 'echo ` + content + ` > "$IMAGE_FUNNEL_ACTION"'`
	}
	return `cmd /c "echo ` + content + ` > %IMAGE_FUNNEL_ACTION%"`
}

func TestRunner_SilentKeepRun_ShouldNotNotify(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"silent-keep-test.toml": hookTOML("silent-keep-test", "silent-keep-test", keepWriterCmd("KEEP"), "[on.image_dispatch]\n"),
		},
	})

	absPath := filepath.Join(env.rootDir, "a.png")
	require.NoError(t, env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:silent-keep-test"), "image_dispatch"))

	assert.Empty(t, env.notifier.Notifications(), "脚本覆盖 KEEP 且无输出时应跳过成功通知")
}

func TestRunner_KeepRunWithOutput_ShouldNotify(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"keep-output-test.toml": hookTOML("keep-output-test", "keep-output-test", shellKeepThenPrint("kept_result_msg"), "[on.image_dispatch]\n"),
		},
	})

	absPath := filepath.Join(env.rootDir, "a.png")
	require.NoError(t, env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:keep-output-test"), "image_dispatch"))

	notifs := env.notifier.Notifications()
	require.Len(t, notifs, 1, "KEEP 但有输出时仍应照常通知")
	assert.Equal(t, shared.NotificationPriorityLow, notifs[0].Opts.Priority())
	assert.Contains(t, notifs[0].Opts.Body(), "kept_result_msg")
}

func TestRunner_SilentSuccessWithoutKeep_ShouldNotify(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"silent-no-keep-test.toml": hookTOML("silent-no-keep-test", "silent-no-keep-test", shellPrint("exit 0"), "[on.image_dispatch]\n"),
		},
	})

	absPath := filepath.Join(env.rootDir, "a.png")
	require.NoError(t, env.runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:silent-no-keep-test"), "image_dispatch"))

	assert.Len(t, env.notifier.Notifications(), 1, "未写 KEEP 的静默成功不属于跳过，仍应照常通知")
}

func TestRunner_Order_Parsing(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"with-order.toml": "id = \"with-order\"\nname = \"有顺序测试\"\ncommand = \"echo 1\"\norder = 5\n\n[on.image_dispatch]\n",
			"no-order.toml":   "id = \"no-order\"\nname = \"无顺序测试\"\ncommand = \"echo 2\"\n\n[on.image_dispatch]\n",
		},
	})

	configs, err := env.config.Load()
	require.NoError(t, err)
	require.Len(t, configs, 2)

	for _, c := range configs {
		switch c.ID {
		case "with-order":
			assert.Equal(t, 5, c.Order, "显式声明的 order 应被正确解析")
		case "no-order":
			assert.Equal(t, 0, c.Order, "未声明的 order 应默认为 0")
		}
	}
}

func TestRunner_sortByOrderAndFilename(t *testing.T) {
	hooks := []hookConfig{
		{Filename: "02_foo.toml", ID: "a", Order: 1},
		{Filename: "01_bar.toml", ID: "z", Order: 1},
		{Filename: "03_baz.toml", ID: "m", Order: 2},
	}

	slices.SortStableFunc(hooks, sortByOrderAndFilename)

	assert.Equal(t, "01_bar.toml", hooks[0].Filename, "order 相同（均为 1），按 Filename 升序应为 01_bar.toml")
	assert.Equal(t, "02_foo.toml", hooks[1].Filename, "order 相同（均为 1），按 Filename 升序应为 02_foo.toml")
	assert.Equal(t, "03_baz.toml", hooks[2].Filename, "order 较大（2），应排在最后")
}

// TestRunner_Order_PostCommitSession 纯提交钩子按 order 升序执行
func TestRunner_Order_PostCommitSession(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{})
	flags := map[string]string{
		"a": filepath.Join(env.rootDir, "order_flag_a"),
		"b": filepath.Join(env.rootDir, "order_flag_b"),
		"c": filepath.Join(env.rootDir, "order_flag_c"),
	}
	orders := map[string]string{"a": "order = 2\n", "b": "order = 1\n", "c": ""}
	for id, flag := range flags {
		env.writeHook(id+".toml", fmt.Sprintf("id = %q\ncommand = '''%s'''\n%s\n[on.post_commit_session]\n",
			id, shellRedirect("echo "+id, flag), orders[id]))
	}

	require.NoError(t, env.runner.OnCommitSession(context.Background(), relpath.Path{}))

	require.True(t, waitFlagFile(flags["c"], 1500*time.Millisecond), "order=0 的 c 应最先执行")
	require.True(t, waitFlagFile(flags["b"], 1500*time.Millisecond), "order=1 的 b 应第二个执行")
	require.True(t, waitFlagFile(flags["a"], 1500*time.Millisecond), "order=2 的 a 应最后执行")

	// 验证文件创建时间顺序（c 最早，a 最晚）
	infoC := fileInfo(t, flags["c"])
	infoB := fileInfo(t, flags["b"])
	infoA := fileInfo(t, flags["a"])
	assert.True(t, infoC.ModTime().Before(infoB.ModTime()) || infoC.ModTime().Equal(infoB.ModTime()), "c 应早于或等于 b 执行")
	assert.True(t, infoB.ModTime().Before(infoA.ModTime()) || infoB.ModTime().Equal(infoA.ModTime()), "b 应早于或等于 a 执行")
}

// TestRunner_Order_PostUpdateNote 无指令的笔记钩子按 order 升序执行
func TestRunner_Order_PostUpdateNote(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{})
	flags := map[string]string{
		"x": filepath.Join(env.rootDir, "order_flag_x"),
		"y": filepath.Join(env.rootDir, "order_flag_y"),
		"z": filepath.Join(env.rootDir, "order_flag_z"),
	}
	orders := map[string]string{"x": "order = 3\n", "y": "order = 1\n", "z": "order = 2\n"}
	for id, flag := range flags {
		env.writeHook(id+".toml", fmt.Sprintf("id = %q\ncommand = '''%s'''\n%s\n[on.post_update_note]\n",
			id, shellRedirect("echo "+id, flag), orders[id]))
	}

	noteRelPath := testutil.RelIn(t, env.rootDir, "test_note.md")
	require.NoError(t, os.WriteFile(noteRelPath.Abs(env.rootDir), []byte("Normal note without directives"), 0644))

	require.NoError(t, env.runner.noteUpdate.HandleFileChanged(&directory.FileChangedEvent{
		DirectoryID: scalar.ToID("dir:1"),
		RelPath:     noteRelPath,
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	}))

	require.True(t, waitFlagFile(flags["y"], 1500*time.Millisecond), "order=1 的 y 应最先执行")
	require.True(t, waitFlagFile(flags["z"], 1500*time.Millisecond), "order=2 的 z 应第二个执行")
	require.True(t, waitFlagFile(flags["x"], 1500*time.Millisecond), "order=3 的 x 应最后执行")

	// 验证文件创建时间顺序（y 最早，x 最晚）
	infoY := fileInfo(t, flags["y"])
	infoZ := fileInfo(t, flags["z"])
	infoX := fileInfo(t, flags["x"])
	assert.True(t, infoY.ModTime().Before(infoZ.ModTime()) || infoY.ModTime().Equal(infoZ.ModTime()), "y 应早于或等于 z 执行")
	assert.True(t, infoZ.ModTime().Before(infoX.ModTime()) || infoZ.ModTime().Equal(infoX.ModTime()), "z 应早于或等于 x 执行")
}

// TestNewRunner_MissingDependencyPanics 任一依赖缺失都在构造期失败，
// 而不是等到某条触发路径被走到时才退化为静默无输出
func TestNewRunner_MissingDependencyPanics(t *testing.T) {
	tempDir := t.TempDir()
	full := Deps{
		RootDir:            tempDir,
		HooksDir:           tempDir,
		DataDir:            tempDir,
		GraphQLURL:         "http://127.0.0.1:0/graphql",
		Logger:             zap.NewNop(),
		MetadataUpdatedSub: &mockMetadataUpdatedSub{},
		FileChangedSub:     &mockFileChangedSub{},
		TokenSource:        &mockTokenSource{},
		ImageRepo:          &mockImageRepository{},
		DirRepo:            &mockDirectoryRepository{},
		NotificationSender: &mockNotificationSender{},
	}

	missing := map[string]func(*Deps){
		"RootDir":            func(d *Deps) { d.RootDir = "" },
		"HooksDir":           func(d *Deps) { d.HooksDir = "" },
		"DataDir":            func(d *Deps) { d.DataDir = "" },
		"GraphQLURL":         func(d *Deps) { d.GraphQLURL = "" },
		"Logger":             func(d *Deps) { d.Logger = nil },
		"MetadataUpdatedSub": func(d *Deps) { d.MetadataUpdatedSub = nil },
		"FileChangedSub":     func(d *Deps) { d.FileChangedSub = nil },
		"TokenSource":        func(d *Deps) { d.TokenSource = nil },
		"ImageRepo":          func(d *Deps) { d.ImageRepo = nil },
		"DirRepo":            func(d *Deps) { d.DirRepo = nil },
		"NotificationSender": func(d *Deps) { d.NotificationSender = nil },
	}

	for name, drop := range missing {
		t.Run(name, func(t *testing.T) {
			deps := full
			drop(&deps)
			assert.PanicsWithValue(t, "hook.Deps."+name+" is required", func() { NewRunner(deps) })
		})
	}
}

// readFile 读取文件内容
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// fileInfo 读取文件元信息
func fileInfo(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info
}
