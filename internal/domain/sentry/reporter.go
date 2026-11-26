package sentry

import (
	"context"
	"database/sql"
	"errors"
	"io"

	"main/internal/apperror"
)

// #region 上报端口

// Event 是服务端上报的领域描述，不承载任何 SDK 细节。
// Err 用于普通错误；Panic 用于未处理 panic，两者互斥。
// Fingerprint 显式指定落盘分片目录的元素，缺省时由接收端按事件内容推导。
type Event struct {
	Err         error
	Panic       any
	Stack       []byte
	Fingerprint []string
}

// Reporter 是服务端错误上报端口，由最外层依据配置 DSN 注入实现。
// Report 的失败由调用方按日志处理，不得再次进入上报链路。
type Reporter interface {
	Report(ctx context.Context, event Event) error
}

type noopReporter struct{}

// NoopReporter 是 DSN 为空时的空实现：不创建客户端、不产生任何事件。
var NoopReporter Reporter = noopReporter{}

func (noopReporter) Report(context.Context, Event) error { return nil }

// #endregion

// ShouldReport 只放开需要开发者处理的错误：
// 已知用户错误（apperror）与可恢复错误（取消、超时）只记日志，不进入上报。
func ShouldReport(err error) bool {
	if err == nil {
		return false
	}
	var appErr *apperror.AppError
	if apperror.As(err, &appErr) || apperror.IsNotFound(err) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// 流结束与空查询结果都是正常控制流，不是需要开发者处理的错误
	if errors.Is(err, io.EOF) || errors.Is(err, sql.ErrNoRows) {
		return false
	}
	return true
}
