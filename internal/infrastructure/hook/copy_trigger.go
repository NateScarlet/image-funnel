package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"main/internal/domain/hook"
	"main/internal/domain/relpath"
	"main/internal/scalar"

	"go.uber.org/zap"
)

// copyContentRaw 复制增强脚本 stdout 的单行 JSON 信封解析结构
type copyContentRaw struct {
	Content     string `json:"content"`
	Description string `json:"description"`
}

// copyTrigger 复制增强触发路径（image_copy）：
// 请求期同步执行单个脚本，取得应写入剪贴板的内容。它不是事件触发器，而是
// 用户复制图片时的即时内容提供者。
type copyTrigger struct {
	rootDir  string
	hooksDir string
	config   configSource
	env      *envBuilder
	process  processRunner
	logger   *zap.Logger
}

func newCopyTrigger(rootDir, hooksDir string, config configSource, env *envBuilder, process processRunner, logger *zap.Logger) *copyTrigger {
	return &copyTrigger{rootDir: rootDir, hooksDir: hooksDir, config: config, env: env, process: process, logger: logger}
}

// Content 执行复制增强钩子脚本，获取应写入剪贴板的内容。
// 全局至多一个声明 [copy] 能力的钩子：
//   - 0 个 → 返回 nil（调用方降级为复制文件/路径）
//   - >1 个 → 报错并列出冲突的 hook id，组合逻辑由脚本自身负责
func (t *copyTrigger) Content(ctx context.Context, imageID scalar.ID, imageRelPath relpath.Path) (*hook.CopyContent, error) {
	hooks, err := t.config.Load()
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

	content, err := t.run(ctx, *target, imageID, imageRelPath)
	if err != nil {
		return nil, err
	}
	if content != nil {
		content.Description = strings.TrimSpace(content.Description)
	}
	return content, nil
}

// run 单次 spawn 复制增强脚本并按契约解析 stdout
func (t *copyTrigger) run(ctx context.Context, target hookConfig, imageID scalar.ID, imageRelPath relpath.Path) (*hook.CopyContent, error) {
	env, err := t.env.Build(ctx, envRequest{
		HookID:      target.ID,
		HookName:    target.Name,
		TriggerName: "image_copy",
		ImageIDs:    []string{imageID.String()},
		ImagePaths:  []string{imageRelPath.Abs(t.rootDir)},
		CustomEnv:   target.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build copy env: %w", err)
	}

	argv, err := parseCommandArgs(target.Command)
	if err != nil {
		return nil, fmt.Errorf("invalid copy command: %w", err)
	}

	start := time.Now()
	out, err := runOnceScript(ctx, t.process, t.logger, t.hooksDir, argv, env, "copy enhancement script")
	duration := time.Since(start)
	if err != nil {
		return nil, err
	}

	t.logger.Info("did execute copy enhancement script", zap.Duration("duration", duration))
	return parseCopyEnvelope(out.Stdout)
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
