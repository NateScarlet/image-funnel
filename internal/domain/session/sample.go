package session

import (
	"context"
	"time"

	"main/internal/scalar"
	"main/internal/shared"
)

// SampleRecorder 训练样本采集端口。实现方负责在提交当下生成缩略图、计算 CID 与
// 字符串指纹，并写入 append-only 归档；本端口只接收决策事实，不感知存储细节。
// 未启用归档时由装配方注入 NullSampleRecorder，Commit 行为保持不变。
type SampleRecorder interface {
	Record(ctx context.Context, sample *Sample) error
}

// Sample 一条实际发生的标注决策。SourcePath 是原图绝对路径，供适配器立即生成缩略图；
// 缩略图 CID 与字符串指纹由归档适配器计算后写入 JSONL，不属于领域输入。
type Sample struct {
	ImageID     scalar.ID
	DirectoryID scalar.ID
	SessionID   scalar.ID
	Round       int
	Action      shared.ImageAction
	Rating      int
	Timestamp   time.Time
	SourcePath  string
}

// NullSampleRecorder 空实现：零值安全，未配置归档时保持原有提交行为。
type NullSampleRecorder struct{}

func (NullSampleRecorder) Record(context.Context, *Sample) error { return nil }

var _ SampleRecorder = NullSampleRecorder{}
