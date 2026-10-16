package hook

import (
	"context"
	"fmt"

	"main/internal/scalar"
)

// imageDispatchTrigger 手动图片派发触发路径（image_dispatch）：
// 由用户在界面上对选中图片主动触发钩子，同步等待脚本执行结果。
type imageDispatchTrigger struct {
	config configSource
	exec   *hookExecutor
	notes  *noteContext
}

func newImageDispatchTrigger(config configSource, exec *hookExecutor, notes *noteContext) *imageDispatchTrigger {
	return &imageDispatchTrigger{config: config, exec: exec, notes: notes}
}

// Dispatch 同步执行一次手动派发。
// triggerName 由领域接口保留参数形式：当前只有 image_dispatch 会走到这里，
// note_dispatch 的能力校验一并保留，使接口签名不变时派发权限仍在构造点把关。
func (t *imageDispatchTrigger) Dispatch(ctx context.Context, ids []string, paths []string, hookID scalar.ID, triggerName string) error {
	hooks, err := t.config.Load()
	if err != nil {
		return err
	}
	target, err := resolveHookConfig(hooks, hookID)
	if err != nil {
		return err
	}

	switch triggerName {
	case "image_dispatch":
		if target.On.ImageDispatch == nil {
			return fmt.Errorf("hook %s does not allow manual dispatch", hookID.String())
		}
	case "note_dispatch":
		if target.On.NoteDispatch == nil {
			return fmt.Errorf("hook %s does not allow note dispatch", hookID.String())
		}
	}

	events := make([]hookEvent, 0, len(ids))
	for i, id := range ids {
		var path string
		if i < len(paths) {
			path = paths[i]
		}
		events = append(events, hookEvent{ID: id, Path: path})
	}

	// 从第一个路径推导目录信息，触发必然涉及图片，目录信息总是可推导的
	dir, err := t.notes.DirOf(ctx, paths[0])
	if err != nil {
		return fmt.Errorf("failed to resolve directory for trigger %q: %w", triggerName, err)
	}

	if res := t.exec.Execute(newHookTask(target, triggerName, events, "", dir)); res.Err != nil {
		return res.Err
	}
	return nil
}
