package notification

import (
	"context"
	"iter"
	"log/slog"
	"slices"
	"time"

	domnotif "main/internal/domain/notification"
	"main/internal/pagination"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
)

// Handler 协调通知系统的应用层逻辑
type Handler struct {
	repo          domnotif.Repository
	service       *domnotif.Service
	dtoFactory    *DTOFactory
	filterBuilder *domnotif.FilterBuilder
	topic         pubsub.Topic[*shared.NotificationChangedEventDTO]
}

func NewHandler(
	repo domnotif.Repository,
	service *domnotif.Service,
	dtoFactory *DTOFactory,
	filterBuilder *domnotif.FilterBuilder,
	topic pubsub.Topic[*shared.NotificationChangedEventDTO],
) *Handler {
	return &Handler{
		repo:          repo,
		service:       service,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
		topic:         topic,
	}
}

// Notification 获取单条通知
func (h *Handler) Notification(ctx context.Context, id scalar.ID) (*shared.NotificationDTO, error) {
	notif, err := h.repo.Get(ctx, id.String())
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(notif), nil
}

// #region SendNotification

// SendNotificationOptions 发送通知的选项
type SendNotificationOptions struct {
	NotAfter   time.Time
	NotBefore  time.Time
	DetailsURL scalar.URI
}

// SendNotificationOption 发送通知的选项函数
type SendNotificationOption func(*SendNotificationOptions)

// WithNotAfter 设置过期时间
func WithNotAfter(t time.Time) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.NotAfter = t }
}

// WithNotBefore 设置最早可见时间
func WithNotBefore(t time.Time) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.NotBefore = t }
}

// WithDetailsURL 设置详情 URL
func WithDetailsURL(u scalar.URI) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.DetailsURL = u }
}

// SendNotification 发送或覆盖通知
func (h *Handler) SendNotification(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	body string,
	priority shared.NotificationPriority,
	opts ...SendNotificationOption,
) (*shared.NotificationDTO, bool, error) {
	var options SendNotificationOptions
	for _, o := range opts {
		o(&options)
	}

	result, err := h.service.SendNotification(ctx, domnotif.SendNotificationInput{
		Tag:        tag,
		Channel:    channel,
		Title:      title,
		Body:       body,
		Priority:   priority,
		NotAfter:   options.NotAfter,
		NotBefore:  options.NotBefore,
		DetailsURL: options.DetailsURL,
	})
	if err != nil {
		return nil, false, err
	}

	dto := h.dtoFactory.New(result.Notification)

	eventType := shared.NotificationEventTypeSent
	if !result.DidCreate {
		eventType = shared.NotificationEventTypeUpdated
	}
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        eventType,
		Notification: dto,
	})

	slog.Debug("sendNotification", "tag", tag, "didCreate", result.DidCreate)

	return dto, result.DidCreate, nil
}

// #endregion

// #region UpdateNotification

// UpdateNotification 更新通知元数据（已读时间、关闭时间）
func (h *Handler) UpdateNotification(
	ctx context.Context,
	id scalar.ID,
	readAt *time.Time,
	dismissedAt *time.Time,
) (*shared.NotificationDTO, error) {
	notif, err := h.service.UpdateNotification(ctx, id, readAt, dismissedAt)
	if err != nil {
		return nil, err
	}

	dto := h.dtoFactory.New(notif)
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        shared.NotificationEventTypeUpdated,
		Notification: dto,
	})

	slog.Debug("updateNotification", "id", id)

	return dto, nil
}

// #endregion

// #region UnsendNotification

// UnsendNotification 撤回（删除）通知
func (h *Handler) UnsendNotification(ctx context.Context, tag string) error {
	err := h.service.UnsendNotification(ctx, tag)
	if err != nil {
		return err
	}

	slog.Debug("unsendNotification", "tag", tag)

	return nil
}

// #endregion

// #region NotificationChannels

// NotificationChannels 获取所有通知频道
func (h *Handler) NotificationChannels(
	ctx context.Context,
	filters shared.NotificationFilters,
	first *int,
	after *string,
) (*shared.NotificationChannelConnectionDTO, error) {
	cs, err := h.service.GetChannels(ctx, filters)
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
			var nodes = make([]*shared.NotificationChannelDTO, len(edges))
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
			return &shared.NotificationChannelConnectionDTO{
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

// #endregion

// #region Notifications

// Notifications 获取通知列表，支持过滤及游标分页
func (h *Handler) Notifications(
	ctx context.Context,
	channel string,
	filters shared.NotificationFilters,
	first *int,
	after *string,
) (*shared.NotificationConnectionDTO, error) {
	filters.Channel = []string{channel}

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
	notifFilter := h.filterBuilder.Build(filters)

	filteredSeq := func(yield func(*shared.NotificationDTO, error) bool) {
		for item, err := range h.repo.Find(ctx, domnotif.FindWithFilter(shared.NotificationFilters{Channel: []string{channel}})) {
			if err != nil {
				yield(nil, err)
				return
			}

			if !notifFilter(item) {
				continue
			}

			if !yield(h.dtoFactory.New(item), nil) {
				return
			}
		}
	}

	err := pagination.ByIndexE(filteredSeq, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
}

// #endregion

// SubscribeNotificationChanged 订阅通知全局实时流
func (h *Handler) SubscribeNotificationChanged(ctx context.Context) iter.Seq2[*shared.NotificationChangedEventDTO, error] {
	return h.topic.Subscribe(ctx)
}