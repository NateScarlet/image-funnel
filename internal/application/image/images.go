package image

import (
	"context"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
	"slices"
	"time"
)

// Images 获取目录下的图片列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Images(
	ctx context.Context,
	id scalar.ID,
	filterBy shared.ImageFilters,
	first *int,
	after *string,
) (connection *shared.ImageConnectionDTO, err error) {
	dirInfo, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}

	imgFilter := h.imageFilterBuilder.Build(filterBy)

	builder := pagination.NewConnectionBufferBuilder[*shared.ImageDTO, *shared.ImageEdgeDTO, *shared.ImageConnectionDTO]()
	buf := builder(
		func(item *shared.ImageDTO, cursor string) (*shared.ImageEdgeDTO, error) {
			return &shared.ImageEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.ImageEdgeDTO, pageInfo pagination.PageInfo) (*shared.ImageConnectionDTO, error) {
				var nodes = make([]*shared.ImageDTO, len(edges))
				for i, edge := range edges {
					nodes[i] = edge.Node
				}
				return &shared.ImageConnectionDTO{
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

	var items []*shared.ImageDTO
	for img, scanErr := range h.imgRepo.Find(ctx, relPath) {
		if scanErr != nil {
			return nil, scanErr
		}
		if !imgFilter(img) {
			continue
		}
		dto, factoryErr := h.dtoFactory.New(img)
		if factoryErr != nil {
			return nil, factoryErr
		}
		items = append(items, dto)
	}

	if cursorStr != "" {
		var filtered []*shared.ImageDTO
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

	slices.SortFunc(items, func(a, b *shared.ImageDTO) int {
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

	var writer pagination.Writer[*shared.ImageDTO] = buf
	if direction == pagination.ODAscend {
		writer = pagination.NewReverseWriter[*shared.ImageDTO](buf)
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