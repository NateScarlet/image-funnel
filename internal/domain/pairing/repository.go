package pairing

import (
	"context"
	"iter"
)

// Repository 定义了配对请求（PairingRequest）的持久化生命周期操作。
type Repository interface {
	// Save 保存或更新配对请求
	Save(ctx context.Context, req *Request) error

	// Get 根据唯一的配对码查询配对请求，如果不存在应当返回对应的业务错误
	Get(ctx context.Context, code string) (*Request, error)

	// Delete 彻底移除某个配对码对应的配对请求
	Delete(ctx context.Context, code string) error

	// Find 返回所有的配对请求
	Find(ctx context.Context) iter.Seq2[*Request, error]
}
