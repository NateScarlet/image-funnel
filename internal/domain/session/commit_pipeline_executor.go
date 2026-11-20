package session

import (
	"context"
	"time"

	"main/internal/domain/hook"
	"main/internal/scalar"

	"go.uber.org/zap"
)

// commitPipelineExecutor 提交后流水线的共用编排模块。
// 采样阶段全部结束后才解析目录并触发钩子；单条采样失败只记日志并继续其余样本，
// 目录解析失败只记日志并跳过钩子。顺序与错误策略只在此处实现，
// 后台/同步两种调度只决定「何时、在哪个 goroutine」调用 run。
type commitPipelineExecutor struct {
	recorderFactory   SampleRecorderFactory
	hookRunner        hook.Runner
	directoryResolver DirectoryResolver
	logger            *zap.Logger
}

func newCommitPipelineExecutor(
	recorderFactory SampleRecorderFactory,
	hookRunner hook.Runner,
	directoryResolver DirectoryResolver,
	logger *zap.Logger,
) *commitPipelineExecutor {
	return &commitPipelineExecutor{
		recorderFactory:   recorderFactory,
		hookRunner:        hookRunner,
		directoryResolver: directoryResolver,
		logger:            logger,
	}
}

// run 执行整条编排。canProceed 由调度方在每个阶段边界判定是否中止
// （后台调度据此在服务端关闭时中止），nil 表示不中止；同步调度没有后台任务，传入 nil。
// 取消判定刻意不折叠进 ctx：同步调度不获得取消语义，以保持「返回前副作用全部完成」的契约。
func (e *commitPipelineExecutor) run(ctx context.Context, samples []*Sample, directoryID scalar.ID, canProceed func() bool) {
	// 采样阶段：图片 ID 带版本，文件不会被原地修改；按 Mark 当下捕获的相对路径解析出绝对路径读原图。
	// 记录器由工厂创建（工厂语义见 SampleRecorderFactory），保证解码器与区域并集只在本次 Commit 内有效。
	samplingStart := time.Now()
	recorder, err := e.recorderFactory.NewRecorder()
	if err != nil {
		e.logger.Error("failed to create training sample recorder", zap.Error(err))
	} else {
		for i, sample := range samples {
			if canProceed != nil && !canProceed() {
				return
			}
			recordStart := time.Now()
			err := recorder.Record(ctx, sample)
			// 调试用逐层耗时日志：定位「采样阶段拖慢钩子」时，看单条样本 → 目录解析 → 钩子触发的耗时分布；
			// Debug 级，仅在 dev 构建可见。
			e.logger.Debug("did record training sample",
				zap.Duration("duration", time.Since(recordStart)),
				zap.Int("index", i),
				zap.Stringer("imageID", sample.ImageID),
				zap.Int("baselines", len(sample.Baselines)),
				zap.Error(err),
			)
			if err != nil {
				e.logger.Error("failed to archive training sample",
					zap.Stringer("imageID", sample.ImageID),
					zap.Error(err),
				)
			}
		}
	}
	e.logger.Debug("did sample training samples phase",
		zap.Duration("duration", time.Since(samplingStart)),
		zap.Int("count", len(samples)),
	)
	if canProceed != nil && !canProceed() {
		return
	}

	// 目录解析与钩子触发必须在采样阶段全部结束之后：钩子可能移走或删除源图。
	resolveStart := time.Now()
	dir, err := e.directoryResolver.GetDirectory(ctx, directoryID)
	e.logger.Debug("did resolve commit hook directory",
		zap.Duration("duration", time.Since(resolveStart)),
		zap.Stringer("directoryID", directoryID),
		zap.Error(err),
	)
	if err != nil || dir == nil {
		e.logger.Error("failed to resolve directory for post commit session hook",
			zap.Stringer("directoryID", directoryID),
			zap.Error(err),
		)
		return
	}
	hookStart := time.Now()
	err = e.hookRunner.OnCommitSession(ctx, dir.RelPath())
	e.logger.Debug("did trigger post commit session hook",
		zap.Duration("duration", time.Since(hookStart)),
		zap.Stringer("directoryID", directoryID),
		zap.Error(err),
	)
	if err != nil {
		e.logger.Error("failed to trigger post commit session hook", zap.Error(err))
	}
}
