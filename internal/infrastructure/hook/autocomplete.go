package hook

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"main/internal/apperror"
	"main/internal/domain/hook"
	"main/internal/scalar"

	"go.uber.org/zap"
)

// autocompleteSuggestionRaw JSONL 行解析结构
type autocompleteSuggestionRaw struct {
	Text        string `json:"text"`
	DisplayText string `json:"displayText"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Style       string `json:"style,omitempty"`
}

// Autocomplete 通过钩子脚本获取指令参数自动完成建议
func (r *Runner) Autocomplete(ctx context.Context, hookID scalar.ID, noteRelPath string, linePrefix string, query string) ([]*hook.AutocompleteSuggestion, error) {
	hooks, err := r.loadHooks()
	if err != nil {
		return nil, err
	}

	var targetHook *hookConfig
	for _, h := range hooks {
		domH := hook.FromRepository(h.ID, h.Name, h.Description, h.On.ImageDispatch != nil, h.On.NoteDispatch != nil, nil, false, false)
		if domH.ID() == hookID {
			targetHook = &h
			break
		}
	}

	if targetHook == nil {
		return nil, fmt.Errorf("hook %s not found", hookID.String())
	}

	if targetHook.Directive == nil || targetHook.Directive.Autocomplete == nil || targetHook.Directive.Autocomplete.Command == "" {
		return nil, nil
	}

	// 构建笔记和配套图片的路径与 ID
	var noteAbsPath string
	var imageIDs []string
	var imagePaths []string
	if noteRelPath != "" {
		noteAbsPath = filepath.Join(r.rootDir, noteRelPath)
	}
	if imgRelPath, ok := associatedImageRelPath(noteRelPath); ok {
		imagePaths = []string{filepath.Join(r.rootDir, imgRelPath)}
		img, err := apperror.IgnoreNotFound(r.imgRepo.Get(ctx, imgRelPath))
		if err != nil {
			return nil, fmt.Errorf("failed to resolve associated image for autocomplete: %w", err)
		}
		if img != nil {
			imageIDs = []string{img.ID().String()}
		}
	}

	cmd := newHookCmd(ctx, targetHook.Directive.Autocomplete.Command)
	cmd.Dir = r.hooksDir

	env, err := r.buildAutocompleteEnv(ctx, targetHook, linePrefix, query, imageIDs, imagePaths, noteAbsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build autocomplete env: %w", err)
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	r.logger.Debug("will execute autocomplete command",
		zap.String("hook_id", targetHook.ID),
		zap.String("command", targetHook.Directive.Autocomplete.Command),
		zap.String("line_prefix", linePrefix),
		zap.String("query", query),
	)

	err = cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			r.logger.Debug("autocomplete command canceled by context",
				zap.String("hook_id", targetHook.ID),
				zap.Error(ctx.Err()),
			)
			return nil, ctx.Err()
		}
		r.logger.Warn("autocomplete command failed",
			zap.String("hook_id", targetHook.ID),
			zap.Error(err),
			zap.String("stderr", stderr.String()),
		)
		return nil, fmt.Errorf("autocomplete script failed: %w, stderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	if stderr.Len() > 0 {
		r.logger.Debug("autocomplete stderr",
			zap.String("hook_id", targetHook.ID),
			zap.String("stderr", stderr.String()),
		)
	}

	return parseAutocompleteJSONL(stdout.Bytes())
}

// parseLineContext 将 raw linePrefix 解析为类似 bash COMP_WORDS / COMP_CWORD 的上下文，
// 使脚本无需自行实现参数位置解析。
func parseLineContext(linePrefix, query string) (cwords []string, cwordIdx int, prevWord string) {
	hasTrailingSpace := strings.HasSuffix(linePrefix, " ")

	completedPrefix := linePrefix
	if query != "" {
		completedPrefix = strings.TrimSuffix(linePrefix, query)
	}

	cwords = strings.Fields(completedPrefix)

	if hasTrailingSpace && query == "" {
		cwordIdx = len(cwords)
	} else {
		cwordIdx = len(cwords)
	}

	if cwordIdx > 0 {
		prevWord = cwords[cwordIdx-1]
	}

	return
}

func (r *Runner) buildAutocompleteEnv(ctx context.Context, targetHook *hookConfig, linePrefix, query string, imageIDs, imagePaths []string, noteAbsPath string) ([]string, error) {
	env, err := r.buildBaseEnv(ctx, targetHook.ID, targetHook.Name, "autocomplete", imageIDs, imagePaths, noteAbsPath, targetHook.Env, "", "")
	if err != nil {
		return nil, err
	}

	// 类似 bash COMP_WORDS / COMP_CWORD / prev word 的解析上下文
	cwords, cwordIdx, prevWord := parseLineContext(linePrefix, query)
	cwordsJSON := mustJSON(cwords)
	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS="+string(cwordsJSON))
	env = append(env, fmt.Sprintf("IMAGE_FUNNEL_AUTOCOMPLETE_CWORD_IDX=%d", cwordIdx))
	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD="+prevWord)

	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX="+linePrefix)
	env = append(env, "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY="+query)

	return env, nil
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
		displayText := raw.DisplayText
		if displayText == "" {
			displayText = raw.Text
		}
		suggestions = append(suggestions, &hook.AutocompleteSuggestion{
			Text:        raw.Text,
			DisplayText: displayText,
			Description: raw.Description,
			Type:        raw.Type,
			Style:       raw.Style,
		})
	}
	return suggestions, scanner.Err()
}
