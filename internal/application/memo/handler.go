package memo

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/memo"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"strings"
)

type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	repo          memo.Repository
	service       *memo.Service
	ebus          EventBus
	dtoFactory    *DTOFactory
	filterBuilder *memo.FilterBuilder
}

func NewHandler(repo memo.Repository, service *memo.Service, ebus EventBus, dtoFactory *DTOFactory, filterBuilder *memo.FilterBuilder) *Handler {
	return &Handler{
		repo:          repo,
		service:       service,
		ebus:          ebus,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
	}
}

// UpdateMemo 更新备忘录
func (h *Handler) UpdateMemo(ctx context.Context, id scalar.ID, content string) error {
	// 将更新逻辑及所需依赖提取到 domain/memo/service.go 执行
	return h.service.Save(ctx, id, content)
}

// CreateMemo 创建新备忘录文件，若已存在则返回 ALREADY_EXISTS 错误。
func (h *Handler) CreateMemo(ctx context.Context, directoryID scalar.ID, name string, content string) (*shared.MemoDTO, error) {
	dirRelPath, err := directory.DecodeID(directoryID)
	if err != nil {
		return nil, err
	}
	m, err := h.service.Create(ctx, dirRelPath, name, content)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(m), nil
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
	m, err := h.repo.ReadByRelPath(ctx, relPath)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(m), nil
}

// #region 订阅逻辑

// MemoUpdated 订阅特定备忘录更新（保持老版本向后兼容）
func (h *Handler) MemoUpdated(ctx context.Context, id scalar.ID) iter.Seq2[*shared.MemoDTO, error] {
	return func(yield func(*shared.MemoDTO, error) bool) {
		relPath, err := memo.DecodeID(id)
		if err != nil {
			yield(nil, err)
			return
		}
		for event, err := range h.ebus.SubscribeFileChanged(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// 检查是否是目标备注文件的变更
			if event.RelPath == relPath {
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
		var filters shared.MemoFilters
		if filter != nil {
			filters = *filter
		}

		// 事件级目录粗筛
		var allowedDirectoryIDs util.Set[scalar.ID]
		if len(filters.DirectoryID) > 0 {
			allowedDirectoryIDs = util.AddToSet(nil, filters.DirectoryID...)
		}

		memoFilter := h.filterBuilder.Build(filters)

		for event, err := range h.ebus.SubscribeFileChanged(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !strings.HasSuffix(strings.ToLower(event.RelPath), ".md") {
				continue
			}

			if allowedDirectoryIDs != nil && !allowedDirectoryIDs.Has(event.DirectoryID) {
				continue
			}

			m, err := h.repo.ReadByRelPath(ctx, event.RelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !memoFilter(m) {
				continue
			}

			if !yield(h.dtoFactory.New(m), nil) {
				return
			}
		}
	}
}

// #endregion
