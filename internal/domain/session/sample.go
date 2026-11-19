package session

import (
	"context"
	"time"

	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
)

// SampleRecorder 训练样本采集端口。实现方负责从原图生成候选块与基准块、计算 CID 与
// 字符串指纹，并写入 append-only 归档；本端口只接收决策事实，不感知存储细节。
// 调用方是提交后后台流水线，单条失败由流水线记日志，不进入 Commit 返回值。
type SampleRecorder interface {
	Record(ctx context.Context, sample *Sample) error
}

// SampleImage 标识一张参与本次决策比较的图片：领域 ID + 原图相对路径。
// 相对路径供后台适配器解析出绝对路径读原图；领域层不做任何图像计算。
type SampleImage struct {
	ID      scalar.ID
	RelPath relpath.Path
}

// Sample 一条在 Mark 当下捕获的标注决策。
//
// Baselines 是本次 Mark 之前会话内全部处于 KEEP 状态的图（排除当前图自己），
// 代表用户作出这次取舍所依据的参照集合；无基准时为空切片，不伪造。
// SourceRelPath 是当前候选图的相对路径。所有图像产物（CID）由归档适配器在后台生成。
type Sample struct {
	ImageID       scalar.ID
	DirectoryID   scalar.ID
	SessionID     scalar.ID
	Round         int
	Action        shared.ImageAction
	Timestamp     time.Time
	SourceRelPath relpath.Path
	Baselines     []SampleImage
}

// NullSampleRecorder 空实现：零值安全，归档关闭时由装配方注入，
// Record 为空操作，配合同步流水线不创建采样后台任务。
type NullSampleRecorder struct{}

func (NullSampleRecorder) Record(context.Context, *Sample) error { return nil }

var _ SampleRecorder = NullSampleRecorder{}
