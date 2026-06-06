package device

import (
	"context"
	"iter"

	"main/internal/scalar"
)

// Repository 负责持久化 Device 实体
type Repository interface {
	Save(ctx context.Context, device *Device) error
	Get(ctx context.Context, id scalar.ID) (*Device, error)
	Delete(ctx context.Context, id scalar.ID) error
	Find(ctx context.Context) iter.Seq2[*Device, error]
}