package notification

import (
	"context"

	"main/internal/pagination"
	"main/internal/shared"
)

// NotificationChannels 获取所有通知频道
func (h *Handler) NotificationChannels(
	ctx context.Context,
	first *int,
	after *string,
) (conn *shared.NotificationChannelConnectionDTO, err error) {
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

	// 直接使用 repo 的 Channels 流式处理，按 repo 返回的时间顺序
	chSeq := func(yield func(*shared.NotificationChannelDTO, error) bool) {
		for cs, err := range h.repo.Channels(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(h.dtoFactory.NewChannel(cs), nil) {
				return
			}
		}
	}

	err = pagination.ByIndexE(chSeq, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
}