package hook

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/util"

	"go.uber.org/zap"
)

var fastCheckReg = regexp.MustCompile(`(?m)^[ \t]*/[a-zA-Z0-9_-]+`)

var directiveReg = regexp.MustCompile(`(?m)^[ \t]*/([a-zA-Z0-9_-]+)(?:\s+([^\r\n]*))?\r?\n?`)

// isFileNotFoundError 检查错误是否与文件不存在相关。
// 通过 errors.Is 检查底层的文件系统错误（如 cmd.exe 启动时的路径解析失败）。
func isFileNotFoundError(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

// #region 指令运行的阶段跟踪

type taskPhase int

const (
	phaseBefore3 taskPhase = iota
	phaseAfter3
)

// directiveIdentity 一条指令的身份：指令行原文 + 该原文在笔记中出现的次序。
// 原文相同的多条指令靠出现次序区分，因此结果找回不依赖「两次正则匹配的数量与顺序一致」。
type directiveIdentity struct {
	line    string
	ordinal int
}

// directiveResult 一条指令的执行结果，供擦除阶段应用动作与生成结果提示块
type directiveResult struct {
	action     string    // 已解析的操作（REMOVE/KEEP/COMMENT_OUT）
	stdout     string    // 脚本标准输出
	stderr     string    // 脚本标准错误输出
	executedAt time.Time // 脚本执行时间
}

// identityScanner 按「原文 + 出现次序」为逐条匹配到的指令行编号。
// 收集与擦除两个阶段共用同一口径，保证同一条指令在两个阶段得到相同身份。
type identityScanner struct {
	counts map[string]int
}

func newIdentityScanner() *identityScanner {
	return &identityScanner{counts: make(map[string]int)}
}

// identify 返回指令行原文的身份，并推进该原文的出现计数
func (s *identityScanner) identify(rawLine string) directiveIdentity {
	line := strings.TrimSpace(rawLine)
	ident := directiveIdentity{line: line, ordinal: s.counts[line]}
	s.counts[line]++
	return ident
}

// activeTask 一次指令运行的登记：run_id 关联的所有笔记路径与执行阶段，
// 并按指令身份保存完整执行结果，供迟到的文件变更事件做后置擦除
type activeTask struct {
	phase   taskPhase
	paths   map[string]struct{}
	results map[directiveIdentity]directiveResult
}

// #endregion

// noteRewriteRequest 一次笔记指令改写的输入
type noteRewriteRequest struct {
	// Dir 笔记所在目录实体，作为钩子脚本的目录上下文
	Dir *directory.Directory
	// NoteRelPath 笔记的根相对路径
	NoteRelPath relpath.Path
	// Content 笔记当前文本，内容不做预擦除以供钩子脚本读取指令原文
	Content string
	// TriggerName 触发来源，决定指令适用的触发器过滤条件
	TriggerName string
	// FilterHookID 非零时只处理该 hook 的指令（手动派发场景）
	FilterHookID scalar.ID
}

// noteRewriter 笔记指令改写器：包住笔记指令的解析与文本擦除。
// 一次改写把 run_id 写进 frontmatter → 执行全部指令 → 在关联路径上擦除指令行，
// 写入动作经防重入守卫登记，避免自身写入触发的事件再次进入本改写器。
type noteRewriter struct {
	config   configSource
	exec     *hookExecutor
	notes    *noteContext
	guard    *writeGuard
	notifier *hookNotifier
	rootDir  string
	logger   *zap.Logger

	mu    sync.Mutex
	tasks map[string]*activeTask
}

func newNoteRewriter(
	config configSource,
	exec *hookExecutor,
	notes *noteContext,
	guard *writeGuard,
	notifier *hookNotifier,
	rootDir string,
	logger *zap.Logger,
) *noteRewriter {
	return &noteRewriter{
		config:   config,
		exec:     exec,
		notes:    notes,
		guard:    guard,
		notifier: notifier,
		rootDir:  rootDir,
		logger:   logger,
		tasks:    make(map[string]*activeTask),
	}
}

// Rewrite 解析笔记中的斜杠指令、执行对应钩子并擦除指令行。
// 返回是否执行过指令：false 表示本次没有任何指令，调用方应回退到无指令的处理路径。
func (w *noteRewriter) Rewrite(ctx context.Context, req noteRewriteRequest) (bool, error) {
	fastCheck := fastCheckReg.MatchString(req.Content)
	contentSummary := req.Content
	if len(contentSummary) > 500 {
		contentSummary = contentSummary[:500] + "...(truncated)"
	}
	w.logger.Debug("note rewrite start",
		zap.String("relPath", req.NoteRelPath.String()),
		zap.String("triggerName", req.TriggerName),
		zap.String("filterHookID", req.FilterHookID.String()),
		zap.Bool("fastCheck", fastCheck),
		zap.String("contentHead", contentSummary),
	)
	// 快速粗筛：如果文本内没有任何可能匹配指令的行，直接退出，零磁盘 I/O 浪费
	if !fastCheck {
		return false, nil
	}

	// 1. 获取临时的 hook-run-id
	runID := getHookRunID(req.Content)
	var isKnown bool
	if runID != "" {
		w.mu.Lock()
		_, isKnown = w.tasks[runID]
		w.mu.Unlock()
		if !isKnown {
			// 无法识别的运行标记（崩溃后残留、用户手动重放等）按全新运行处理
			w.logger.Debug("unrecognized hook-run-id, treating as a fresh run",
				zap.String("run_id", runID),
				zap.String("path", req.NoteRelPath.String()),
			)
		}
	}

	noteAbsPath := req.NoteRelPath.Abs(w.rootDir)

	// 1a. 检查笔记文件是否存在，如果已被 retention 删除则静默跳过
	if _, err := os.Stat(noteAbsPath); os.IsNotExist(err) {
		return false, nil
	}

	// 1b. 若是有已知且正在运行的 ID，则进行快速返回或执行后置迟到擦除
	if runID != "" && isKnown {
		isBefore3, results := func() (bool, map[directiveIdentity]directiveResult) {
			w.mu.Lock()
			defer w.mu.Unlock()
			task := w.tasks[runID]
			if task.phase == phaseBefore3 {
				task.paths[noteAbsPath] = struct{}{}
				return true, nil
			}
			copied := make(map[directiveIdentity]directiveResult, len(task.results))
			maps.Copy(copied, task.results)
			return false, copied
		}()

		if isBefore3 {
			w.logger.Debug("associated new path with active hook-run-id", zap.String("path", req.NoteRelPath.String()), zap.String("run_id", runID))
			return false, nil
		}

		// 1b2. 若步骤 3 已完毕，执行后置迟到擦除
		w.logger.Debug("executing late path cleanup for known hook-run-id", zap.String("path", req.NoteRelPath.String()), zap.String("run_id", runID))
		if err := w.CleanupLate(noteAbsPath, runID, results); err != nil {
			return false, err
		}
		return false, nil
	}

	hookMap, err := w.directiveHooks()
	if err != nil {
		return false, err
	}

	// 2. 对于新启动的指令流程，我们先匹配提取 pending 任务
	pending := w.collectDirectives(ctx, req, hookMap)
	if len(pending) == 0 {
		return false, nil
	}

	// 3. 确定有 pending 指令要运行，我们才在 frontmatter 设置新随机 runID 并写入
	runID = newRunID()
	newContent := setHookRunID(req.Content, runID)
	if newContent != req.Content {
		if err := w.guard.Write(noteAbsPath, []byte(newContent), 0644); err != nil {
			w.logger.Error("failed to write hook-run-id to note", zap.String("path", req.NoteRelPath.String()), zap.Error(err))
			return false, err
		}
	}

	// 在内存中注册防重入路径
	w.registerTask(runID, noteAbsPath)

	defer func() {
		time.AfterFunc(writeIgnoreTTL, func() {
			w.mu.Lock()
			delete(w.tasks, runID)
			w.mu.Unlock()
		})
	}()

	// 4. 执行斜杠指令 (注入唯一的 hook-run-id)
	errB := util.NewErrorsBuilder(len(pending))
	for i, p := range pending {
		if p.syntaxErr != nil {
			// 指令参数引号未闭合等语法错误：不执行钩子，按指令执行失败处理（应用 on_fail_action），
			// 并向用户报告语法错误，避免静默吞字
			w.notifier.Notify(ctx, w.pendingTask(p, runID), p.syntaxErr, "", p.syntaxErr.Error())
			errB.Add(fmt.Errorf("hook %s: invalid directive args: %w", p.config.ID, p.syntaxErr))
			pending[i].result = directiveResult{action: p.config.Directive.OnFailAction, executedAt: time.Now()}
			continue
		}

		res := w.exec.Execute(w.pendingTask(p, runID))
		if res.Err != nil {
			// 仅当 on_fail_action 为 REMOVE 且错误与文件不存在相关时，不传播错误
			if p.config.Directive.OnFailAction == "REMOVE" && isFileNotFoundError(res.Err) {
				// 文件已被删除导致的钩子失败，移除指令行是预期行为
			} else {
				errB.Add(fmt.Errorf("hook %s: %w", p.config.ID, res.Err))
			}
		}
		pending[i].result = directiveResult{
			action:     res.Action,
			stdout:     res.Stdout,
			stderr:     res.Stderr,
			executedAt: time.Now(),
		}
	}

	// 5. 执行完成后，按指令身份汇总结果，在 activeTask 关联的所有路径上执行擦除
	results := make(map[directiveIdentity]directiveResult, len(pending))
	for _, p := range pending {
		results[p.identity] = p.result
	}
	pathsToProcess := w.finishTask(runID, results)
	errB.Add(w.erase(pathsToProcess, runID, results))

	return true, errB.Build()
}

// pendingDirective 一条待执行的笔记指令
type pendingDirective struct {
	config      hookConfig
	triggerName string
	events      []hookEvent
	args        []string
	relPath     relpath.Path
	dir         *directory.Directory
	identity    directiveIdentity // 指令身份：指令行原文 + 原文中的出现次序
	result      directiveResult   // 执行结果，钩子执行完成后填充
	syntaxErr   error             // 指令参数引号未闭合等语法错误，非 nil 时不执行钩子而按失败处理
}

// collectDirectives 从笔记文本中提取匹配当前触发器过滤条件的指令，
// 内容不做任何抹除以供 Hook 脚本使用
func (w *noteRewriter) collectDirectives(ctx context.Context, req noteRewriteRequest, hookMap map[string]hookConfig) []pendingDirective {
	matches := directiveReg.FindAllStringSubmatch(req.Content, -1)
	w.logger.Debug("note rewrite regexp matched lines", zap.Int("count", len(matches)))

	// 身份按所有匹配到的指令行编号，与擦除阶段口径一致；原文相同的行过滤结果必然相同，
	// 因此被过滤跳过的行不会影响其它行的出现次序
	scanner := newIdentityScanner()
	var pending []pendingDirective
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		matchedLine := match[0]
		identity := scanner.identify(matchedLine)
		cmdName := match[1]
		cmdArgs := ""
		if len(match) > 2 {
			cmdArgs = strings.TrimSpace(match[2])
		}

		w.logger.Debug("note rewrite processing line",
			zap.String("matchedLine", strings.TrimSpace(matchedLine)),
			zap.String("cmdName", cmdName),
			zap.String("cmdArgs", cmdArgs),
		)

		config, ok := hookMap[cmdName]
		if !ok {
			w.logger.Debug("note rewrite cmdName not found in hookMap", zap.String("cmdName", cmdName))
			continue
		}

		if !w.triggerAllowed(config, req.TriggerName) {
			continue
		}

		if !req.FilterHookID.IsZero() {
			id := domainID(config)
			idMatch := id == req.FilterHookID
			w.logger.Debug("note rewrite hook ID filter check",
				zap.String("domH.ID", id.String()),
				zap.String("filterHookID", req.FilterHookID.String()),
				zap.Bool("matched", idMatch),
			)
			if !idMatch {
				continue
			}
		}

		// 寻找配套的图片
		evs, err := w.notes.AssociatedImageEvents(ctx, req.NoteRelPath)
		if err != nil {
			w.logger.Error("failed to get associated image for note directive", zap.String("note_path", req.NoteRelPath.String()), zap.Error(err))
			continue
		}

		var args []string
		var syntaxErr error
		if cmdArgs != "" {
			args, syntaxErr = splitArgs(cmdArgs)
		}

		w.logger.Debug("note rewrite appending pending directive",
			zap.String("hookID", config.ID),
			zap.Strings("args", args),
			zap.Error(syntaxErr),
		)

		// 将此任务入队暂存，暂不执行
		pending = append(pending, pendingDirective{
			config:      config,
			triggerName: req.TriggerName,
			events:      evs,
			args:        args,
			relPath:     req.NoteRelPath,
			dir:         req.Dir,
			identity:    identity,
			syntaxErr:   syntaxErr,
		})
	}

	w.logger.Debug("note rewrite loop finished", zap.Int("pendingCount", len(pending)))
	return pending
}

// triggerAllowed 判断钩子在当前触发来源下是否接受指令：
// 未声明该触发器、或声明为「无条件静默」时，指令阶段不重复触发
func (w *noteRewriter) triggerAllowed(config hookConfig, triggerName string) bool {
	switch triggerName {
	case "post_update_note":
		if config.On.PostUpdateNote == nil {
			w.logger.Debug("note rewrite trigger filter skipped: On.PostUpdateNote is nil")
			return false
		}
		if config.On.PostUpdateNote.IgnoreDirective {
			w.logger.Debug("note rewrite trigger filter skipped: On.PostUpdateNote.IgnoreDirective is true")
			return false
		}
		return true
	case "post_commit_session":
		if config.On.PostCommitSession == nil || config.On.PostCommitSession.NoteScan == nil {
			w.logger.Debug("note rewrite trigger filter skipped: On.PostCommitSession.NoteScan is nil")
			return false
		}
		if config.On.PostCommitSession.NoteScan.IgnoreDirective {
			w.logger.Debug("note rewrite trigger filter skipped: On.PostCommitSession.NoteScan.IgnoreDirective is true")
			return false
		}
		return true
	case "note_dispatch":
		if config.On.NoteDispatch == nil {
			w.logger.Debug("note rewrite trigger filter skipped: On.NoteDispatch is nil")
			return false
		}
		return true
	default:
		w.logger.Debug("note rewrite trigger filter skipped: unknown triggerName")
		return false
	}
}

// pendingTask 把一条待执行指令翻译为单次执行器的输入
func (w *noteRewriter) pendingTask(p pendingDirective, runID string) hookExecutionTask {
	task := newHookTask(p.config, p.triggerName, p.events, p.relPath.String(), p.dir)
	task.ExtraArgs = p.args
	task.RunID = runID
	task.DirectiveText = p.identity.line
	return task
}

// registerTask 登记一次指令运行的起始路径
func (w *noteRewriter) registerTask(runID string, noteAbsPath string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	task, exists := w.tasks[runID]
	if !exists {
		task = &activeTask{phase: phaseBefore3, paths: make(map[string]struct{})}
		w.tasks[runID] = task
	}
	task.paths[noteAbsPath] = struct{}{}
}

// finishTask 标记指令执行收尾并按指令身份登记完整执行结果，返回需要擦除的笔记路径
func (w *noteRewriter) finishTask(runID string, results map[directiveIdentity]directiveResult) []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	task := w.tasks[runID]
	task.phase = phaseAfter3
	task.results = results

	paths := make([]string, 0, len(task.paths))
	for p := range task.paths {
		paths = append(paths, p)
	}
	return paths
}

// erase 在指令运行关联的所有路径上按指令身份擦除指令行并移除 run_id
func (w *noteRewriter) erase(paths []string, runID string, results map[directiveIdentity]directiveResult) error {
	errB := util.NewErrorsBuilder(len(paths))
	for _, p := range paths {
		contentBytes, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errB.Add(fmt.Errorf("failed to read file during directive cleanup %s: %w", p, err))
			continue
		}

		fileContent := string(contentBytes)
		currentID := getHookRunID(fileContent)
		if currentID != runID {
			continue
		}

		finalContent := removeHookRunID(w.eraseDirectives(fileContent, results))

		if finalContent == fileContent {
			continue
		}
		if finalContent == "" {
			if err := os.Remove(p); err != nil {
				errB.Add(fmt.Errorf("failed to delete empty note file after directive cleanup %s: %w", p, err))
			}
			continue
		}
		if err := w.guard.Write(p, []byte(finalContent), 0644); err != nil {
			errB.Add(fmt.Errorf("failed to write clean content to note file during cleanup %s: %w", p, err))
		}
	}
	return errB.Build()
}

// eraseDirectives 按指令身份（原文 + 出现次序）查表替换当前文本中的指令行。
// 查不到记录的指令行（例如脚本在执行期间改写了该行）保留当前内容，不做替换，
// 从而避免按顺序错配到别的指令的结果上。
func (w *noteRewriter) eraseDirectives(content string, results map[directiveIdentity]directiveResult) string {
	scanner := newIdentityScanner()
	return directiveReg.ReplaceAllStringFunc(content, func(matchedLine string) string {
		identity := scanner.identify(matchedLine)
		result, ok := results[identity]
		if !ok {
			// 当前内容与执行记录对不上：保留该行并留痕，便于定位脚本改写指令行的场景
			w.logger.Debug("directive line has no recorded result, keeping current content",
				zap.String("line", identity.line),
				zap.Int("ordinal", identity.ordinal),
			)
			return matchedLine
		}
		return applyDirectiveAction(result.action, matchedLine, result.stdout, result.stderr, result.executedAt)
	})
}

// CleanupLate 迟到擦除：指令执行早已收尾、但带 run_id 的文件写入此刻才到达时，
// 按已记录的指令结果擦除指令行并移除 run_id。与主擦除路径共用同一套身份查表与动作应用，
// 因此产出的文本（含 stdout/stderr 结果提示块）与主路径一致。
func (w *noteRewriter) CleanupLate(absPath string, runID string, results map[directiveIdentity]directiveResult) error {
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read file during late task cleanup: %w", err)
	}

	fileContent := string(contentBytes)
	if getHookRunID(fileContent) != runID {
		return nil
	}

	finalContent := removeHookRunID(w.eraseDirectives(fileContent, results))

	if finalContent == fileContent {
		return nil
	}
	if finalContent == "" {
		if err := os.Remove(absPath); err != nil {
			return fmt.Errorf("failed to delete empty note file during late cleanup: %w", err)
		}
		return nil
	}
	if err := w.guard.Write(absPath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write clean content to note file during late cleanup: %w", err)
	}
	return nil
}

// directiveHooks 构建指令名到钩子配置的映射；同名指令后声明者覆盖先声明者并记录冲突
func (w *noteRewriter) directiveHooks() (map[string]hookConfig, error) {
	hooks, err := w.config.Load()
	if err != nil {
		return nil, err
	}

	hookMap := make(map[string]hookConfig)
	var registered []string
	for _, h := range hooks {
		if h.Directive == nil || h.Directive.Name == "" {
			continue
		}
		if existing, ok := hookMap[h.Directive.Name]; ok {
			w.logger.Error("duplicate directive name detected, the later hook will override the earlier one",
				zap.String("directive", h.Directive.Name),
				zap.String("existing_hook_id", existing.ID),
				zap.String("new_hook_id", h.ID),
			)
		}
		hookMap[h.Directive.Name] = h
		registered = append(registered, h.Directive.Name)
	}
	w.logger.Debug("note rewrite directive map built", zap.Strings("directives", registered))
	return hookMap, nil
}
