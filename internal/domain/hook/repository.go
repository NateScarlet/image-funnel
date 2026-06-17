package hook

import (
	"context"
	"strings"

	"main/internal/scalar"
)

// Hook 外部钩子领域实体
type Hook struct {
	id                 scalar.ID
	name               string
	description        string
	canDispatchByImage bool
}

// FromRepository 从仓库构造外部钩子领域实体，进行安全隔离 ID 编码
func FromRepository(rawID string, name, description string, canDispatchByImage bool) *Hook {
	return &Hook{
		id:                 encodeID(rawID),
		name:               name,
		description:        description,
		canDispatchByImage: canDispatchByImage,
	}
}

func encodeID(rawID string) scalar.ID {
	if strings.HasPrefix(rawID, "hk:") {
		return scalar.ToID(rawID)
	}
	return scalar.ToID("hk:" + rawID)
}

func decodeID(id scalar.ID) string {
	return strings.TrimPrefix(id.String(), "hk:")
}

func (h *Hook) ID() scalar.ID              { return h.id }
func (h *Hook) Name() string               { return h.name }
func (h *Hook) Description() string        { return h.description }
func (h *Hook) CanDispatchByImage() bool   { return h.canDispatchByImage }

// Repository 钩子领域持久化层接口
type Repository interface {
	List(ctx context.Context) ([]*Hook, error)
}

// Runner 外部钩子执行器接口
type Runner interface {
	Trigger(ctx context.Context, ids []string, paths []string, hookID scalar.ID, triggerName string) error
}
