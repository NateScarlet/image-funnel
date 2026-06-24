package note

import (
	"context"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
)

// Notes 获取目录下的笔记列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Notes(
	ctx context.Context,
	id scalar.ID,
	filterBy shared.NoteFilters,
	first *int,
	after *string,
) (connection *shared.NoteConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	dirInfo, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.NoteDTO, *shared.NoteEdgeDTO, *shared.NoteConnectionDTO]()
	buf := builder(
		func(item *shared.NoteDTO, cursor string) (*shared.NoteEdgeDTO, error) {
			return &shared.NoteEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.NoteEdgeDTO, pageInfo pagination.PageInfo) (*shared.NoteConnectionDTO, error) {
			var nodes = make([]*shared.NoteDTO, len(edges))
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
			return &shared.NoteConnectionDTO{
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

	filteredSeq := func(yield func(*shared.NoteDTO, error) bool) {
		noteFilter := h.filterBuilder.Build(filterBy)
		for n, scanErr := range h.repo.Find(ctx, relPath) {
			if scanErr != nil {
				if !yield(nil, scanErr) {
					return
				}
				continue
			}
			if !noteFilter(n) {
				continue
			}
			dto := h.dtoFactory.New(n)
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
