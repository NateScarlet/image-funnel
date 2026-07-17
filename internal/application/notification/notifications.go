package notification

import (
	"context"

	domnotif "main/internal/domain/notification"
	"main/internal/pagination"
	"main/internal/shared"
)

// Notifications 获取通知列表，支持过滤及游标分页
func (h *Handler) Notifications(
	ctx context.Context,
	channel *string,
	filters shared.NotificationFilters,
	first *int,
	after *string,
) (conn *shared.NotificationConnectionDTO, err error) {
	if channel != nil {
		filters.Channel = []string{*channel}
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.NotificationDTO, *shared.NotificationEdgeDTO, *shared.NotificationConnectionDTO]()
	buf := builder(
		func(item *shared.NotificationDTO, cursor string) (*shared.NotificationEdgeDTO, error) {
			return &shared.NotificationEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.NotificationEdgeDTO, pageInfo pagination.PageInfo) (*shared.NotificationConnectionDTO, error) {
			var nodes = make([]*shared.NotificationDTO, len(edges))
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
			return &shared.NotificationConnectionDTO{
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

	// repo 通过 filter 保证只返回符合筛选条件的条目，不在此二次筛选
	notificationsSeq := func(yield func(*shared.NotificationDTO, error) bool) {
		for item, err := range h.repo.Find(ctx, domnotif.FindWithFilter(filters)) {
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(h.dtoFactory.New(item), nil) {
				return
			}
		}
	}

	err = pagination.ByIndexE(notificationsSeq, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
}