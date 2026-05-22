package memo

import (
	"context"
	"main/internal/scalar"
)

// Service 备忘录领域服务，负责处理更新等业务逻辑
type Service struct {
	repo Repository
}

// NewService 创建一个新的备忘录服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Save 保存备忘录，将更新操作及相关逻辑封装在领域层
// 传入的 content 为包含 frontmatter 的完整内容（rawContent）
func (s *Service) Save(ctx context.Context, id scalar.ID, content string) error {
	// 在此处可以通过领域服务直接对内容或者状态进行处理，如果未来有更为复杂的修改或者状态验证可以在这编写
	// 目前封装了 repo.Write 的调用
	return s.repo.Write(ctx, id, content)
}
