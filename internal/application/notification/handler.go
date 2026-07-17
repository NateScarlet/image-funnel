package notification

import (
	"context"
	"iter"
	"slices"
	"time"

	"main/internal/apperror"
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

// SendNotification 发送或覆盖通知，如果 tag 已存在则更新内容，否则创建新通知
func (h *Handler) SendNotification(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	body string,
	priority shared.NotificationPriority,
	notAfter *time.Time,
	notBefore *time.Time,
) (*shared.NotificationDTO, bool, error) {
	now := time.Now()
	var notAfterVal, notBeforeVal time.Time
	if notAfter != nil {
		notAfterVal = *notAfter
	}
	if notBefore != nil {
		notBeforeVal = *notBefore
	}

	existing, err := h.repo.GetByTag(ctx, tag)
	if err != nil {
		return nil, false, err
	}

	var notif *domnotif.Notification
	if existing != nil {
		existing.Update(title, body, priority, notAfterVal, notBeforeVal, now)
		notif = existing
	} else {
		// 由领域 Service 统一构造新实体，实现 ID 的内聚生成与封装
		notif = h.service.CreateNew(tag, channel, title, body, priority, notAfterVal, notBeforeVal)
	}

	didCreate, err := h.repo.Save(ctx, notif)
	if err != nil {
		return nil, false, err
	}

	dto := h.dtoFactory.New(notif)

	eventType := shared.NotificationEventTypeSent
	if !didCreate {
		eventType = shared.NotificationEventTypeUpdated
	}
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        eventType,
		Notification: dto,
	})

	return dto, didCreate, nil
}

// UpdateNotification 更新通知元数据（已读时间、关闭时间）
func (h *Handler) UpdateNotification(
	ctx context.Context,
	id scalar.ID,
	readAt *time.Time,
	dismissedAt *time.Time,
) (*shared.NotificationDTO, error) {
	notif, err := h.repo.Get(ctx, id.String())
	if err != nil {
		return nil, err
	}
	if notif == nil {
		return nil, apperror.New("NOT_FOUND", "notification not found", "未找到指定通知")
	}

	if readAt != nil {
		if readAt.IsZero() {
			notif.MarkRead(time.Time{})
		} else {
			notif.MarkRead(*readAt)
		}
	}

	if dismissedAt != nil {
		if dismissedAt.IsZero() {
			notif.Dismiss(time.Time{})
		} else {
			notif.Dismiss(*dismissedAt)
		}
	}

	_, err = h.repo.Save(ctx, notif)
	if err != nil {
		return nil, err
	}

	dto := h.dtoFactory.New(notif)
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        shared.NotificationEventTypeUpdated,
		Notification: dto,
	})

	return dto, nil
}

// UnsendNotification 撤回（物理删除）通知
func (h *Handler) UnsendNotification(ctx context.Context, id scalar.ID) (scalar.ID, error) {
	notif, err := h.repo.Get(ctx, id.String())
	if err != nil {
		return id, err
	}
	if notif == nil {
		return id, apperror.New("NOT_FOUND", "notification not found", "未找到指定通知")
	}

	// 标记删除状态
	notif.MarkDeleted()

	// 物理删除
	_, err = h.repo.Save(ctx, notif)
	if err != nil {
		return id, err
	}

	// 触发推送，通知前端撤销该通知展示
	dto := h.dtoFactory.New(notif)
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        shared.NotificationEventTypeUnsent,
		Notification: dto,
	})

	return id, nil
}

// NotificationChannels 获取所有通知频道
func (h *Handler) NotificationChannels(
	ctx context.Context,
	filters shared.NotificationFilters,
) ([]*shared.NotificationChannelDTO, error) {
	var results []*shared.NotificationChannelDTO

	for cs, err := range h.repo.Channels(ctx) {
		if err != nil {
			return nil, err
		}

		// 委派给 DTOFactory.NewChannel 统一构造，保证 DTO 边界内聚与减少 Feature Envy
		results = append(results, h.dtoFactory.NewChannel(cs))
	}

	// 按频道名称字母排序返回
	slices.SortFunc(results, func(a, b *shared.NotificationChannelDTO) int {
		if a.Channel < b.Channel {
			return -1
		}
		if a.Channel > b.Channel {
			return 1
		}
		return 0
	})

	return results, nil
}

// Notifications 获取通知列表，支持过滤及游标分页
func (h *Handler) Notifications(
	ctx context.Context,
	channel string,
	filters shared.NotificationFilters,
	first *int,
	after *string,
) (*shared.NotificationConnectionDTO, error) {
	filters.Channel = &channel

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
		for item, err := range h.repo.Find(ctx, domnotif.FindWithFilter(shared.NotificationFilters{Channel: &channel})) {
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

// SubscribeNotificationChanged 订阅通知全局实时流
func (h *Handler) SubscribeNotificationChanged(ctx context.Context) iter.Seq2[*shared.NotificationChangedEventDTO, error] {
	return h.topic.Subscribe(ctx)
}
