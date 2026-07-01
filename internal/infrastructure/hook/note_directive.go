package hook

import (
	"context"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	domhook "main/internal/domain/hook"
	"main/internal/scalar"

	"go.uber.org/zap"
)

var fastCheckReg = regexp.MustCompile(`(?m)^[ \t]*/[a-zA-Z0-9_-]+`)

var directiveReg = regexp.MustCompile(`(?m)^[ \t]*/([a-zA-Z0-9_-]+)(?:\s+([^\r\n]*))?\r?\n?`)

func (r *Runner) executeNoteDirectives(ctx context.Context, dirID scalar.ID, dirRelPath string, relPath string, content string, triggerType string, filterHookID scalar.ID) (bool, error) {
	fastCheck := fastCheckReg.MatchString(content)
	contentSummary := content
	if len(contentSummary) > 500 {
		contentSummary = contentSummary[:500] + "...(truncated)"
	}
	r.logger.Debug("executeNoteDirectives start",
		zap.String("relPath", relPath),
		zap.String("triggerType", triggerType),
		zap.String("filterHookID", filterHookID.String()),
		zap.Bool("fastCheck", fastCheck),
		zap.String("contentHead", contentSummary),
	)
	// 快速粗筛：如果文本内没有任何可能匹配指令的行，直接退出，零磁盘 I/O 浪费
	if !fastCheck {
		return false, nil
	}

	hooks, err := r.loadHooks()
	if err != nil {
		return false, err
	}

	hookMap := make(map[string]hookConfig)
	var registeredDirectives []string
	for _, h := range hooks {
		if h.Directive != nil && h.Directive.Name != "" {
			if existing, ok := hookMap[h.Directive.Name]; ok {
				r.logger.Error("duplicate directive name detected, the later hook will override the earlier one",
					zap.String("directive", h.Directive.Name),
					zap.String("existing_hook_id", existing.ID),
					zap.String("new_hook_id", h.ID),
				)
			}
			hookMap[h.Directive.Name] = h
			registeredDirectives = append(registeredDirectives, h.Directive.Name)
		}
	}
	r.logger.Debug("executeNoteDirectives hookMap built", zap.Strings("directives", registeredDirectives))

	// 1. 获取临时的 hook-run-id
	runID := getHookRunID(content)
	var isKnown bool
	if runID != "" {
		r.muTasks.Lock()
		_, isKnown = r.activeTasks[runID]
		r.muTasks.Unlock()
	}

	noteAbsPath := filepath.Join(r.rootDir, relPath)

	// 1b. 若是有已知且正在运行的 ID，则进行快速返回或执行后置迟到擦除
	if runID != "" && isKnown {
		isBefore3, failedDirectives := func() (bool, map[string]bool) {
			r.muTasks.Lock()
			defer r.muTasks.Unlock()
			task := r.activeTasks[runID]
			if task.phase == phaseBefore3 {
				task.paths[noteAbsPath] = struct{}{}
				return true, nil
			}
			fd := make(map[string]bool)
			if task.failedDirectives != nil {
				maps.Copy(fd, task.failedDirectives)
			}
			return false, fd
		}()

		if isBefore3 {
			r.logger.Debug("associated new path with active hook-run-id", zap.String("path", relPath), zap.String("run_id", runID))
			return false, nil
		}

		// 1b2. 若步骤 3 已完毕，执行后置迟到擦除
		r.logger.Debug("executing late path cleanup for known hook-run-id", zap.String("path", relPath), zap.String("run_id", runID))
		r.postProcessNoteDirectives(ctx, noteAbsPath, runID, triggerType, hookMap, failedDirectives)
		return false, nil
	}

	// 2. 对于新启动的指令流程，我们先匹配提取 pending 任务
	type pendingHook struct {
		config      hookConfig
		triggerType string
		events      []hookEvent
		args        []string
		relPath     string
		dirID       string
		dirRelPath  string
		action      string    // 已解析的操作（COMMENT_OUT/REMOVE/KEEP），在钩子执行完成后设置
		stdout      string    // 脚本标准输出
		stderr      string    // 脚本标准错误输出
		executedAt  time.Time // 脚本执行时间
	}

	var pending []pendingHook

	// 通过正则遍历提取指令到 pending 列表中，内容不做任何抹除以供 Hook 脚本使用
	matches := directiveReg.FindAllStringSubmatch(content, -1)
	r.logger.Debug("executeNoteDirectives regexp matched lines", zap.Int("count", len(matches)))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		matchedLine := match[0]
		cmdName := match[1]
		cmdArgs := ""
		if len(match) > 2 {
			cmdArgs = strings.TrimSpace(match[2])
		}

		r.logger.Debug("executeNoteDirectives processing line",
			zap.String("matchedLine", strings.TrimSpace(matchedLine)),
			zap.String("cmdName", cmdName),
			zap.String("cmdArgs", cmdArgs),
		)

		hookConfig, ok := hookMap[cmdName]
		if !ok {
			r.logger.Debug("executeNoteDirectives cmdName not found in hookMap", zap.String("cmdName", cmdName))
			continue
		}

		// 根据触发类型进行筛选
		filterPassed := false
		switch triggerType {
		case "post_update_note":
			if hookConfig.On.PostUpdateNote == nil {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostUpdateNote is nil")
				continue
			}
			// 若设定为 true，说明它是无条件静默触发，指令阶段不重复触发
			if hookConfig.On.PostUpdateNote.IgnoreDirective {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostUpdateNote.IgnoreDirective is true")
				continue
			}
			filterPassed = true
		case "post_commit_session":
			if hookConfig.On.PostCommitSession == nil || hookConfig.On.PostCommitSession.NoteScan == nil {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostCommitSession.NoteScan is nil")
				continue
			}
			// 若设定为 true，说明它是无条件静默扫描，指令阶段不重复触发
			if hookConfig.On.PostCommitSession.NoteScan.IgnoreDirective {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostCommitSession.NoteScan.IgnoreDirective is true")
				continue
			}
			filterPassed = true
		case "note_dispatch":
			if hookConfig.On.NoteDispatch == nil {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.NoteDispatch is nil")
				continue
			}
			filterPassed = true
		default:
			r.logger.Debug("executeNoteDirectives trigger filter skipped: unknown triggerType")
			continue
		}

		r.logger.Debug("executeNoteDirectives trigger filter passed", zap.Bool("passed", filterPassed))

		if !filterHookID.IsZero() {
			domH := domhook.FromRepository(hookConfig.ID, hookConfig.Name, hookConfig.Description, hookConfig.On.ImageDispatch != nil, hookConfig.On.NoteDispatch != nil, nil, false, false)
			idMatch := domH.ID() == filterHookID
			r.logger.Debug("executeNoteDirectives hook ID filter check",
				zap.String("domH.ID", domH.ID().String()),
				zap.String("filterHookID", filterHookID.String()),
				zap.Bool("matched", idMatch),
			)
			if !idMatch {
				continue
			}
		}

		// 寻找配套的图片
		evs, err := r.findAssociatedImageEvents(ctx, relPath)
		if err != nil {
			r.logger.Error("failed to get associated image for note directive", zap.String("note_path", relPath), zap.Error(err))
			continue
		}

		var args []string
		if cmdArgs != "" {
			args = splitArgs(cmdArgs)
		}

		r.logger.Debug("executeNoteDirectives appending pending directive",
			zap.String("hookID", hookConfig.ID),
			zap.Strings("args", args),
		)

		// 将此任务入队暂存，暂不执行
		pending = append(pending, pendingHook{
			config:      hookConfig,
			triggerType: triggerType,
			events:      evs,
			args:        args,
			relPath:     relPath,
			dirID:       dirID.String(),
			dirRelPath:  dirRelPath,
		})
	}

	r.logger.Debug("executeNoteDirectives loop finished", zap.Int("pendingCount", len(pending)))
	if len(pending) == 0 {
		return false, nil
	}

	// 3. 确定有 pending 指令要运行，我们才在 frontmatter 设置新随机 runID 并写入
	runID = fmt.Sprintf("run_%019d_%06d", time.Now().UnixNano(), rand.Intn(1000000))
	newContent := setHookRunID(content, runID)
	if newContent != content {
		newContentBytes := []byte(newContent)
		if err := r.writeFileWithIgnore(noteAbsPath, newContentBytes, 0644); err != nil {
			r.logger.Error("failed to write hook-run-id to note", zap.String("path", relPath), zap.Error(err))
			return false, err
		}
		content = newContent
	}

	// 在内存中注册防重入路径
	r.muTasks.Lock()
	task, exists := r.activeTasks[runID]
	if !exists {
		task = &activeTask{
			phase: phaseBefore3,
			paths: make(map[string]struct{}),
		}
		r.activeTasks[runID] = task
	}
	task.paths[noteAbsPath] = struct{}{}
	r.muTasks.Unlock()

	defer func() {
		time.AfterFunc(10*time.Second, func() {
			r.muTasks.Lock()
			delete(r.activeTasks, runID)
			r.muTasks.Unlock()
		})
	}()

	// 4. 执行斜杠指令 (注入唯一的 hook-run-id)
	for i, p := range pending {
		action, stdout, stderr, err := r.executeHookSync(p.config, p.triggerType, p.events, p.args, p.relPath, p.dirID, p.dirRelPath, runID)
		if err != nil {
			r.logger.Error("failed to execute hook for directive", zap.String("hook_id", p.config.ID), zap.Error(err))
		}
		pending[i].action = action
		pending[i].stdout = stdout
		pending[i].stderr = stderr
		pending[i].executedAt = time.Now()
	}

	// 5. 执行完成后，在 activeTask 关联的所有路径上执行擦除
	r.muTasks.Lock()
	r.activeTasks[runID].phase = phaseAfter3
	failedDirectives := make(map[string]bool)
	for _, p := range pending {
		failedDirectives[p.config.Directive.Name] = p.action == p.config.Directive.OnFailAction
	}
	r.activeTasks[runID].failedDirectives = failedDirectives
	var pathsToProcess []string
	for p := range r.activeTasks[runID].paths {
		pathsToProcess = append(pathsToProcess, p)
	}
	r.muTasks.Unlock()

	for _, p := range pathsToProcess {
		contentBytes, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			r.logger.Error("failed to read file during directive cleanup", zap.String("path", p), zap.Error(err))
			continue
		}

		fileContent := string(contentBytes)
		currentID := getHookRunID(fileContent)
		if currentID != runID {
			continue // 3b. 如果不一致，说明该路径已被覆盖，忽略之
		}

		// 3a. 擦除指令和 hook-run-id 并写入文件
		idx := 0
		finalContent := directiveReg.ReplaceAllStringFunc(fileContent, func(matchedLine string) string {
			if idx >= len(pending) {
				return matchedLine
			}
			pTask := pending[idx]
			idx++

			return applyDirectiveAction(pTask.action, matchedLine, pTask.stdout, pTask.stderr, pTask.executedAt)
		})

		// 擦除 hook-run-id
		finalContent = removeHookRunID(finalContent)

		if finalContent != fileContent {
			if finalContent == "" {
				// 处理后的内容为空，直接删除文件
				if err := os.Remove(p); err != nil {
					r.logger.Error("failed to delete empty note file after directive cleanup", zap.String("path", p), zap.Error(err))
				}
				continue
			}
			// 在写回磁盘前，计算 xxhash 并注册为忽略事件，自防循环
			finalContentBytes := []byte(finalContent)
			if err := r.writeFileWithIgnore(p, finalContentBytes, 0644); err != nil {
				r.logger.Error("failed to write clean content to note file during cleanup", zap.String("path", p), zap.Error(err))
			}
		}
	}

	return true, nil
}

func (r *Runner) postProcessNoteDirectives(ctx context.Context, absPath string, runID string, triggerType string, hookMap map[string]hookConfig, failedDirectives map[string]bool) {
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		r.logger.Error("failed to read file during late task cleanup", zap.String("path", absPath), zap.Error(err))
		return
	}

	fileContent := string(contentBytes)
	currentID := getHookRunID(fileContent)
	if currentID != runID {
		return
	}

	finalContent := directiveReg.ReplaceAllStringFunc(fileContent, func(matchedLine string) string {
		matches := directiveReg.FindStringSubmatch(matchedLine)
		if len(matches) < 2 {
			return matchedLine
		}
		cmdName := matches[1]
		hookConfig, ok := hookMap[cmdName]
		if !ok {
			return matchedLine
		}

		action := hookConfig.Directive.OnSuccessAction
		if failedDirectives != nil && failedDirectives[cmdName] {
			action = hookConfig.Directive.OnFailAction
		}
		return applyDirectiveAction(action, matchedLine, "", "", time.Time{})
	})

	finalContent = removeHookRunID(finalContent)

	if finalContent != fileContent {
		if finalContent == "" {
			// 处理后的内容为空，直接删除文件
			if err := os.Remove(absPath); err != nil {
				r.logger.Error("failed to delete empty note file during late cleanup", zap.String("path", absPath), zap.Error(err))
			}
			return
		}
		finalContentBytes := []byte(finalContent)
		if err := r.writeFileWithIgnore(absPath, finalContentBytes, 0644); err != nil {
			r.logger.Error("failed to write clean content to note file during late cleanup", zap.String("path", absPath), zap.Error(err))
		}
	}
}