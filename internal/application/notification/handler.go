package notification

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"time"

	"main/internal/apperror"
	domnotif "main/internal/domain/notification"
	"main/internal/pagination"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/google/uuid"
)

// Handler 协调通知系统的应用层逻辑
type Handler struct {
	repo          domnotif.Repository
	dtoFactory    *DTOFactory
	filterBuilder *domnotif.FilterBuilder
	topic         pubsub.Topic[*shared.NotificationChangedEventDTO]
}

func NewHandler(
	repo domnotif.Repository,
	dtoFactory *DTOFactory,
	filterBuilder *domnotif.FilterBuilder,
	topic pubsub.Topic[*shared.NotificationChangedEventDTO],
) *Handler {
	return &Handler{
		repo:          repo,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
		topic:         topic,
	}
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
	var na, nb time.Time
	if notAfter != nil {
		na = *notAfter
	}
	if notBefore != nil {
		nb = *notBefore
	}

	existing, err := h.repo.GetByTag(ctx, tag)
	if err != nil {
		return nil, false, err
	}

	var notif *domnotif.Notification
	if existing != nil {
		existing.Update(title, body, priority, na, nb, now)
		notif = existing
	} else {
		id := scalar.ToID(fmt.Sprintf("notif:%s", uuid.NewString()))
		notif = domnotif.FromRepository(
			id, tag, channel, title, body, priority,
			time.Time{}, time.Time{}, na, nb, now, now,
		)
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

			results = append(results, &shared.NotificationChannelDTO{
				Channel:            cs.Channel,
				UnreadCount:        unreadCount,
				LatestNotification: h.dtoFactory.New(matched[0]),
			})
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

	var items []*shared.NotificationDTO
	notifFilter := h.filterBuilder.Build(filters)

	for n, scanErr := range h.repo.Find(ctx, domnotif.FindWithFilter(shared.NotificationFilters{Channel: &channel})) {
		if scanErr != nil {
			return nil, scanErr
		}
		if !notifFilter(n) {
			continue
		}
		dto := h.dtoFactory.New(n)
		items = append(items, dto)
	}

	if cursorStr != "" {
		var filtered []*shared.NotificationDTO
		for _, item := range items {
			if direction == pagination.ODDescend {
				if item.CreatedAt.Before(cursorTime) {
					filtered = append(filtered, item)
				}
			} else {
				if item.CreatedAt.After(cursorTime) {
					filtered = append(filtered, item)
				}
			}
		}
		items = filtered
	}

	slices.SortFunc(items, func(a, b *shared.NotificationDTO) int {
		if direction == pagination.ODDescend {
			if a.CreatedAt.After(b.CreatedAt) {
				return -1
			}
			if a.CreatedAt.Before(b.CreatedAt) {
				return 1
			}
			if a.ID.String() > b.ID.String() {
				return -1
			}
			if a.ID.String() < b.ID.String() {
				return 1
			}
			return 0
		} else {
			if a.CreatedAt.Before(b.CreatedAt) {
				return -1
			}
			if a.CreatedAt.After(b.CreatedAt) {
				return 1
			}
			if a.ID.String() < b.ID.String() {
				return -1
			}
			if a.ID.String() > b.ID.String() {
				return 1
			}
			return 0
		}
	})

	var writer pagination.Writer[*shared.NotificationDTO] = buf
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
		cursor := pagination.TimeToCursor(item.CreatedAt)
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

// SubscribeNotificationChanged 订阅通知变更事件流
func (h *Handler) SubscribeNotificationChanged(ctx context.Context) iter.Seq2[*shared.NotificationChangedEventDTO, error] {
	return func(yield func(*shared.NotificationChangedEventDTO, error) bool) {
		for ev, err := range h.topic.Subscribe(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// Notification 根据 ID 获取单个通知，用于 GraphQL Node 接口解析
func (h *Handler) Notification(ctx context.Context, id scalar.ID) (*shared.NotificationDTO, error) {
	n, err := h.repo.Get(ctx, id.String())
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil // 遵循 DTO 回退与 GraphQL Node 规范
	}
	return h.dtoFactory.New(n), nil
}


