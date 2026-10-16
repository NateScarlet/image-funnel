package hook

import (
	"context"
	"errors"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// scriptedProcessRunner 进程原语替身：每次执行时对文件系统施加一段副作用，
// 用于模拟「脚本在执行期间改写笔记」这类进程之外无法直接触发的时序。
type scriptedProcessRunner struct {
	onRun func(env []string) (processOutput, error)
}

func (p *scriptedProcessRunner) Run(_ context.Context, _ []string, env []string, _ string) (processOutput, error) {
	if p.onRun != nil {
		return p.onRun(env)
	}
	return processOutput{}, nil
}

func (p *scriptedProcessRunner) SpawnResident(context.Context, []string, []string, string) (residentProcess, error) {
	return nil, errors.New("scriptedProcessRunner 不实现常驻进程")
}

// newTestNoteRewriter 装配只含指令改写器的测试环境：配置来源与进程原语从 seam 注入，
// 无需构造完整 Runner。
func newTestNoteRewriter(t *testing.T, hooks map[string]string, process processRunner, logger *zap.Logger) (*noteRewriter, *isolatedTriggerDeps) {
	t.Helper()
	deps := newIsolatedDeps(t, hooks, process, nil)
	rewriter := newNoteRewriter(
		deps.config, deps.exec, deps.notes, newWriteGuard(), deps.exec.notifier, deps.rootDir, logger,
	)
	return rewriter, deps
}

// newTestEraseRewriter 只构造擦除所需的改写器：擦除阶段不依赖配置、执行器与笔记上下文
func newTestEraseRewriter() *noteRewriter {
	return &noteRewriter{logger: zap.NewNop()}
}

// TestIdentityScanner_DistinguishesRepeatedText 身份由「原文 + 出现次序」构成，
// 原文相同的指令行靠次序各自区分，空白差异不改变身份
func TestIdentityScanner_DistinguishesRepeatedText(t *testing.T) {
	scanner := newIdentityScanner()

	assert.Equal(t, directiveIdentity{line: "/fork a", ordinal: 0}, scanner.identify("/fork a\n"))
	assert.Equal(t, directiveIdentity{line: "/comfyui b", ordinal: 0}, scanner.identify("  /comfyui b\r\n"))
	assert.Equal(t, directiveIdentity{line: "/fork a", ordinal: 1}, scanner.identify("/fork a"))
}

// TestEraseDirectives_MatchesByIdentity 擦除按指令身份查表，而不是按两次匹配的数组下标配对
func TestEraseDirectives_MatchesByIdentity(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		results  map[directiveIdentity]directiveResult
		contains []string
		absent   []string
	}{
		{
			name: "脚本改写指令行后保留当前内容而不错配",
			// 收集阶段见到的是 /fork a 与 /fork b；脚本随后把 /fork b 改写为 /fork c
			content: "text\n/fork a\n/fork c\nmore",
			results: map[directiveIdentity]directiveResult{
				{line: "/fork a", ordinal: 0}: {action: "REMOVE"},
				{line: "/fork b", ordinal: 0}: {action: "REMOVE"},
			},
			contains: []string{"/fork c"},
			absent:   []string{"/fork a"},
		},
		{
			name:    "原文完全相同的重复指令各自取回自己的结果",
			content: "/dup a\n/dup a\n",
			results: map[directiveIdentity]directiveResult{
				{line: "/dup a", ordinal: 0}: {action: "REMOVE"},
				{line: "/dup a", ordinal: 1}: {
					action:     "KEEP",
					stdout:     "second_run",
					executedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				},
			},
			contains: []string{"/dup a", "second_run"},
		},
		{
			name:    "未收集到结果的指令行（如被过滤跳过）保持原样",
			content: "/unknown a\n/fork a\n",
			results: map[directiveIdentity]directiveResult{
				{line: "/fork a", ordinal: 0}: {action: "REMOVE"},
			},
			contains: []string{"/unknown a"},
			absent:   []string{"/fork a"},
		},
	}

	rewriter := newTestEraseRewriter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriter.eraseDirectives(tt.content, tt.results)
			for _, want := range tt.contains {
				assert.Contains(t, got, want)
			}
			for _, unwanted := range tt.absent {
				assert.NotContains(t, got, unwanted)
			}
		})
	}

	// 重复指令的首次出现被移除、第二次保留：两者结果互不串位
	got := rewriter.eraseDirectives("/dup a\n/dup a\n", map[directiveIdentity]directiveResult{
		{line: "/dup a", ordinal: 0}: {action: "REMOVE"},
		{line: "/dup a", ordinal: 1}: {action: "KEEP"},
	})
	assert.Equal(t, 1, strings.Count(got, "/dup a"), "只有一条 /dup a 应被移除")
}

// TestNoteRewriter_ScriptRewritesDirectiveLine_KeepsCurrentContent
// 脚本在执行期间改写了指令行时，该行按身份查不到结果，应保留当前内容而不是错配到别的指令结果
func TestNoteRewriter_ScriptRewritesDirectiveLine_KeepsCurrentContent(t *testing.T) {
	runner := &scriptedProcessRunner{}
	rewriter, deps := newTestNoteRewriter(t, map[string]string{
		"alpha.toml": noteHookTOML("alpha-test", shellPrint("echo alpha_out"), "alpha", "REMOVE", "REMOVE", "[on.post_update_note]\n"),
	}, runner, zap.NewNop())

	noteRelPath := testutil.RelIn(t, deps.rootDir, "note.md")
	noteAbsPath := noteRelPath.Abs(deps.rootDir)
	initialContent := "text\n/alpha 1\nmore"
	require.NoError(t, os.WriteFile(noteAbsPath, []byte(initialContent), 0644))

	// 脚本执行时把指令行改写成参数不同的另一条指令
	runner.onRun = func(_ []string) (processOutput, error) {
		content, err := os.ReadFile(noteAbsPath)
		if err != nil {
			return processOutput{}, err
		}
		rewritten := strings.Replace(string(content), "/alpha 1", "/alpha 2", 1)
		if err := os.WriteFile(noteAbsPath, []byte(rewritten), 0644); err != nil {
			return processOutput{}, err
		}
		return processOutput{Stdout: "alpha_out"}, nil
	}

	executed, err := rewriter.Rewrite(context.Background(), noteRewriteRequest{
		Dir:         directory.FromRepository(relpath.Path{}),
		NoteRelPath: noteRelPath,
		Content:     initialContent,
		TriggerName: "post_update_note",
	})
	require.NoError(t, err)
	require.True(t, executed)

	content := readFile(t, noteAbsPath)
	assert.Contains(t, content, "/alpha 2", "被脚本改写的指令行应保留当前内容")
	assert.NotContains(t, content, "/alpha 1", "原指令行不应残留")
	assert.NotContains(t, content, "hook-run-id")
}

// TestNoteRewriter_CleanupLate_MatchesMainErase 迟到擦除路径与主擦除路径产出的文本一致，
// 包括 stdout 结果提示块（不再退化为只能按动作兜底、输出传空）
func TestNoteRewriter_CleanupLate_MatchesMainErase(t *testing.T) {
	rewriter, deps := newTestNoteRewriter(t, map[string]string{
		"keepout.toml": noteHookTOML("keepout-test", shellPrint("echo keepout_stdout"), "keepout", "KEEP", "KEEP", "[on.post_update_note]\n"),
	}, execProcessRunner{log: zap.NewNop()}, zap.NewNop())

	noteRelPath := testutil.RelIn(t, deps.rootDir, "note.md")
	noteAbsPath := noteRelPath.Abs(deps.rootDir)
	initialContent := "text\n/keepout\nmore"
	require.NoError(t, os.WriteFile(noteAbsPath, []byte(initialContent), 0644))

	executed, err := rewriter.Rewrite(context.Background(), noteRewriteRequest{
		Dir:         directory.FromRepository(relpath.Path{}),
		NoteRelPath: noteRelPath,
		Content:     initialContent,
		TriggerName: "post_update_note",
	})
	require.NoError(t, err)
	require.True(t, executed)
	mainContent := readFile(t, noteAbsPath)
	require.Contains(t, mainContent, "keepout_stdout")

	// 取出主运行登记的结果，并还原「带 run_id 的迟到写入」现场
	rewriter.mu.Lock()
	require.Len(t, rewriter.tasks, 1)
	var runID string
	var results map[directiveIdentity]directiveResult
	for id, task := range rewriter.tasks {
		runID = id
		results = make(map[directiveIdentity]directiveResult, len(task.results))
		maps.Copy(results, task.results)
	}
	rewriter.mu.Unlock()
	require.NotEmpty(t, runID)

	require.NoError(t, os.WriteFile(noteAbsPath, []byte(setHookRunID(initialContent, runID)), 0644))

	require.NoError(t, rewriter.CleanupLate(noteAbsPath, runID, results))

	assert.Equal(t, mainContent, readFile(t, noteAbsPath), "迟到擦除应与主擦除产出完全一致的文本")
}

// TestNoteRewriter_UnknownRunID_TreatedAsFreshRun 无法识别的运行标记（崩溃残留、手动重放）
// 按全新运行重新执行指令，并落一条 debug 日志说明按新运行处理
func TestNoteRewriter_UnknownRunID_TreatedAsFreshRun(t *testing.T) {
	core, recorded := observer.New(zap.DebugLevel)
	rewriter, deps := newTestNoteRewriter(t, map[string]string{
		"alpha.toml": noteHookTOML("alpha-test", shellPrint("echo alpha_out"), "alpha", "REMOVE", "REMOVE", "[on.post_update_note]\n"),
	}, &scriptedProcessRunner{}, zap.New(core))

	noteRelPath := testutil.RelIn(t, deps.rootDir, "note.md")
	noteAbsPath := noteRelPath.Abs(deps.rootDir)
	initialContent := "---\nhook-run-id: run_stale_123\n---\ntext\n/alpha\nmore"
	require.NoError(t, os.WriteFile(noteAbsPath, []byte(initialContent), 0644))

	executed, err := rewriter.Rewrite(context.Background(), noteRewriteRequest{
		Dir:         directory.FromRepository(relpath.Path{}),
		NoteRelPath: noteRelPath,
		Content:     initialContent,
		TriggerName: "post_update_note",
	})
	require.NoError(t, err)
	require.True(t, executed, "未知运行标记应按全新运行执行指令")

	content := readFile(t, noteAbsPath)
	assert.NotContains(t, content, "/alpha", "指令应被全新运行执行并擦除")
	assert.NotContains(t, content, "run_stale_123", "残留运行标记应被替换并清理")

	assert.NotEmpty(t,
		recorded.FilterMessage("unrecognized hook-run-id, treating as a fresh run").All(),
		"未知运行标记应落一条 debug 日志")
}
