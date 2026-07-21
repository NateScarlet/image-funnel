package note

import (
	"context"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
	"slices"
	"time"
)

// Notes 获取目录下的笔记列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Notes(
	ctx context.Context,
	id scalar.ID,
	filterBy shared.NoteFilters,
	first *int,
	after *string,
) (connection *shared.NoteConnectionDTO, err error) {
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
				return &shared.NoteConnectionDTO{
					Edges:    edges,
					Nodes:    nodes,
					PageInfo: &pageInfo,
				}, nil
			},
	)

	options := pagination.OptionFromInput(after, nil, first, nil)

	relPath := dirInfo.RelPath()

	direction, cursorStr, limit, err := pagination.ByDirection(options, true)
	if err != nil {
		return nil, err
	}

	var cursorTime time.Time
	if cursorStr != "" {
		cursorTime, err = pagination.TimeFromCursor(cursorStr)
		if err != nil {
			return nil, err
		}
	}

	var items []*shared.NoteDTO
	noteFilter := h.filterBuilder.Build(filterBy)
	for n, scanErr := range h.repo.Find(ctx, relPath) {
		if scanErr != nil {
			return nil, scanErr
		}
		if !noteFilter(n) {
			continue
		}
		dto := h.dtoFactory.New(n)
		items = append(items, dto)
	}

	if cursorStr != "" {
		var filtered []*shared.NoteDTO
		for _, item := range items {
			if direction == pagination.ODDescend {
				if item.ModTime.Before(cursorTime) {
					filtered = append(filtered, item)
				}
			} else {
				if item.ModTime.After(cursorTime) {
					filtered = append(filtered, item)
				}
			}
		}
		items = filtered
	}

	slices.SortFunc(items, func(a, b *shared.NoteDTO) int {
		if direction == pagination.ODDescend {
			if a.ModTime.After(b.ModTime) {
				return -1
			}
			if a.ModTime.Before(b.ModTime) {
				return 1
			}
			aStr := a.ID.String()
			bStr := b.ID.String()
			if aStr > bStr {
				return -1
			}
			if aStr < bStr {
				return 1
			}
			return 0
		} else {
			if a.ModTime.Before(b.ModTime) {
				return -1
			}
			if a.ModTime.After(b.ModTime) {
				return 1
			}
			aStr := a.ID.String()
			bStr := b.ID.String()
			if aStr < bStr {
				return -1
			}
			if aStr > bStr {
				return 1
			}
			return 0
		}
	})

	var writer pagination.Writer[*shared.NoteDTO] = buf
	if direction == pagination.ODAscend {
		writer = pagination.NewReverseWriter(buf)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	writer.WriteHasNextPage(hasMore)
	writer.WriteHasPreviousPage(cursorStr != "")

	for _, item := range items {
		cursor := pagination.TimeToCursor(item.ModTime)
		err = writer.Write(item, cursor)
		if err != nil {
			return nil, err
		}
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Value()
}
