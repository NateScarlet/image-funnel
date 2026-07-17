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

// Send 发送或覆盖通知，如果 tag 已存在则更新内容，否则创建新通知
func (h *Handler) Send(
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

// Update 更新通知元数据（已读时间、关闭时间）
func (h *Handler) Update(
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

// Unsend 撤回（物理删除）通知
func (h *Handler) Unsend(ctx context.Context, id scalar.ID) (scalar.ID, error) {
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

// NotificationChannels 获取所有通知频道，支持 filterBy 过滤未读数和最新通知
func (h *Handler) NotificationChannels(
	ctx context.Context,
	filters shared.NotificationFilters,
) ([]*shared.NotificationChannelDTO, error) {
	var results []*shared.NotificationChannelDTO

	for cs, err := range h.repo.Channels(ctx) {
		if err != nil {
			return nil, err
		}

		f := filters
		f.Channel = &cs.Channel

		notifFilter := h.filterBuilder.Build(f)
		var matched []*domnotif.Notification
		unreadCount := 0

		for n, err := range h.repo.Find(ctx, domnotif.FindWithFilter(shared.NotificationFilters{Channel: f.Channel})) {
			if err != nil {
				return nil, err
			}
			if notifFilter(n) {
				matched = append(matched, n)
				if n.ReadAt().IsZero() {
					unreadCount++
				}
			}
		}

		if len(matched) > 0 {
			slices.SortFunc(matched, func(a, b *domnotif.Notification) int {
				if a.CreatedAt().After(b.CreatedAt()) {
					return -1
				}
				if a.CreatedAt().Before(b.CreatedAt()) {
					return 1
				}
				return 0
			})

			// 通过 DTOFactory 统一拼装 DTO，保证层级边界
			results = append(results, h.dtoFactory.NewChannelWithData(cs.Channel, unreadCount, matched[0]))
		}
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

// SubscribeNotificationChanged 订阅通知全局实时流，并在首次建立订阅时重发每个频道最新一条未读通知（低优先级不补发）
func (h *Handler) SubscribeNotificationChanged(ctx context.Context) iter.Seq2[*shared.NotificationChangedEventDTO, error] {
	var replayList []*shared.NotificationChangedEventDTO

	for cs, err := range h.repo.Channels(ctx) {
		if err != nil {
			continue
		}

		var latestUnread *domnotif.Notification
		for n, err := range h.repo.Find(ctx, domnotif.FindWithFilter(shared.NotificationFilters{Channel: &cs.Channel})) {
			if err != nil {
				break
			}
			if n.ReadAt().IsZero() {
				latestUnread = n
				break
			}
		}

		if latestUnread != nil {
			// 仅重发非低优先级的未读通知 (LOW 优先级不补发)
			if latestUnread.Priority() != shared.NotificationPriorityLow {
				dto := h.dtoFactory.New(latestUnread)
				replayList = append(replayList, &shared.NotificationChangedEventDTO{
					Event:        shared.NotificationEventTypeSent, // 模拟 SENT 事件以触发展示和 Toast 唤醒
					Notification: dto,
				})
			}
		}
	}

	subSeq := h.topic.Subscribe(ctx)

	return func(yield func(*shared.NotificationChangedEventDTO, error) bool) {
		// 1. 先补发未读通知事件
		for _, event := range replayList {
			if !yield(event, nil) {
				return
			}
		}

		// 2. 然后推送实时的通知事件
		for event, err := range subSeq {
			if !yield(event, err) {
				return
			}
		}
	}
}
