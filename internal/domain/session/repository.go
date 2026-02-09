package session

import (
	"context"
	"iter"
	"main/internal/scalar"
)

// Repository Session 仓库接口
// 负责 Session 的持久化和并发控制
type Repository interface {
	// Create 创建新 Session 并返回释放函数
	// 创建后调用者持有访问权，必须调用 release 释放
	Create(session *Session) (release func(), err error)

	// Acquire 获取 Session 的独占访问权
	// 阻塞直到上一个使用者释放才返回
	// 返回 Session、释放函数和错误
	// 释放函数必须在使用完 Session 后调用，以便其他调用者可以获取访问权
	Acquire(ctx context.Context, id scalar.ID) (*Session, func(), error)

	// FindByDirectory 查找指定目录下的所有 Session ID
	FindByDirectory(directoryID scalar.ID) iter.Seq2[scalar.ID, error]
}
