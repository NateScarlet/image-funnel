package session

import (
	"context"
	"sync"
	"time"

	"main/internal/domain/hook"
	"main/internal/scalar"

	"go.uber.org/zap"
)

// CommitPipeline 提交后流水线端口。Commit 通过 Enqueue 登记一次提交的全部决策。
//
// Enqueue 的时序取决于实现：后台异步实现登记后立即返回，同步实现在返回前完成全部副作用。
// 两种实现共用同一份编排，保证「采样全部完成 → 解析目录 → 触发钩子」的顺序硬约束，
// 以及单条采样失败只记日志并继续、目录解析失败记日志并跳过钩子的错误策略。
// 调用方不应从 Enqueue 的名字推断时序；需要等待后台任务时可调用异步实现的 Wait。
//
// 顺序是硬约束：钩子可能把源图移走或删除，抢先执行会让采样拿不到原图。
type CommitPipeline interface {
	Enqueue(samples []*Sample, directoryID scalar.ID)
}

// #region 归档开启：服务端持有的后台流水线

// AsyncCommitPipeline 归档开启时的后台流水线。
// 每条任务使用服务端持有的 ctx（随服务优雅关闭而取消），不复用已结束的请求 ctx。
// 调度之外的编排（采样 → 目录解析 → 钩子）由 commitPipelineExecutor 统一实现。
type AsyncCommitPipeline struct {
	ctx      context.Context
	logger   *zap.Logger
	executor *commitPipelineExecutor
	wg       sync.WaitGroup
}

var _ CommitPipeline = (*AsyncCommitPipeline)(nil)

// NewAsyncCommitPipeline 的 recorderFactory 为每次 Commit 创建独立的记录器，
// 从而让解码缓存与差异区域并集随 Commit 创建、结束丢弃，不跨 Commit 共享。
func NewAsyncCommitPipeline(
	ctx context.Context,
	recorderFactory SampleRecorderFactory,
	hookRunner hook.Runner,
	directoryResolver DirectoryResolver,
	logger *zap.Logger,
) *AsyncCommitPipeline {
	return &AsyncCommitPipeline{
		ctx:      ctx,
		logger:   logger,
		executor: newCommitPipelineExecutor(recorderFactory, hookRunner, directoryResolver, logger),
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

// run 只负责后台任务级的耗时日志与取消判定，编排细节交给共用 executor
func (p *AsyncCommitPipeline) run(samples []*Sample, directoryID scalar.ID) {
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
	p.executor.run(p.ctx, samples, directoryID, func() bool { return p.ctx.Err() == nil })
}

// Wait 等待已登记的后台任务结束；应在取消服务端 ctx 后调用
func (p *AsyncCommitPipeline) Wait() {
	p.wg.Wait()
}

// #endregion

// #region 归档关闭：不创建后台任务的空实现

// NullCommitPipeline 未开启归档时使用：同步跑完同一份编排，不创建后台任务，
// 注入的 recorder 为零值安全的 NullSampleRecorder，调用方行为与现状一致
// （钩子自身的 fire-and-forget 语义由 Runner 保证，不在此处改变）。
type NullCommitPipeline struct {
	ctx      context.Context
	executor *commitPipelineExecutor
}

var _ CommitPipeline = (*NullCommitPipeline)(nil)

func NewNullCommitPipeline(
	ctx context.Context,
	recorder SampleRecorder,
	hookRunner hook.Runner,
	directoryResolver DirectoryResolver,
	logger *zap.Logger,
) *NullCommitPipeline {
	// 同步调度只有一个 recorder，适配为工厂让两种调度走同一份编排；此工厂不会失败
	factory := SampleRecorderFactoryFunc(func() (SampleRecorder, error) { return recorder, nil })
	return &NullCommitPipeline{
		ctx:      ctx,
		executor: newCommitPipelineExecutor(factory, hookRunner, directoryResolver, logger),
	}
}

// Enqueue 同步完成采样与钩子后才返回，调度差异之外不复制任何编排逻辑
func (p *NullCommitPipeline) Enqueue(samples []*Sample, directoryID scalar.ID) {
	p.executor.run(p.ctx, samples, directoryID, nil)
}

// #endregion
