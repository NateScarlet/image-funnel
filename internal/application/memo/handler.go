package memo

import (
	"context"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/domain/memo"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"path/filepath"
	"strings"
)

type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	repo          memo.Repository
	service       *memo.Service
	memoScanner   memo.Scanner
	dirSvc        *directory.Service
	ebus          EventBus
	dtoFactory    *DTOFactory
	filterBuilder *memo.FilterBuilder
}

func NewHandler(repo memo.Repository, service *memo.Service, memoScanner memo.Scanner, dirSvc *directory.Service, ebus EventBus, dtoFactory *DTOFactory, filterBuilder *memo.FilterBuilder) *Handler {
	return &Handler{
		repo:          repo,
		service:       service,
		memoScanner:   memoScanner,
		dirSvc:        dirSvc,
		ebus:          ebus,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
	}
}

// UpdateMemo 更新备忘录
func (h *Handler) UpdateMemo(ctx context.Context, id scalar.ID, content string) error {
	return h.service.Save(ctx, id, content)
}

// CreateMemo 创建新备忘录文件，若已存在则返回 ALREADY_EXISTS 错误。
func (h *Handler) CreateMemo(ctx context.Context, directoryID scalar.ID, name string, content string) (*shared.MemoDTO, error) {
	dir, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return nil, err
	}
	m, err := h.service.Create(ctx, dir.RelPath(), name, content)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(m), nil
}

// Memo 获取备忘录内容
func (h *Handler) Memo(ctx context.Context, id scalar.ID) (*shared.MemoDTO, error) {
	m, err := h.service.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(m), nil
}

// MemoByRelPath 根据相对路径获取备忘录内容
func (h *Handler) MemoByRelPath(ctx context.Context, relPath string) (*shared.MemoDTO, error) {
	m, err := h.repo.Read(ctx, relPath)
	if err != nil {
		// 若读取到了非文本文件，则视同文件不存在进行降级处理，返回空备忘对象
		if apperror.ErrCode(err) == "NOT_TEXT" {
			absPath := filepath.Join(h.dtoFactory.rootDir, relPath)
			m = memo.NewEmpty(relPath, absPath)
			return h.dtoFactory.New(m), nil
		}
		return nil, err
	}
	if m == nil {
		// 返回一个空的 MemoDTO 以支持 GraphQL Schema 中的 Memo! 约束
		absPath := filepath.Join(h.dtoFactory.rootDir, relPath)
		m = memo.NewEmpty(relPath, absPath)
	}
	return h.dtoFactory.New(m), nil
}

// #region 订阅逻辑

// MemoUpdated 订阅特定备忘录更新（保持老版本向后兼容）
func (h *Handler) MemoUpdated(ctx context.Context, id scalar.ID) iter.Seq2[*shared.MemoDTO, error] {
	return func(yield func(*shared.MemoDTO, error) bool) {
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

			m, err := h.repo.Read(ctx, event.RelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if m == nil || m.ID() != id {
				continue
			}

			if !yield(h.dtoFactory.New(m), nil) {
				return
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

			m, err := h.repo.Read(ctx, event.RelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if m == nil || !memoFilter(m) {
				continue
			}

			if !yield(h.dtoFactory.New(m), nil) {
				return
			}
		}
	}
}

// #endregion

// Memos 获取目录下的备忘录列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Memos(
	ctx context.Context,
	id scalar.ID,
	filterBy shared.MemoFilters,
	first *int,
	after *string,
) (connection *shared.MemoConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	dirInfo, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.MemoDTO, *shared.MemoEdgeDTO, *shared.MemoConnectionDTO]()
	buf := builder(
		func(item *shared.MemoDTO, cursor string) (*shared.MemoEdgeDTO, error) {
			return &shared.MemoEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.MemoEdgeDTO, pageInfo pagination.PageInfo) (*shared.MemoConnectionDTO, error) {
			var nodes = make([]*shared.MemoDTO, len(edges))
			for i, edge := range edges {
				nodes[i] = edge.Node
			}
			var startCursor, endCursor string
			if pageInfo.StartCursor != nil {
				startCursor = *pageInfo.StartCursor
			}
			if pageInfo.EndCursor != nil {
				endCursor = *pageInfo.EndCursor
			}
			return &shared.MemoConnectionDTO{
				Edges: edges,
				Nodes: nodes,
				PageInfo: &shared.PageInfoDTO{
					HasNextPage:     pageInfo.HasNextPage,
					HasPreviousPage: pageInfo.HasPreviousPage,
					StartCursor:     startCursor,
					EndCursor:       endCursor,
				},
			}, nil
		},
	)

	options := pagination.OptionFromInput(after, nil, first, nil)

	relPath := dirInfo.RelPath()

	filteredSeq := func(yield func(*shared.MemoDTO, error) bool) {
		memoFilter := h.filterBuilder.Build(filterBy)
		for m, scanErr := range h.memoScanner.Scan(ctx, relPath) {
			if scanErr != nil {
				if !yield(nil, scanErr) {
					return
				}
				continue
			}
			if !memoFilter(m) {
				continue
			}
			dto := h.dtoFactory.New(m)
			if !yield(dto, nil) {
				return
			}
		}
	}

	err = pagination.ByIndexE(filteredSeq, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
}