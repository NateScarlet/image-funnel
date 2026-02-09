package session

import (
	"context"
	"main/internal/scalar"
)

// Acquire 获取会话并锁定
// 调用者必须在处理完成后调用返回的函数释放资源。
func (s *Service) Acquire(ctx context.Context, id scalar.ID) (*Session, func(), error) {
	return s.sessionRepo.Acquire(ctx, id)
}
