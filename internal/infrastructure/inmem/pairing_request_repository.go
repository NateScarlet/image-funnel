package inmem

import (
	"context"
	"iter"
	"sync"

	"main/internal/apperror"
	"main/internal/domain/pairing"
)

// PairingRequestRepository 提供基于内存的配对请求仓储实现。
// 内部使用读写锁来保护并发安全。
type PairingRequestRepository struct {
	reqs map[string]*pairing.Request
	mu   sync.RWMutex
}

// NewPairingRequestRepository 初始化并返回一个新的内存配对仓库实例。
func NewPairingRequestRepository() *PairingRequestRepository {
	return &PairingRequestRepository{
		reqs: make(map[string]*pairing.Request),
	}
}

// Save 将配对请求存储到内存 map 中。
func (r *PairingRequestRepository) Save(ctx context.Context, req *pairing.Request) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs[req.Code()] = req
	return nil
}

// Get 根据配对码获取相应的配对请求。
// 如果配对码不存在，则返回表示无效配对码的业务错误。
func (r *PairingRequestRepository) Get(ctx context.Context, code string) (*pairing.Request, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	req, ok := r.reqs[code]
	if !ok {
		return nil, apperror.New("INVALID_PAIRING_CODE", "Invalid pairing code", "无效的配对码")
	}
	return req, nil
}

// Delete 从内存 map 中移除对应的配对码数据。
func (r *PairingRequestRepository) Delete(ctx context.Context, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.reqs, code)
	return nil
}

// Find 返回所有的配对请求。
// 为了保证并发安全，在锁内先克隆一份切片，在锁外执行 yield。
func (r *PairingRequestRepository) Find(ctx context.Context) iter.Seq2[*pairing.Request, error] {
	r.mu.RLock()
	reqs := make([]*pairing.Request, 0, len(r.reqs))
	for _, req := range r.reqs {
		reqs = append(reqs, req)
	}
	r.mu.RUnlock()

	return func(yield func(*pairing.Request, error) bool) {
		for _, req := range reqs {
			if !yield(req, nil) {
				return
			}
		}
	}
}

// 静态断言，确保满足领域层定义的仓储接口
var _ pairing.Repository = (*PairingRequestRepository)(nil)
