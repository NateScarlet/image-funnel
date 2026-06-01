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
	repo    Repository
	rootDir string
}

// NewService 创建一个新的备忘录服务
func NewService(repo Repository, rootDir string) *Service {
	return &Service{
		repo:    repo,
		rootDir: rootDir,
	}
}

// Save 保存备忘录，将更新操作及相关逻辑封装在领域层
// 传入的 content 为包含 frontmatter 的完整内容（rawContent）
func (s *Service) Save(ctx context.Context, id scalar.ID, content string) error {
	relPath, err := decodeID(id)
	if err != nil {
		return err
	}
	return s.repo.Write(ctx, relPath, content)
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

	// 检查该相对路径是否已存在
	existing, err := s.repo.Read(ctx, relPath)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apperror.New("ALREADY_EXISTS", "memo already exists", "备忘录已存在")
	}

	// 写入新文件内容
	if err := s.repo.Write(ctx, relPath, content); err != nil {
		return nil, err
	}

	// 重新读取并返回包含系统元数据信息的实体
	return s.repo.Read(ctx, relPath)
}

// Read 根据备忘 ID 获取备忘实体，由领域层内部解码 ID。如果磁盘上不存在对应文件，则返回空备忘录实体以支持非空约束。
func (s *Service) Read(ctx context.Context, id scalar.ID) (*Memo, error) {
	relPath, err := decodeID(id)
	if err != nil {
		return nil, err
	}
	m, err := s.repo.Read(ctx, relPath)
	if err != nil {
		return nil, err
	}
	if m == nil {
		m = s.newEmpty(relPath)
	}
	return m, nil
}

// ReadByRelPath 根据相对路径获取备忘实体，如果磁盘上不存在对应文件或不是文本文件，则返回空备忘录实体。
func (s *Service) ReadByRelPath(ctx context.Context, relPath string) (*Memo, error) {
	m, err := s.repo.Read(ctx, relPath)
	if err != nil {
		if apperror.ErrCode(err) == "NOT_TEXT" {
			return s.newEmpty(relPath), nil
		}
		return nil, err
	}
	if m == nil {
		m = s.newEmpty(relPath)
	}
	return m, nil
}

// newEmpty 创建一个未在磁盘中持久化的空备忘实体，仅供内部使用。
func (s *Service) newEmpty(relPath string) *Memo {
	absPath := filepath.Join(s.rootDir, relPath)
	return newMemo(encodeID(relPath), relPath, absPath, "")
}




