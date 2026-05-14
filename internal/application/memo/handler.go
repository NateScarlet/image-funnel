package memo

import (
	"context"
	"iter"
	"main/internal/domain/memo"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
	"strings"
)

type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	repo       memo.Repository
	ebus       EventBus
	dtoFactory *DTOFactory
}

func NewHandler(repo memo.Repository, ebus EventBus, rootDir string) *Handler {
	return &Handler{
		repo:       repo,
		ebus:       ebus,
		dtoFactory: NewDTOFactory(rootDir),
	}
}

// UpdateMemo 更新备忘录
func (h *Handler) UpdateMemo(ctx context.Context, id scalar.ID, content string) error {
	return h.repo.Write(ctx, id, content)
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

// MemoUpdated 订阅备忘录更新
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
