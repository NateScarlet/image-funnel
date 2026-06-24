package session

import (
	"context"

	"main/internal/scalar"
)

// FakeHookRunner 空实现的钩子执行器，供测试使用
type FakeHookRunner struct{}

func (r *FakeHookRunner) Trigger(ctx context.Context, ids []string, paths []string, hookID scalar.ID, triggerName string) error {
	return nil
}

func (r *FakeHookRunner) OnCommitSession(ctx context.Context, dirID scalar.ID, dirRelPath string) error {
	return nil
}

func (r *FakeHookRunner) TriggerForNote(ctx context.Context, noteRelPath string, hookID scalar.ID) error {
	return nil
}