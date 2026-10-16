package hook

import (
	"context"
	"fmt"
	"os"

	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/scalar"
)

// noteDispatchTrigger 手动笔记派发触发路径（note_dispatch）：
// 由用户在界面上对笔记主动触发钩子。若目标钩子声明了指令，
// 先让指令改写器处理笔记中的斜杠指令；没有指令被执行时才回退为整篇派发。
type noteDispatchTrigger struct {
	rootDir string
	config  configSource
	exec    *hookExecutor
	rewrite *noteRewriter
	notes   *noteContext
}

func newNoteDispatchTrigger(
	rootDir string,
	config configSource,
	exec *hookExecutor,
	rewrite *noteRewriter,
	notes *noteContext,
) *noteDispatchTrigger {
	return &noteDispatchTrigger{rootDir: rootDir, config: config, exec: exec, rewrite: rewrite, notes: notes}
}

// Dispatch 同步执行一次手动笔记派发
func (t *noteDispatchTrigger) Dispatch(ctx context.Context, noteRelPath relpath.Path, hookID scalar.ID) error {
	hooks, err := t.config.Load()
	if err != nil {
		return err
	}
	target, err := resolveHookConfig(hooks, hookID)
	if err != nil {
		return err
	}
	if target.On.NoteDispatch == nil {
		return fmt.Errorf("hook %s does not allow note dispatch", hookID.String())
	}

	// 寻找配套的图片
	events, err := t.notes.AssociatedImageEvents(ctx, noteRelPath)
	if err != nil {
		return fmt.Errorf("failed to get associated image for note dispatch: %w", err)
	}

	// 依照领域仓库通过根相对路径加载目录实体
	dir, err := t.notes.DirOfRelPath(ctx, noteRelPath.Dir())
	if err != nil {
		return fmt.Errorf("failed to get directory for note dispatch: %w", err)
	}

	if target.Directive == nil || target.Directive.Name == "" {
		return t.dispatchWhole(target, events, noteRelPath, dir)
	}

	contentBytes, err := os.ReadFile(noteRelPath.Abs(t.rootDir))
	if err != nil {
		return fmt.Errorf("failed to read note file for dispatch: %w", err)
	}

	executed, err := t.rewrite.Rewrite(ctx, noteRewriteRequest{
		Dir:          dir,
		NoteRelPath:  noteRelPath,
		Content:      string(contentBytes),
		TriggerName:  "note_dispatch",
		FilterHookID: hookID,
	})
	if err != nil {
		return err
	}
	if executed {
		return nil
	}

	return t.dispatchWhole(target, events, noteRelPath, dir)
}

// dispatchWhole 把整篇笔记（而非单条指令）交给钩子脚本
func (t *noteDispatchTrigger) dispatchWhole(target hookConfig, events []hookEvent, noteRelPath relpath.Path, dir *directory.Directory) error {
	res := t.exec.Execute(newHookTask(target, "note_dispatch", events, noteRelPath.String(), dir))
	return res.Err
}
