package memo

import (
	"context"
	"iter"
	"main/internal/domain/memo"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"strings"
)

type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	repo       memo.Repository
	service    *memo.Service
	ebus       EventBus
	dtoFactory *DTOFactory
	rootDir    string
}

func NewHandler(repo memo.Repository, ebus EventBus, rootDir string) *Handler {
	return &Handler{
		repo:       repo,
		service:    memo.NewService(repo),
		ebus:       ebus,
		dtoFactory: NewDTOFactory(rootDir),
		rootDir:    rootDir,
	}
}

// UpdateMemo 更新备忘录
func (h *Handler) UpdateMemo(ctx context.Context, id scalar.ID, content string) error {
	// 将更新逻辑及所需依赖提取到 domain/memo/service.go 执行
	return h.service.Save(ctx, id, content)
}

// Memo 获取备忘录内容
func (h *Handler) Memo(ctx context.Context, id scalar.ID) (*shared.MemoDTO, error) {
	m, err := h.repo.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return h.dtoFactory.NewEmpty(id)
	}
	return h.dtoFactory.New(m), nil
}

// MemoByRelPath 根据相对路径获取备忘录内容
func (h *Handler) MemoByRelPath(ctx context.Context, relPath string) (*shared.MemoDTO, error) {
	id := memo.EncodeID(relPath)
	return h.Memo(ctx, id)
}

// #region 辅助方法

// deriveMemoID 根据 markdown 物理文件相对路径推导其关联的最佳 Memo ID
func (h *Handler) deriveMemoID(relPath string) scalar.ID {
	if !strings.HasSuffix(strings.ToLower(relPath), ".md") {
		return memo.EncodeID(relPath)
	}

	baseName := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	supportedExtensions := []string{".jpg", ".jpeg", ".png", ".webp", ".avif"}

	// 检查该目录下是否存在同名图片
	for _, ext := range supportedExtensions {
		imageFilename := baseName + ext
		imageAbsPath := filepath.Join(h.rootDir, imageFilename)
		if _, err := os.Stat(imageAbsPath); err == nil {
			return memo.EncodeID(imageFilename)
		}
	}

	return memo.EncodeID(relPath)
}

// #endregion

// #region 订阅逻辑

// MemoUpdated 订阅特定备忘录更新（保持老版本向后兼容）
func (h *Handler) MemoUpdated(ctx context.Context, id scalar.ID) iter.Seq2[*shared.MemoDTO, error] {
	return func(yield func(*shared.MemoDTO, error) bool) {
		targetRelPath, err := memo.DecodeID(id)
		if err != nil {
			yield(nil, err)
			return
		}
		targetMemoPath := strings.TrimSuffix(targetRelPath, filepath.Ext(targetRelPath)) + ".md"

		for event, err := range h.ebus.SubscribeFileChanged(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// 检查是否是目标备注文件的变更
			if event.RelPath == targetMemoPath {
				m, err := h.Memo(ctx, id)
				if !yield(m, err) {
					return
				}
			}
		}
	}
}

// MemoSaved 订阅备忘录改变（新接口，支持目录/ID/是否隐藏等条件灵活过滤）
func (h *Handler) MemoSaved(ctx context.Context, filter *shared.MemoFilters) iter.Seq2[*shared.MemoDTO, error] {
	return func(yield func(*shared.MemoDTO, error) bool) {
		var allowedDirectoryIDs map[scalar.ID]bool
		var allowedIDs map[scalar.ID]bool

		if filter != nil {
			if len(filter.DirectoryID) > 0 {
				allowedDirectoryIDs = make(map[scalar.ID]bool)
				for _, id := range filter.DirectoryID {
					allowedDirectoryIDs[id] = true
				}
			}
			if len(filter.ID) > 0 {
				allowedIDs = make(map[scalar.ID]bool)
				for _, id := range filter.ID {
					allowedIDs[id] = true
				}
			}
		}

		for event, err := range h.ebus.SubscribeFileChanged(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// 仅关心以 .md 结尾的文件变更
			if !strings.HasSuffix(strings.ToLower(event.RelPath), ".md") {
				continue
			}

			// 过滤目录 ID
			if allowedDirectoryIDs != nil && !allowedDirectoryIDs[event.DirectoryID] {
				continue
			}

			// 推导 Memo ID
			derivedID := h.deriveMemoID(event.RelPath)

			// 过滤备忘录 ID
			if allowedIDs != nil && !allowedIDs[derivedID] {
				continue
			}

			// 读取备忘录最新内容（删除事件读取会得到空 DTO，完全匹配语义）
			m, err := h.Memo(ctx, derivedID)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// 过滤是否隐藏
			if filter != nil && filter.Hidden != nil {
				if *filter.Hidden != m.Hidden {
					continue
				}
			}

			if !yield(m, nil) {
				return
			}
		}
	}
}

// #endregion
