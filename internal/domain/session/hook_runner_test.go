package session

import (
	"context"

	"main/internal/domain/hook"
	"main/internal/domain/relpath"
	"main/internal/scalar"
)

// FakeHookRunner 空实现的钩子执行器，供测试使用。
// OnCommitSessionFunc 可选：设置后可观察/拦截提交后钩子的触发时机（零值安全）。
type FakeHookRunner struct {
	OnCommitSessionFunc func(relpath.Path) error
}

func (r *FakeHookRunner) Trigger(ctx context.Context, ids []string, paths []string, hookID scalar.ID, triggerName string) error {
	return nil
}

func (r *FakeHookRunner) OnCommitSession(ctx context.Context, dirRelPath relpath.Path) error {
	if r.OnCommitSessionFunc != nil {
		return r.OnCommitSessionFunc(dirRelPath)
	}
	return nil
}

func (r *FakeHookRunner) TriggerForNote(ctx context.Context, noteRelPath relpath.Path, hookID scalar.ID) error {
	return nil
}

func (r *FakeHookRunner) Autocomplete(ctx context.Context, hookID scalar.ID, noteRelPath string, linePrefix string, query string) ([]*hook.AutocompleteSuggestion, error) {
	return nil, nil
}

func (r *FakeHookRunner) CopyContent(ctx context.Context, imageID scalar.ID, imageRelPath relpath.Path) (*hook.CopyContent, error) {
	return nil, nil
}
