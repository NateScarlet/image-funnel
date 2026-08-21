package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"main/internal/domain/hook"
	"main/internal/scalar"

	"go.uber.org/zap"
)

// copyContentRaw 复制增强脚本 stdout 的单行 JSON 信封解析结构
type copyContentRaw struct {
	Content     string `json:"content"`
	Description string `json:"description"`
}

// CopyContent 执行复制增强钩子脚本，获取应写入剪贴板的内容。
// 全局至多一个声明 [copy] 能力的钩子：
//   - 0 个 → 返回 nil（调用方降级为复制文件/路径）
//   - >1 个 → 报错并列出冲突的 hook id，组合逻辑由脚本自身负责
func (r *Runner) CopyContent(ctx context.Context, imageID scalar.ID, imageRelPath string) (*hook.CopyContent, error) {
	hooks, err := r.loadHooks()
	if err != nil {
		return nil, err
	}

	var target *hookConfig
	var conflictIDs []string
	for i := range hooks {
		if hooks[i].Copy == nil {
			continue
		}
		conflictIDs = append(conflictIDs, hooks[i].ID)
		if target == nil {
			target = &hooks[i]
		}
	}
	switch {
	case len(conflictIDs) == 0:
		return nil, nil
	case len(conflictIDs) > 1:
		return nil, fmt.Errorf("multiple [copy] hooks configured: %s; only one is allowed",
			strings.Join(conflictIDs, ", "))
	}

	content, err := r.executeCopyScript(ctx, *target, imageID, imageRelPath)
	if err != nil {
		return nil, err
	}
	if content != nil {
		content.Description = strings.TrimSpace(content.Description)
	}
	return content, nil
}

// executeCopyScript 单次 spawn 复制增强脚本并按契约解析 stdout（结构复用 autocomplete 单次执行管道）
func (r *Runner) executeCopyScript(ctx context.Context, target hookConfig, imageID scalar.ID, imageRelPath string) (*hook.CopyContent, error) {
	argv, err := parseCommandArgs(target.Command)
	if err != nil {
		return nil, fmt.Errorf("invalid copy command: %w", err)
	}
	cmd := newHookCmd(ctx, argv)
	cmd.Dir = r.hooksDir

	env, err := r.buildBaseEnv(ctx, target.ID, target.Name, "image_copy",
		[]string{imageID.String()}, []string{filepath.Join(r.rootDir, imageRelPath)},
		"", target.Env, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to build copy env: %w", err)
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	r.logger.Debug("will execute copy content script",
		zap.String("hook_id", target.ID),
		zap.String("command", target.Command),
		zap.String("image_id", imageID.String()),
	)

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if err != nil {
		if ctx.Err() != nil {
			r.logger.Debug("copy content script canceled by context",
				zap.String("hook_id", target.ID),
				zap.Error(ctx.Err()),
			)
			return nil, ctx.Err()
		}
		r.logger.Warn("copy content script failed",
			zap.String("hook_id", target.ID),
			zap.Duration("duration", duration),
			zap.Error(err),
			zap.String("stderr", stderrStr),
		)
		return nil, fmt.Errorf("copy enhancement script failed: %w, stderr: %s",
			err, strings.TrimSpace(stderrStr))
	}

	if stderrStr != "" {
		r.logger.Debug("copy content script stderr",
			zap.String("hook_id", target.ID),
			zap.String("stderr", stderrStr),
		)
	}

	r.logger.Info("did execute copy content script",
		zap.String("hook_id", target.ID),
		zap.Duration("duration", duration),
	)
	return parseCopyEnvelope(stdoutStr)
}

// parseCopyEnvelope 对脚本 stdout 做唯一一次边界校验：
// 空 stdout 视为「不适用」返回 nil；非 JSON 或 content 为空均属协议违约，显式报错
func parseCopyEnvelope(stdout string) (*hook.CopyContent, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, nil
	}

	var raw copyContentRaw
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, fmt.Errorf(
			"copy enhancement script must output a single JSON envelope, got invalid JSON: %w, stdout: %s",
			err, trimmed)
	}
	if raw.Content == "" {
		return nil, fmt.Errorf(
			"copy enhancement script envelope has empty content field; output nothing instead to mark not applicable")
	}
	return &hook.CopyContent{Content: raw.Content, Description: raw.Description}, nil
}
