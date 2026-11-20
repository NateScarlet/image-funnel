package session

import (
	"context"
	stdimage "image"
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

// SampleRecorderFactory 为一次 Commit 创建独立的 SampleRecorder。
// 解码图像缓存与差异区域并集只在单次 Commit 内复用，工厂保证它们不跨 Commit 共享，
// 也避免并发提交互相污染状态。
type SampleRecorderFactory interface {
	NewRecorder() (SampleRecorder, error)
}

// SampleRecorderFactoryFunc 把函数适配为 SampleRecorderFactory。
type SampleRecorderFactoryFunc func() (SampleRecorder, error)

func (f SampleRecorderFactoryFunc) NewRecorder() (SampleRecorder, error) { return f() }

var _ SampleRecorderFactory = SampleRecorderFactoryFunc(nil)

// SampleImage 标识一张参与本次决策比较的图片：版本化领域 ID + 原图相对路径。
// ID 是图像的规范身份（含版本），相对路径仅供适配器解析出绝对路径读原图。
type SampleImage struct {
	ID      scalar.ID
	RelPath relpath.Path
}

// ImageDecoder 把领域图像身份解码为图像，并持有单次 Commit 内的解码缓存。
// 缓存由实现方以版本化 SampleImage.ID 为键；调用方不得自行读盘，也不得用路径作缓存键。
type ImageDecoder interface {
	Decode(image SampleImage) (stdimage.Image, error)
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
