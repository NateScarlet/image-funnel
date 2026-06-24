package note

import (
	"context"
	"main/internal/apperror"
	"main/internal/scalar"
	"path/filepath"
	"strings"
)

// Service 笔记领域服务，负责处理更新等业务逻辑
type Service struct {
	repo    Repository
	factory *Factory
}

// NewService 创建一个新的笔记服务
func NewService(repo Repository, factory *Factory) *Service {
	return &Service{
		repo:    repo,
		factory: factory,
	}
}

// Save 保存笔记，将更新操作及相关逻辑封装在领域层
// 传入的 content 为包含 frontmatter 的完整内容（rawContent）
func (s *Service) Save(ctx context.Context, id scalar.ID, content string) error {
	relPath, err := decodeID(id)
	if err != nil {
		return err
	}
	return s.repo.Write(ctx, relPath, content)
}

// Create 创建新的笔记文件，若已存在同名笔记则返回 ALREADY_EXISTS 错误。
// 返回创建成功后的 Note 实体。
func (s *Service) Create(ctx context.Context, dirRelPath string, name string, content string) (*Note, error) {
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
		return nil, apperror.New("ALREADY_EXISTS", "note already exists", "笔记已存在")
	}

	// 写入新文件内容
	if err := s.repo.Write(ctx, relPath, content); err != nil {
		return nil, err
	}

	// 重新读取并返回包含系统元数据信息的实体
	return s.repo.Read(ctx, relPath)
}

// Read 根据笔记 ID 获取笔记实体，由领域层内部解码 ID。如果磁盘上不存在对应文件，则返回空笔记实体以支持非空约束。
func (s *Service) Read(ctx context.Context, id scalar.ID) (*Note, error) {
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

// ReadByRelPath 根据相对路径获取笔记实体，如果磁盘上不存在对应文件或不是文本文件，则返回空笔记实体。
func (s *Service) ReadByRelPath(ctx context.Context, relPath string) (*Note, error) {
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

// newEmpty 创建一个未在磁盘中持久化的空笔记实体，仅供内部使用。
func (s *Service) newEmpty(relPath string) *Note {
	m, err := s.factory.New(relPath, "")
	if err != nil {
		panic(err)
	}
	return m
}
