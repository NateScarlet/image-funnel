package hook

import (
	"bufio"
	"bytes"
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

// autocompleteSuggestionRaw JSONL / JSON-RPC result 建议项解析结构
type autocompleteSuggestionRaw struct {
	Text        string `json:"text"`
	DisplayText string `json:"displayText"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Style       string `json:"style,omitempty"`
}

// convertSuggestion 将建议项原始结构转换为领域值对象
func convertSuggestion(raw autocompleteSuggestionRaw) *hook.AutocompleteSuggestion {
	displayText := raw.DisplayText
	if displayText == "" {
		displayText = raw.Text
	}
	return &hook.AutocompleteSuggestion{
		Text:        raw.Text,
		DisplayText: displayText,
		Description: raw.Description,
		Type:        raw.Type,
		Style:       raw.Style,
	}
}

// autocompleteTrigger 指令参数自动补全触发路径：
// 按 protocol 分流到常驻 JSON-RPC 复用通道或单次执行通道，两者对外都只产出建议列表。
type autocompleteTrigger struct {
	rootDir  string
	hooksDir string
	config   configSource
	env      *envBuilder
	notes    *noteContext
	process  processRunner
	pool     *jsonrpcPool
	timeout  time.Duration
	logger   *zap.Logger

	// ctx 常驻补全进程的生命周期：随应用退出被取消，进程树随之回收
	ctx context.Context
}

func newAutocompleteTrigger(
	ctx context.Context,
	rootDir string,
	hooksDir string,
	config configSource,
	env *envBuilder,
	notes *noteContext,
	process processRunner,
	idleTimeout time.Duration,
	timeout time.Duration,
	logger *zap.Logger,
) *autocompleteTrigger {
	t := &autocompleteTrigger{
		ctx:      ctx,
		rootDir:  rootDir,
		hooksDir: hooksDir,
		config:   config,
		env:      env,
		notes:    notes,
		process:  process,
		timeout:  timeout,
		logger:   logger,
	}
	t.pool = newJSONRPCPool(logger, idleTimeout, t.spawn)
	return t
}

// Close 随应用退出：终止所有常驻补全进程
func (t *autocompleteTrigger) Close() {
	t.pool.Close()
}

// Complete 返回当前输入位置的建议项；noteRelPath 为空串表示「没有关联笔记」
func (t *autocompleteTrigger) Complete(ctx context.Context, hookID scalar.ID, noteRelPath string, linePrefix string, query string) ([]*hook.AutocompleteSuggestion, error) {
	hooks, err := t.config.Load()
	if err != nil {
		return nil, err
	}
	target, err := resolveHookConfig(hooks, hookID)
	if err != nil {
		return nil, err
	}
	if target.Directive == nil || target.Directive.Autocomplete == nil || target.Directive.Autocomplete.Command == "" {
		return nil, nil
	}

	// 构建笔记和配套图片的路径与 ID
	var noteAbsPath string
	var imageIDs []string
	var imagePaths []string
	if noteRelPath != "" {
		notePath, err := relpath.New(t.rootDir, noteRelPath)
		if err != nil {
			return nil, fmt.Errorf("invalid note path for autocomplete: %w", err)
		}
		noteAbsPath = notePath.Abs(t.rootDir)

		imgPath, ok, err := t.notes.AssociatedImagePath(notePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve associated image for autocomplete: %w", err)
		}
		if ok {
			img, err := t.notes.Image(ctx, imgPath)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve associated image for autocomplete: %w", err)
			}
			imagePaths = []string{imgPath.Abs(t.rootDir)}
			if img != nil {
				imageIDs = []string{img.ID().String()}
			}
		}
	}

	// 按 protocol 分流：json-rpc 走常驻复用通道；缺省保持单次执行（向后兼容，现有脚本零改动）
	if target.Directive.Autocomplete.Protocol == "json-rpc" {
		return t.viaJSONRPC(ctx, target, linePrefix, query, imageIDs, imagePaths, noteAbsPath)
	}
	return t.viaSingleRun(ctx, target, linePrefix, query, imageIDs, imagePaths, noteAbsPath)
}

// viaSingleRun 单次执行自动补全脚本：每次请求 spawn 新进程（默认行为）
func (t *autocompleteTrigger) viaSingleRun(ctx context.Context, target hookConfig, linePrefix, query string, imageIDs, imagePaths []string, noteAbsPath string) ([]*hook.AutocompleteSuggestion, error) {
	argv, err := parseCommandArgs(target.Directive.Autocomplete.Command)
	if err != nil {
		return nil, fmt.Errorf("invalid autocomplete command: %w", err)
	}

	env, err := t.buildEnv(ctx, target, linePrefix, query, imageIDs, imagePaths, noteAbsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build autocomplete env: %w", err)
	}

	out, err := runOnceScript(ctx, t.process, t.logger, t.hooksDir, argv, env, "autocomplete script")
	if err != nil {
		return nil, err
	}
	return parseAutocompleteJSONL([]byte(out.Stdout))
}

// buildEnv 构建自动补全脚本的环境变量：基础上下文 + bash 风格的补全位置解析
func (t *autocompleteTrigger) buildEnv(ctx context.Context, target hookConfig, linePrefix, query string, imageIDs, imagePaths []string, noteAbsPath string) ([]string, error) {
	env, err := t.env.Build(ctx, envRequest{
		HookID:      target.ID,
		HookName:    target.Name,
		TriggerName: "autocomplete",
		ImageIDs:    imageIDs,
		ImagePaths:  imagePaths,
		NoteAbsPath: noteAbsPath,
		CustomEnv:   target.Env,
	})
	if err != nil {
		return nil, err
	}

	cwords, cwordIdx, prevWord := parseLineContext(linePrefix, query)
	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS="+mustJSON(cwords))
	env = append(env, fmt.Sprintf("IMAGE_FUNNEL_AUTOCOMPLETE_CWORD_IDX=%d", cwordIdx))
	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD="+prevWord)
	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX="+linePrefix)
	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY="+query)

	return env, nil
}

// parseLineContext 将 raw linePrefix 解析为类似 bash COMP_WORDS / COMP_CWORD 的上下文，
// 使脚本无需自行实现参数位置解析。
func parseLineContext(linePrefix, query string) (cwords []string, cwordIdx int, prevWord string) {
	completedPrefix := linePrefix
	if query != "" {
		completedPrefix = strings.TrimSuffix(linePrefix, query)
	}

	cwords = strings.Fields(completedPrefix)
	// 光标所在位置恒为已解析出的词数：query 非空时它是被 query 替换掉的那个词的末尾，
	// 为空时（行尾空格或整行空白）则是下一个待补全的词
	cwordIdx = len(cwords)
	if cwordIdx > 0 {
		prevWord = cwords[cwordIdx-1]
	}

	return
}

func parseAutocompleteJSONL(data []byte) ([]*hook.AutocompleteSuggestion, error) {
	var suggestions []*hook.AutocompleteSuggestion
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw autocompleteSuggestionRaw
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("failed to parse autocomplete suggestion line: %w, line: %s", err, line)
		}
		suggestions = append(suggestions, convertSuggestion(raw))
	}
	return suggestions, scanner.Err()
}
