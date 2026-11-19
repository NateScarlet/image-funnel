package session

import (
	"context"
	"sync"
	"time"

	"main/internal/domain/hook"
	"main/internal/scalar"

	"go.uber.org/zap"
)

// CommitPipeline 提交后流水线端口。Commit 通过 Enqueue 非阻塞登记一次提交的全部决策，
// 实现保证采样阶段全部结束后才触发该目录的提交后钩子。
//
// 顺序是硬约束：钩子可能把源图移走或删除，抢先执行会让采样拿不到原图。
type CommitPipeline interface {
	Enqueue(samples []*Sample, directoryID scalar.ID)
}

// #region 归档开启：服务端持有的后台流水线

// AsyncCommitPipeline 归档开启时的后台流水线。
// 每条任务使用服务端持有的 ctx（随服务优雅关闭而取消），不复用已结束的请求 ctx。
// 单条采样失败只记 zap 日志，不中断后续样本，也不阻止钩子触发。
type AsyncCommitPipeline struct {
	ctx               context.Context
	recorder          SampleRecorder
	hookRunner        hook.Runner
	directoryResolver DirectoryResolver
	logger            *zap.Logger
	wg                sync.WaitGroup
}

var _ CommitPipeline = (*AsyncCommitPipeline)(nil)

func NewAsyncCommitPipeline(
	ctx context.Context,
	recorder SampleRecorder,
	hookRunner hook.Runner,
	directoryResolver DirectoryResolver,
	logger *zap.Logger,
) *AsyncCommitPipeline {
	return &AsyncCommitPipeline{
		ctx:               ctx,
		recorder:          recorder,
		hookRunner:        hookRunner,
		directoryResolver: directoryResolver,
		logger:            logger,
	}
}

// Enqueue 登记任务后立即返回，不等待采样或钩子完成
func (p *AsyncCommitPipeline) Enqueue(samples []*Sample, directoryID scalar.ID) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.run(samples, directoryID)
	}()
}

func (p *AsyncCommitPipeline) run(samples []*Sample, directoryID scalar.ID) {
	// 调试用逐层耗时日志：定位「采样阶段拖慢钩子」时，整条任务 → 采样阶段 → 单条样本 →
	// 目录解析 → 钩子触发的耗时分布；均为 Debug 级，仅在 dev 构建可见。
	taskStart := time.Now()
	p.logger.Debug("will process commit session pipeline",
		zap.Stringer("directoryID", directoryID),
		zap.Int("samples", len(samples)),
	)
	defer func() {
		p.logger.Debug("did process commit session pipeline",
			zap.Duration("duration", time.Since(taskStart)),
			zap.Stringer("directoryID", directoryID),
			zap.Int("samples", len(samples)),
		)
	}()

	// 采样阶段：图片 ID 带版本，文件不会被原地修改；按 Mark 当下捕获的相对路径在后台解析出绝对路径读原图
	samplingStart := time.Now()
	for i, sample := range samples {
		if p.ctx.Err() != nil {
			return
		}
		recordStart := time.Now()
		err := p.recorder.Record(p.ctx, sample)
		p.logger.Debug("did record training sample",
			zap.Duration("duration", time.Since(recordStart)),
			zap.Int("index", i),
			zap.Stringer("imageID", sample.ImageID),
			zap.Int("baselines", len(sample.Baselines)),
			zap.Error(err),
		)
		if err != nil {
			p.logger.Error("failed to archive training sample",
				zap.Stringer("imageID", sample.ImageID),
				zap.Error(err),
			)
		}
	}
	p.logger.Debug("did sample training samples phase",
		zap.Duration("duration", time.Since(samplingStart)),
		zap.Int("count", len(samples)),
	)
	if p.ctx.Err() != nil {
		return
	}

	resolveStart := time.Now()
	dir, err := p.directoryResolver.GetDirectory(p.ctx, directoryID)
	p.logger.Debug("did resolve commit hook directory",
		zap.Duration("duration", time.Since(resolveStart)),
		zap.Stringer("directoryID", directoryID),
		zap.Error(err),
	)
	if err != nil || dir == nil {
		p.logger.Error("failed to resolve directory for post commit session hook",
			zap.Stringer("directoryID", directoryID),
			zap.Error(err),
		)
		return
	}
	hookStart := time.Now()
	err = p.hookRunner.OnCommitSession(p.ctx, dir.RelPath())
	p.logger.Debug("did trigger post commit session hook",
		zap.Duration("duration", time.Since(hookStart)),
		zap.Stringer("directoryID", directoryID),
		zap.Error(err),
	)
	if err != nil {
		p.logger.Error("failed to trigger post commit session hook", zap.Error(err))
	}
}

// Wait 等待已登记的后台任务结束；应在取消服务端 ctx 后调用
func (p *AsyncCommitPipeline) Wait() {
	p.wg.Wait()
}

// #endregion

// #region 归档关闭：不创建后台任务的空实现

// NullCommitPipeline 未开启归档时使用：采样与钩子都同步完成，不创建后台任务，
// 注入的 recorder 为零值安全的 NullSampleRecorder，调用方行为与现状一致
// （钩子自身的 fire-and-forget 语义由 Runner 保证，不在此处改变）。
type NullCommitPipeline struct {
	ctx               context.Context
	recorder          SampleRecorder
	hookRunner        hook.Runner
	directoryResolver DirectoryResolver
	logger            *zap.Logger
}

var _ CommitPipeline = (*NullCommitPipeline)(nil)

func NewNullCommitPipeline(
	ctx context.Context,
	recorder SampleRecorder,
	hookRunner hook.Runner,
	directoryResolver DirectoryResolver,
	logger *zap.Logger,
) *NullCommitPipeline {
	return &NullCommitPipeline{
		ctx:               ctx,
		recorder:          recorder,
		hookRunner:        hookRunner,
		directoryResolver: directoryResolver,
		logger:            logger,
	}
}

func (p *NullCommitPipeline) Enqueue(samples []*Sample, directoryID scalar.ID) {
	for _, sample := range samples {
		if err := p.recorder.Record(p.ctx, sample); err != nil {
			p.logger.Error("failed to archive training sample",
				zap.Stringer("imageID", sample.ImageID),
				zap.Error(err),
			)
		}
	}
	dir, err := p.directoryResolver.GetDirectory(p.ctx, directoryID)
	if err != nil || dir == nil {
		p.logger.Error("failed to resolve directory for post commit session hook",
			zap.Stringer("directoryID", directoryID),
			zap.Error(err),
		)
		return
	}
	if err := p.hookRunner.OnCommitSession(p.ctx, dir.RelPath()); err != nil {
		p.logger.Error("failed to trigger post commit session hook", zap.Error(err))
	}
}

// #endregion
