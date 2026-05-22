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

// LastSession 获取指定目录下最后更新的 Session 并锁定
// 调用者必须在处理完成后调用返回的函数释放资源。
func (s *Service) LastSession(ctx context.Context, directoryID scalar.ID) (*Session, func(), error) {
	return s.sessionRepo.LastSession(ctx, directoryID)
}
