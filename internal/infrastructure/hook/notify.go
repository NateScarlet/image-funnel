package hook

import (
	"context"
	"strings"

	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// hookNotifier 钩子执行结果的统一通知出口：把一次执行翻译为通用通知系统可消费的消息。
// 标题优先级为指令文本 > 笔记路径 > 目录根相对路径 > 命令行，
// 使通知在多钩子并发时仍能指向用户关心的对象。
type hookNotifier struct {
	sender shared.NotificationSender
	logger *zap.Logger
}

func newHookNotifier(sender shared.NotificationSender, logger *zap.Logger) *hookNotifier {
	return &hookNotifier{sender: sender, logger: logger}
}

func (n *hookNotifier) Notify(ctx context.Context, task hookExecutionTask, execErr error, stdoutStr, stderrStr string) {
	hookName := task.HookName
	if hookName == "" {
		hookName = task.HookID
	}

	var title string
	switch {
	case task.DirectiveText != "":
		title = task.DirectiveText
	case task.NotePath != "":
		title = task.NotePath
	case task.Dir != nil:
		title = task.Dir.RelPath().String()
	default:
		title = task.Command + strings.Join(task.ExtraArgs, " ")
	}

	var sb strings.Builder
	sb.WriteString(stdoutStr)

	var priority shared.NotificationPriority
	if execErr != nil {
		priority = shared.NotificationPriorityHigh
		if stderrStr != "" {
			sb.WriteString("---\n# stderr\n")
			sb.WriteString(stderrStr)
		}
	} else {
		priority = shared.NotificationPriorityLow
	}

	opts := []shared.SendNotificationOption{shared.WithPriority(priority)}
	if body := sb.String(); body != "" {
		opts = append(opts, shared.WithBody(body))
	}
	if task.DirectiveText != "" && task.Dir != nil {
		opts = append(opts, shared.WithDetailsURL(scalar.MustParseURI("/browse?dir="+task.Dir.ID().String())))
	}

	// 通知投递失败不向上传播：脚本已经执行完并产生了结果，把投递失败当成执行失败会让
	// 上层重复报错、并掩盖真正的执行结果。此处只记录，缺失的通知由日志追溯。
	if _, err := n.sender.SendNotification(ctx, hookName, title, opts...); err != nil {
		n.logger.Error("failed to send hook notification",
			zap.String("hook_id", task.HookID),
			zap.Error(err),
		)
	}
}
