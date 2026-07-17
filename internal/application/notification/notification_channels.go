package notification

import (
	"context"
	"slices"

	domnotif "main/internal/domain/notification"
	"main/internal/pagination"
	"main/internal/shared"
)

// NotificationChannels 获取所有通知频道
func (h *Handler) NotificationChannels(
	ctx context.Context,
	filters shared.NotificationFilters,
	first *int,
	after *string,
) (conn *shared.NotificationChannelConnectionDTO, err error) {
	cs, err := h.service.Channels(ctx, filters)
	if err != nil {
		return nil, err
	}

	// 按频道名称字母排序
	slices.SortFunc(cs, func(a, b *domnotif.ChannelStats) int {
		if a.Channel < b.Channel {
			return -1
		}
		if a.Channel > b.Channel {
			return 1
		}
		return 0
	})

	// 转换为 DTO 用于分页
	var allDTOs []*shared.NotificationChannelDTO
	for _, c := range cs {
		allDTOs = append(allDTOs, h.dtoFactory.NewChannel(c))
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.NotificationChannelDTO, *shared.NotificationChannelEdgeDTO, *shared.NotificationChannelConnectionDTO]()
	buf := builder(
		func(item *shared.NotificationChannelDTO, cursor string) (*shared.NotificationChannelEdgeDTO, error) {
			return &shared.NotificationChannelEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.NotificationChannelEdgeDTO, pageInfo pagination.PageInfo) (*shared.NotificationChannelConnectionDTO, error) {
			var startCursor, endCursor string
			if pageInfo.StartCursor != nil {
				startCursor = *pageInfo.StartCursor
			}
			if pageInfo.EndCursor != nil {
				endCursor = *pageInfo.EndCursor
			}
			return &shared.NotificationChannelConnectionDTO{
				Edges: edges,
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

	allSeq := func(yield func(*shared.NotificationChannelDTO, error) bool) {
		for _, dto := range allDTOs {
			if !yield(dto, nil) {
				return
			}
		}
	}

	err = pagination.ByIndexE(allSeq, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
}