package loadlatency

import (
	"context"
	"time"

	"go.uber.org/zap"

	"main/internal/domain/loadlatency"
)

// Appender 段落盘端口。服务端只追加、不去重、不读已有内容。
type Appender interface {
	// Append 把一段 JSONL 文本追加到 runId 对应的文件，返回写出的绝对路径。
	Append(ctx context.Context, runID string, data []byte) (string, error)
}

// Handler 加载延迟段的写入用例：在可信边界校验后委托落盘。
type Handler struct {
	appender Appender
	logger   *zap.Logger
}

func NewHandler(appender Appender, logger *zap.Logger) *Handler {
	return &Handler{appender: appender, logger: logger}
}

// AppendSegment 校验 runId 与段体积后追加落盘，返回绝对路径。
// 重复提交不去重：读取侧按 (runId, segmentSeq) 保留首份。
func (h *Handler) AppendSegment(ctx context.Context, runID, segment string) (path string, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			h.logger.Error("did append load latency segment",
				zap.String("runId", runID),
				zap.Error(err),
				zap.Duration("duration", time.Since(start)),
			)
			return
		}
		h.logger.Info("did append load latency segment",
			zap.String("runId", runID),
			zap.String("path", path),
			zap.Duration("duration", time.Since(start)),
		)
	}()
	h.logger.Info("will append load latency segment", zap.String("runId", runID))

	if err = loadlatency.ValidateRunID(runID); err != nil {
		return "", err
	}
	data := []byte(segment)
	if err = loadlatency.ValidateSegmentSize(data); err != nil {
		return "", err
	}
	return h.appender.Append(ctx, runID, data)
}
