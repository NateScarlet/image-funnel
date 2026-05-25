package memo

import (
	"context"
	"main/internal/apperror"
	"main/internal/scalar"
	"path/filepath"
	"strings"
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

// Create 创建新的备忘录文件，若已存在同名备忘则返回 ALREADY_EXISTS 错误。
// 返回创建成功后的 Memo 实体。
func (s *Service) Create(ctx context.Context, dirRelPath string, name string, content string) (*Memo, error) {
	// 清洗文件名，去除首尾空格及 .md 后缀
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".md")

	// 拼接文件的相对路径，并统一为正斜杠
	var relPath string
	if dirRelPath == "" {
		relPath = name + ".md"
	} else {
		relPath = filepath.ToSlash(filepath.Join(dirRelPath, name)) + ".md"
	}

	// 在领域层内部自建生成 ID
	id := EncodeID(relPath)

	// 检查该 ID 是否已存在
	existing, err := s.repo.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apperror.New("ALREADY_EXISTS", "memo already exists", "备忘录已存在")
	}

	// 写入新文件内容
	if err := s.repo.Write(ctx, id, content); err != nil {
		return nil, err
	}

	// 重新读取并返回包含系统元数据信息的实体
	return s.repo.Read(ctx, id)
}

