package image

import (
	"context"
	"main/internal/pagination"
	"main/internal/shared"
)

// TrashHistory 获取回收站历史记录，支持游标分页
func (h *Handler) TrashHistory(
	ctx context.Context,
	first *int,
	after *string,
) (connection *shared.TrashHistoryConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.TrashHistoryItemDTO, *shared.TrashHistoryEdgeDTO, *shared.TrashHistoryConnectionDTO]()
	buf := builder(
		func(item *shared.TrashHistoryItemDTO, cursor string) (*shared.TrashHistoryEdgeDTO, error) {
			return &shared.TrashHistoryEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.TrashHistoryEdgeDTO, pageInfo pagination.PageInfo) (*shared.TrashHistoryConnectionDTO, error) {
			var nodes = make([]*shared.TrashHistoryItemDTO, len(edges))
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
			return &shared.TrashHistoryConnectionDTO{
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

	filteredSeq := func(yield func(*shared.TrashHistoryItemDTO, error) bool) {
		historySeq := h.imgTrasher.FindTrashHistory(ctx)
		for item, err := range historySeq {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(item, nil) {
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