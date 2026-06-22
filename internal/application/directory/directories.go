package directory

import (
	"context"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
)

// Directories 获取子目录列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Directories(
	ctx context.Context,
	parentID scalar.ID,
	filterBy shared.DirectoryFilters,
	first *int,
	after *string,
) (connection *shared.DirectoryConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	parentDir, err := h.dirSvc.GetDirectory(ctx, parentID)
	if err != nil {
		return nil, err
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.DirectoryDTO, *shared.DirectoryEdgeDTO, *shared.DirectoryConnectionDTO]()
	buf := builder(
		func(item *shared.DirectoryDTO, cursor string) (*shared.DirectoryEdgeDTO, error) {
			return &shared.DirectoryEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.DirectoryEdgeDTO, pageInfo pagination.PageInfo) (*shared.DirectoryConnectionDTO, error) {
			var nodes = make([]*shared.DirectoryDTO, len(edges))
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
			return &shared.DirectoryConnectionDTO{
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

	filteredSeq := func(yield func(*shared.DirectoryDTO, error) bool) {
		dirFilter := h.filterBuilder.Build(filterBy)
		for dir, scanErr := range h.repo.Find(ctx, parentDir.RelPath()) {
			if scanErr != nil {
				if !yield(nil, scanErr) {
					return
				}
				continue
			}
			if !dirFilter(dir) {
				continue
			}
			dto := h.dtoFactory.New(dir)
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