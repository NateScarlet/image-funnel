package hook

import (
	"os"
	"testing"
	"time"

	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoteUpdateTrigger_FinalizesAfterProcessing 普通笔记变更处理完成后发布保存完成事件
func TestNoteUpdateTrigger_FinalizesAfterProcessing(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{})
	rel := testutil.RelIn(t, env.rootDir, "note.md")
	require.NoError(t, os.WriteFile(rel.Abs(env.rootDir), []byte("plain content"), 0644))

	require.NoError(t, env.runner.noteUpdate.HandleFileChanged(&directory.FileChangedEvent{
		DirectoryID: scalar.ToID("dir:1"),
		RelPath:     rel,
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	}))

	assert.Equal(t, []relpath.Path{rel}, env.savedEvents.finalized())
}

// TestNoteUpdateTrigger_DoesNotFinalizeSelfWrite 服务端自身写入产生的事件不发布保存完成事件
func TestNoteUpdateTrigger_DoesNotFinalizeSelfWrite(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{})
	rel := testutil.RelIn(t, env.rootDir, "own-write.md")
	content := []byte("content written by server")
	// 经防重入守卫登记为自身写入，模拟钩子回写产生的文件变更事件
	require.NoError(t, env.runner.noteUpdate.guard.Write(rel.Abs(env.rootDir), content, 0644))

	require.NoError(t, env.runner.noteUpdate.HandleFileChanged(&directory.FileChangedEvent{
		DirectoryID: scalar.ToID("dir:1"),
		RelPath:     rel,
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	}))

	assert.Empty(t, env.savedEvents.finalized())
}

// TestNoteUpdateTrigger_FinalizesAfterDirectiveRewrite 钩子改写完成后才发布，磁盘上是最终内容
func TestNoteUpdateTrigger_FinalizesAfterDirectiveRewrite(t *testing.T) {
	env := newTestHooks(t, testHooksOptions{
		Hooks: map[string]string{
			"alpha.toml": noteHookTOML("alpha-test", shellPrint("echo alpha_out"), "alpha", "REMOVE", "REMOVE", "[on.post_update_note]\n"),
		},
	})
	rel := testutil.RelIn(t, env.rootDir, "directive.md")
	require.NoError(t, os.WriteFile(rel.Abs(env.rootDir), []byte("before\n/alpha\nafter\n"), 0644))

	require.NoError(t, env.runner.noteUpdate.HandleFileChanged(&directory.FileChangedEvent{
		DirectoryID: scalar.ToID("dir:1"),
		RelPath:     rel,
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	}))

	// 指令行已被擦除，说明发布发生在全部回写完成之后
	content, err := os.ReadFile(rel.Abs(env.rootDir))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "/alpha")
	assert.Equal(t, []relpath.Path{rel}, env.savedEvents.finalized())
}
