package hook

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"main/internal/domain/directory"

	"go.uber.org/zap"
)

// hookRunResult 单次钩子执行的结果。
// Action 已解析：脚本通过 IMAGE_FUNNEL_ACTION 写入的覆盖优先，
// 否则回退到配置 [directive] 声明的成功/失败动作。
type hookRunResult struct {
	Action string
	Stdout string
	Stderr string
	Err    error
}

// hookExecutionTask 一次钩子执行的输入。既是单次执行器的投递单元，
// 也是通知文案（标题与详情链接）的上下文载体。
type hookExecutionTask struct {
	HookID   string
	HookName string
	Command  string
	// ExtraArgs 额外参数，原样追加在 command 分词结果之后
	ExtraArgs   []string
	TriggerName string
	Events      []hookEvent
	// Env 来自配置 [env] 的自定义环境变量
	Env map[string]string
	// NotePath 笔记文件的根相对路径，空串表示本次执行不涉及笔记
	NotePath string
	// Dir 本次执行所属的目录实体
	Dir *directory.Directory
	// RunID 本次执行的运行 ID，注入环境变量供脚本回写；空串表示由执行器生成
	RunID string
	// DirectiveText 笔记指令的完整文本（如 "/fork --option value"），用于通知标题
	DirectiveText string
	// OnSuccessAction/OnFailAction 来自配置 [directive]；空串表示该钩子不提供指令动作回退
	OnSuccessAction string
	OnFailAction    string
}

// newHookTask 由配置组装一次执行任务。非指令路径没有指令参数，
// 但配置声明的指令成功/失败动作仍要带上：执行器据此在脚本未写
// IMAGE_FUNNEL_ACTION 时回退出对外可见的动作。
func newHookTask(h hookConfig, triggerName string, events []hookEvent, noteRelPath string, dir *directory.Directory) hookExecutionTask {
	task := hookExecutionTask{
		HookID:      h.ID,
		HookName:    h.Name,
		Command:     h.Command,
		TriggerName: triggerName,
		Events:      events,
		Env:         h.Env,
		NotePath:    noteRelPath,
		Dir:         dir,
	}
	if h.Directive != nil {
		task.OnSuccessAction = h.Directive.OnSuccessAction
		task.OnFailAction = h.Directive.OnFailAction
	}
	return task
}

// queuedRun 入队的一次执行。reply 为 nil 表示调用方不消费结果。
type queuedRun struct {
	task  hookExecutionTask
	reply chan hookRunResult
}

// hookExecutor 单次钩子执行器：组合进程原语、env 构建、action override 读取与通知触发。
// 所有执行经同一条队列串行投递，保证同一 Runner 生命周期内钩子进程不并发，
// 使钩子之间的先后顺序与配置声明的 order 一致。
type hookExecutor struct {
	process  processRunner
	env      *envBuilder
	notifier *hookNotifier
	logger   *zap.Logger
	hooksDir string
	rootDir  string
	tasks    chan queuedRun

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func newHookExecutor(
	parent context.Context,
	process processRunner,
	env *envBuilder,
	notifier *hookNotifier,
	logger *zap.Logger,
	rootDir string,
	hooksDir string,
) *hookExecutor {
	ctx, cancel := context.WithCancel(parent)
	e := &hookExecutor{
		process:  process,
		env:      env,
		notifier: notifier,
		logger:   logger,
		hooksDir: hooksDir,
		rootDir:  rootDir,
		tasks:    make(chan queuedRun, 1024),
		ctx:      ctx,
		cancel:   cancel,
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.runLoop(ctx)
	}()
	return e
}

// Close 终止执行器：取消正在运行的钩子进程并等待 worker 退出。
func (e *hookExecutor) Close() {
	e.closeOnce.Do(func() {
		e.cancel()
		e.wg.Wait()
	})
}

// Enqueue 投递任务后立即返回，不等待执行结果。
// 防抖合批路径使用：回调中无人消费结果，只需要保证任务被串行执行。
func (e *hookExecutor) Enqueue(task hookExecutionTask) {
	e.enqueue(queuedRun{task: task})
}

// Execute 投递任务并同步等待执行结果。
// 执行器随 Runner 关闭后不再接受新任务，等待方以取消错误收尾。
func (e *hookExecutor) Execute(task hookExecutionTask) hookRunResult {
	reply := make(chan hookRunResult, 1)
	if !e.enqueue(queuedRun{task: task, reply: reply}) {
		return hookRunResult{Err: e.ctx.Err()}
	}
	select {
	case res := <-reply:
		return res
	case <-e.ctx.Done():
		return hookRunResult{Err: e.ctx.Err()}
	}
}

func (e *hookExecutor) enqueue(run queuedRun) bool {
	select {
	case e.tasks <- run:
		return true
	case <-e.ctx.Done():
		return false
	}
}

func (e *hookExecutor) runLoop(ctx context.Context) {
	for {
		select {
		case run := <-e.tasks:
			res := e.execute(ctx, run.task)
			if run.reply != nil {
				run.reply <- res
			}
		case <-ctx.Done():
			return
		}
	}
}

// execute 执行一次钩子脚本：分词命令、构建 env、交给进程原语运行，
// 再按脚本是否失败读取 IMAGE_FUNNEL_ACTION 覆盖并触发通知。
func (e *hookExecutor) execute(ctx context.Context, task hookExecutionTask) hookRunResult {
	if task.RunID == "" {
		task.RunID = newRunID()
	}

	// 将 command 分词为 argv 并追加指令参数，直接启动进程（不经 shell）
	argv, err := parseCommandArgs(task.Command)
	if err != nil {
		return e.fail(ctx, task, fmt.Errorf("invalid hook command: %w", err), "", "")
	}
	argv = append(argv, task.ExtraArgs...)

	// 生成临时文件路径供脚本通过 IMAGE_FUNNEL_ACTION 写入覆盖操作，不提前创建文件
	actionFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("image_funnel_action_%s.txt", task.RunID))

	var noteAbsPath string
	if task.NotePath != "" {
		noteAbsPath = filepath.Join(e.rootDir, task.NotePath)
	}

	var ids, paths []string
	for _, ev := range task.Events {
		ids = append(ids, ev.ID)
		paths = append(paths, ev.Path)
	}

	env, buildErr := e.env.Build(ctx, envRequest{
		HookID:      task.HookID,
		HookName:    task.HookName,
		TriggerName: task.TriggerName,
		ImageIDs:    ids,
		ImagePaths:  paths,
		NoteAbsPath: noteAbsPath,
		CustomEnv:   task.Env,
		FallbackDir: task.Dir,
	})
	if buildErr != nil {
		return e.fail(ctx, task, fmt.Errorf("failed to build hook env: %w", buildErr), "", "")
	}

	env = append(env, batchEnv(task.Events)...)
	env = append(env,
		"IMAGE_FUNNEL_ACTION="+actionFilePath,
		"IMAGE_FUNNEL_HOOK_RUN_ID="+task.RunID,
	)

	e.logger.Info("will execute external hook command",
		zap.String("hook_id", task.HookID),
		zap.String("trigger", task.TriggerName),
		zap.Int("batch_size", len(task.Events)),
	)

	start := time.Now()
	out, runErr := e.process.Run(ctx, argv, env, e.hooksDir)
	duration := time.Since(start)

	// 清理临时文件
	defer func() {
		if removeErr := os.Remove(actionFilePath); removeErr != nil && !os.IsNotExist(removeErr) {
			e.logger.Warn("failed to clean up action file", zap.String("path", actionFilePath), zap.Error(removeErr))
		}
	}()

	if runErr != nil {
		// 失败路径同样读取脚本的操作覆盖：覆盖文件由 defer 清理，必须在此之前读
		overrideAction, readErr := readActionOverride(actionFilePath)
		if readErr != nil {
			return e.fail(ctx, task, fmt.Errorf("%s", readErr.Error()), out.Stdout, out.Stderr)
		}
		e.logger.Error("external hook command failed",
			zap.String("hook_id", task.HookID),
			zap.Duration("duration", duration),
			zap.Error(runErr),
			zap.String("stdout", out.Stdout),
			zap.String("stderr", out.Stderr),
		)
		res := e.fail(ctx, task, scriptFailedError(runErr, out.Stderr), out.Stdout, out.Stderr)
		res.Action = resolveAction(task, overrideAction, true)
		return res
	}

	e.logger.Info("external hook command completed",
		zap.String("hook_id", task.HookID),
		zap.Duration("duration", duration),
		zap.String("stdout", out.Stdout),
	)
	// 开发环境通过 Debug 级别输出 stderr，生产环境自动过滤
	if out.Stderr != "" {
		e.logger.Debug("external hook stderr",
			zap.String("hook_id", task.HookID),
			zap.String("stderr", out.Stderr),
		)
	}

	// 读取脚本通过 IMAGE_FUNNEL_ACTION 写入的覆盖操作
	overrideAction, readErr := readActionOverride(actionFilePath)
	if readErr != nil {
		return e.fail(ctx, task, fmt.Errorf("%s", readErr.Error()), out.Stdout, out.Stderr)
	}

	// 脚本通过 IMAGE_FUNNEL_ACTION 覆盖为 KEEP 表示保持现状、本次未产生实际变更；
	// 此时若 stdout 也为空，说明这是一次没有任何可见结果的空跑，跳过成功通知以避免噪音。
	// stderr 是调试通道且成功通知本就不携带 stderr，故不参与判定；有输出或失败时仍照常通知。
	if overrideAction == "KEEP" && strings.TrimSpace(out.Stdout) == "" {
		e.logger.Debug("skipping notification for silent skipped hook run",
			zap.String("hook_id", task.HookID),
			zap.String("trigger", task.TriggerName),
		)
	} else {
		e.notifier.Notify(ctx, task, nil, out.Stdout, out.Stderr)
	}

	res := hookRunResult{Stdout: out.Stdout, Stderr: out.Stderr}
	res.Action = resolveAction(task, overrideAction, false)
	return res
}

// fail 失败出口：向用户报告并按脚本覆盖/配置声明解析出对外可见的操作
func (e *hookExecutor) fail(ctx context.Context, task hookExecutionTask, execErr error, stdout, stderr string) hookRunResult {
	e.notifier.Notify(ctx, task, execErr, stdout, stderr)
	res := hookRunResult{Err: execErr, Stdout: stdout, Stderr: stderr}
	res.Action = resolveAction(task, "", true)
	return res
}

// scriptFailedError 把进程退出错误与脚本 stderr 合并为面向用户的失败原因
func scriptFailedError(runErr error, stderr string) error {
	if trimmed := strings.TrimSpace(stderr); trimmed != "" {
		return fmt.Errorf("hook script failed: %w, stderr: %s", runErr, trimmed)
	}
	return fmt.Errorf("hook script failed: %w", runErr)
}

// resolveAction 解析本次执行对外可见的操作：
// 脚本通过 IMAGE_FUNNEL_ACTION 写入的覆盖优先——脚本比配置更清楚本次执行
// 是否已经产生了对外可见的结果（例如提交过工作流）；否则回退到配置声明的动作。
func resolveAction(task hookExecutionTask, override string, failed bool) string {
	if override != "" {
		return override
	}
	if failed {
		return task.OnFailAction
	}
	return task.OnSuccessAction
}

// batchEnv 把批次的图片元数据注入环境变量。批次非单张图片时元数据无意义，
// 按零值注入以保证脚本读到的键始终存在。
func batchEnv(events []hookEvent) []string {
	var ev hookEvent
	if len(events) == 1 {
		ev = events[0]
	}
	return []string{
		"IMAGE_FUNNEL_IMAGE_RATING=" + fmt.Sprintf("%d", ev.Rating),
		"IMAGE_FUNNEL_IMAGE_LABEL=" + ev.Label,
		"IMAGE_FUNNEL_IMAGE_ACTION=" + ev.Action,
		"IMAGE_FUNNEL_IMAGE_OLD_RATING=" + fmt.Sprintf("%d", ev.OldRating),
		"IMAGE_FUNNEL_IMAGE_OLD_LABEL=" + ev.OldLabel,
		"IMAGE_FUNNEL_IMAGE_OLD_ACTION=" + ev.OldAction,
	}
}

// newRunID 生成一次执行的运行 ID：纳秒时间戳 + 随机后缀，兼作临时文件名与 frontmatter 标记
func newRunID() string {
	return fmt.Sprintf("run_%019d_%06d", time.Now().UnixNano(), rand.Intn(1000000))
}

// readActionOverride 读取脚本通过 IMAGE_FUNNEL_ACTION 写入的覆盖操作。
// 文件不存在表示脚本未覆盖（正常路径）；读取失败或内容非法则报错，不静默忽略。
func readActionOverride(actionFilePath string) (string, error) {
	data, err := os.ReadFile(actionFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read IMAGE_FUNNEL_ACTION file: %w", err)
	}
	action := strings.TrimSpace(string(data))
	if action != "" && !isValidDirectiveAction(action) {
		return "", fmt.Errorf("unsupported action in IMAGE_FUNNEL_ACTION file: %q", action)
	}
	return action, nil
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
