package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"main/internal/scalar"

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

	idsJSON, _ := json.Marshal(ids)
	pathsJSON, _ := json.Marshal(paths)

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

	env := append(os.Environ(),
		"IMAGE_FUNNEL_HOOK_ID="+task.HookID,
		"IMAGE_FUNNEL_HOOK_NAME="+task.HookName,
		"IMAGE_FUNNEL_TRIGGER="+task.TriggerName,
		"IMAGE_FUNNEL_IMAGE_IDS="+string(idsJSON),
		"IMAGE_FUNNEL_IMAGE_PATHS="+string(pathsJSON),
		"IMAGE_FUNNEL_IMAGE_RATING="+fmt.Sprintf("%d", rating),
		"IMAGE_FUNNEL_IMAGE_LABEL="+label,
		"IMAGE_FUNNEL_IMAGE_ACTION="+action,
		"IMAGE_FUNNEL_IMAGE_OLD_RATING="+fmt.Sprintf("%d", oldRating),
		"IMAGE_FUNNEL_IMAGE_OLD_LABEL="+oldLabel,
		"IMAGE_FUNNEL_IMAGE_OLD_ACTION="+oldAction,
		"IMAGE_FUNNEL_ROOT_DIR="+r.rootDir,
		"IMAGE_FUNNEL_ACTION="+actionFilePath,
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
	)

	// 目录信息由所有调用路径保证始终存在，无条件注入环境变量
	env = append(env, "IMAGE_FUNNEL_DIRECTORY_ID="+task.DirectoryID)
	env = append(env, "IMAGE_FUNNEL_DIRECTORY_REL_PATH="+task.DirectoryRel)

	if task.NotePath != "" {
		noteAbsPath := filepath.Join(r.rootDir, task.NotePath)
		mPathsJSON, _ := json.Marshal([]string{noteAbsPath})
		env = append(env, "IMAGE_FUNNEL_NOTE_PATHS="+string(mPathsJSON))
	}
	env = append(env, "IMAGE_FUNNEL_HOOK_RUN_ID="+task.RunID)

	// 注入来自 TOML 配置文件中 [env] 节的自定义环境变量
	for k, v := range task.Env {
		env = append(env, k+"="+v)
	}

	if r.graphqlURL != "" {
		env = append(env, "IMAGE_FUNNEL_GRAPHQL_URL="+r.graphqlURL)
	}

	if r.tokenSource != nil {
		// 签发临时的 JWT 令牌供外部脚本高频访问 GraphQL API 鉴权使用
		tok, err := r.tokenSource.NewAccessToken(ctx, scalar.ToID("hook-runner"))
		if err == nil {
			env = append(env, "IMAGE_FUNNEL_TOKEN="+tok.String())
		} else {
			r.logger.Warn("failed to generate temporary token for hook", zap.Error(err))
		}
	}

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
		if stderrTrimmed != "" {
			task.resultChan <- hookExecutionResult{Error: fmt.Errorf("hook script failed: %w, stderr: %s", err, stderrTrimmed), Stdout: stdoutStr, Stderr: stderrStr}
		} else {
			task.resultChan <- hookExecutionResult{Error: fmt.Errorf("hook script failed: %w", err), Stdout: stdoutStr, Stderr: stderrStr}
		}
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
			task.resultChan <- hookExecutionResult{Error: fmt.Errorf("%s", errMsg), Stdout: stdoutStr, Stderr: stderrStr}
			return
		}
	} else {
		overrideAction = strings.TrimSpace(string(data))
		if overrideAction != "" && !isValidDirectiveAction(overrideAction) {
			errMsg := fmt.Sprintf("unsupported action in IMAGE_FUNNEL_ACTION file: %q", overrideAction)
			r.logger.Error(errMsg, zap.String("hook_id", task.HookID))
			task.resultChan <- hookExecutionResult{Error: fmt.Errorf("%s", errMsg), Stdout: stdoutStr, Stderr: stderrStr}
			return
		}
	}

	task.resultChan <- hookExecutionResult{Action: overrideAction, Stdout: stdoutStr, Stderr: stderrStr}
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