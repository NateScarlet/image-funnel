package memo

import (
	"context"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
)

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
		for m, scanErr := range h.repo.Find(ctx, relPath) {
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