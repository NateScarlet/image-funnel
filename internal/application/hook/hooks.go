package hook

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// Hooks 获取所有外部钩子概要列表
func (h *Handler) Hooks(ctx context.Context) (dtos []*shared.HookDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("hooks failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did hooks",
				zap.Int("count", len(dtos)),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	hooks, err := h.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	var res []*shared.HookDTO
	for _, hk := range hooks {
		res = append(res, h.dtoFactory.New(hk))
	}
	return res, nil
}