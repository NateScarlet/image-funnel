package hook

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"main/internal/shared"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// executeHookSync 同步执行钩子并返回解析后的操作、stdout 和 stderr
// 返回的操作已解析：成功时优先使用脚本覆盖值，否则使用 on_success_action；失败时使用 on_fail_action
func (r *Runner) executeHookSync(hook hookConfig, triggerName string, events []hookEvent, extraArgs []string, notePath string, dirID string, dirRel string, runID string) (action string, stdout string, stderr string, err error) {
	if runID == "" {
		runID = fmt.Sprintf("run_%019d_%06d", time.Now().UnixNano(), rand.Intn(1000000))
	}
	resChan := make(chan hookExecutionResult, 1)

	// 构建完整命令行，对含空格/引号的参数用引号包裹
	var cmdParts []string
	cmdParts = append(cmdParts, hook.Command)
	for _, arg := range extraArgs {
		if strings.Contains(arg, " ") || strings.Contains(arg, "\"") {
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			cmdParts = append(cmdParts, fmt.Sprintf("\"%s\"", escaped))
		} else {
			cmdParts = append(cmdParts, arg)
		}
	}
	fullCommand := strings.Join(cmdParts, " ")

	r.ch <- hookExecutionTask{
		HookID:       hook.ID,
		HookName:     hook.Name,
		Command:      fullCommand,
		ExtraArgs:    extraArgs,
		TriggerName:  triggerName,
		Events:       events,
		Dir:          hook.Dir,
		Env:          hook.Env,
		NotePath:     notePath,
		DirectoryID:  dirID,
		DirectoryRel: dirRel,
		resultChan:   resChan,
		RunID:        runID,
	}

	select {
	case result := <-resChan:
		if result.Error != nil {
			// 失败时总是使用 on_fail_action
			if hook.Directive != nil {
				return hook.Directive.OnFailAction, result.Stdout, result.Stderr, result.Error
			}
			return "", result.Stdout, result.Stderr, result.Error
		}
		// 成功：脚本覆盖优先，否则使用 on_success_action
		if result.Action != "" {
			return result.Action, result.Stdout, result.Stderr, nil
		}
		if hook.Directive != nil {
			return hook.Directive.OnSuccessAction, result.Stdout, result.Stderr, nil
		}
		return "", result.Stdout, result.Stderr, nil
	case <-r.ctx.Done():
		return "", "", "", r.ctx.Err()
	}
}

func (r *Runner) executeHook(ctx context.Context, task hookExecutionTask) {
	var ids []string
	var paths []string

	var rating, oldRating int
	var label, oldLabel, action, oldAction string

	for _, ev := range task.Events {
		ids = append(ids, ev.ID)
		paths = append(paths, ev.Path)
	}

	if len(task.Events) == 1 {
		rating = task.Events[0].Rating
		label = task.Events[0].Label
		action = task.Events[0].Action
		oldRating = task.Events[0].OldRating
		oldLabel = task.Events[0].OldLabel
		oldAction = task.Events[0].OldAction
	}

	cmd := newHookCmd(ctx, task.Command)

	cmd.Dir = task.Dir // 将脚本的工作目录设置为 Hook 配置文件所在的目录

	// 生成临时文件路径供脚本通过 IMAGE_FUNNEL_ACTION 写入覆盖操作，不提前创建文件
	actionFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("image_funnel_action_%s.txt", task.RunID))

	var noteAbsPath string
	if task.NotePath != "" {
		noteAbsPath = filepath.Join(r.rootDir, task.NotePath)
	}

	env, buildErr := r.buildBaseEnv(ctx, task.HookID, task.HookName, task.TriggerName, ids, paths, noteAbsPath, task.Env, task.DirectoryID, task.DirectoryRel)
	if buildErr != nil {
		execErr := fmt.Errorf("failed to build hook env: %w", buildErr)
		r.sendHookNotification(ctx, task, execErr, "", "")
		task.resultChan <- hookExecutionResult{Error: execErr}
		return
	}

	env = append(env,
		"IMAGE_FUNNEL_IMAGE_RATING="+fmt.Sprintf("%d", rating),
		"IMAGE_FUNNEL_IMAGE_LABEL="+label,
		"IMAGE_FUNNEL_IMAGE_ACTION="+action,
		"IMAGE_FUNNEL_IMAGE_OLD_RATING="+fmt.Sprintf("%d", oldRating),
		"IMAGE_FUNNEL_IMAGE_OLD_LABEL="+oldLabel,
		"IMAGE_FUNNEL_IMAGE_OLD_ACTION="+oldAction,
		"IMAGE_FUNNEL_ACTION="+actionFilePath,
	)

	env = append(env, "IMAGE_FUNNEL_HOOK_RUN_ID="+task.RunID)

	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	r.logger.Info("will execute external hook command",
		zap.String("hook_id", task.HookID),
		zap.String("trigger", task.TriggerName),
		zap.Int("batch_size", len(task.Events)),
	)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// 清理临时文件
	defer func() {
		if removeErr := os.Remove(actionFilePath); removeErr != nil && !os.IsNotExist(removeErr) {
			r.logger.Warn("failed to clean up action file", zap.String("path", actionFilePath), zap.Error(removeErr))
		}
	}()

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if err != nil {
		r.logger.Error("external hook command failed",
			zap.String("hook_id", task.HookID),
			zap.Duration("duration", duration),
			zap.Error(err),
			zap.String("stdout", stdoutStr),
			zap.String("stderr", stderrStr),
		)
		stderrTrimmed := strings.TrimSpace(stderrStr)
		var execErr error
		if stderrTrimmed != "" {
			execErr = fmt.Errorf("hook script failed: %w, stderr: %s", err, stderrTrimmed)
		} else {
			execErr = fmt.Errorf("hook script failed: %w", err)
		}
		r.sendHookNotification(ctx, task, execErr, stdoutStr, stderrStr)
		task.resultChan <- hookExecutionResult{Error: execErr, Stdout: stdoutStr, Stderr: stderrStr}
		return
	}

	r.logger.Info("external hook command completed",
		zap.String("hook_id", task.HookID),
		zap.Duration("duration", duration),
		zap.String("stdout", stdoutStr),
	)
	// 开发环境通过 Debug 级别输出 stderr，生产环境自动过滤
	if stderrStr != "" {
		r.logger.Debug("external hook stderr",
			zap.String("hook_id", task.HookID),
			zap.String("stderr", stderrStr),
		)
	}

	// 读取脚本通过 IMAGE_FUNNEL_ACTION 写入的覆盖操作
	var overrideAction string
	data, readErr := os.ReadFile(actionFilePath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			// 文件不存在 = 脚本未覆盖，正常路径
		} else {
			errMsg := fmt.Sprintf("failed to read IMAGE_FUNNEL_ACTION file: %v", readErr)
			r.logger.Error(errMsg, zap.String("hook_id", task.HookID), zap.String("path", actionFilePath))
			execErr := fmt.Errorf("%s", errMsg)
			r.sendHookNotification(ctx, task, execErr, stdoutStr, stderrStr)
			task.resultChan <- hookExecutionResult{Error: execErr, Stdout: stdoutStr, Stderr: stderrStr}
			return
		}
	} else {
		overrideAction = strings.TrimSpace(string(data))
		if overrideAction != "" && !isValidDirectiveAction(overrideAction) {
			errMsg := fmt.Sprintf("unsupported action in IMAGE_FUNNEL_ACTION file: %q", overrideAction)
			r.logger.Error(errMsg, zap.String("hook_id", task.HookID))
			execErr := fmt.Errorf("%s", errMsg)
			r.sendHookNotification(ctx, task, execErr, stdoutStr, stderrStr)
			task.resultChan <- hookExecutionResult{Error: execErr, Stdout: stdoutStr, Stderr: stderrStr}
			return
		}
	}

	r.sendHookNotification(ctx, task, nil, stdoutStr, stderrStr)
	task.resultChan <- hookExecutionResult{Action: overrideAction, Stdout: stdoutStr, Stderr: stderrStr}
}

// sendHookNotification 捕获钩子执行状态并向通用通知系统发送反馈
func (r *Runner) sendHookNotification(ctx context.Context, task hookExecutionTask, execErr error, stdoutStr, stderrStr string) {
	hookName := task.HookName
	if hookName == "" {
		hookName = task.HookID
	}

	var title string
	var priority shared.NotificationPriority
	var body string

	title = task.NotePath

	var sb = new(strings.Builder)
	sb.WriteString(stderrStr)
	if execErr != nil {
		priority = shared.NotificationPriorityHigh
		if stderrStr != "" {
			sb.WriteString("---\n# stderr\n")
			sb.WriteString(stderrStr)
		}
	} else {
		priority = shared.NotificationPriorityLow
	}

	tag := uuid.NewString()
	var opts []shared.SendNotificationOption
	opts = append(opts, shared.WithPriority(priority))
	if body != "" {
		opts = append(opts, shared.WithBody(body))
	}

	if _, err := r.notifSender.SendNotification(ctx, tag, hookName, title, opts...); err != nil {
		r.logger.Error("failed to send hook notification",
			zap.String("hook_id", task.HookID),
			zap.Error(err),
		)
	}
}

// isValidDirectiveAction 检查操作是否为支持的指令操作
func isValidDirectiveAction(action string) bool {
	switch action {
	case "COMMENT_OUT", "REMOVE", "KEEP":
		return true
	default:
		return false
	}
}
