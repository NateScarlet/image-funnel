package session

import (
	"context"
	"fmt"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) CreateSession(
	ctx context.Context,
	directoryId scalar.ID,
	filter *shared.ImageFilters,
	target_keep int,
) (sessionID scalar.ID, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("create session failed",
				zap.Stringer("id", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did create session",
				zap.Stringer("id", sessionID),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	sessionID, err = h.sessionService.Create(ctx, directoryId, filter, target_keep)
	if err != nil {
		return sessionID, err
	}

	// 自动同步新建会话配置到历史存储中，用作下次创建会话时的回退配置
	if h.lastSessionSaver != nil {
		if err := h.lastSessionSaver.SaveLastSession(ctx, directoryId, sessionID, filter, target_keep); err != nil {
			return sessionID, fmt.Errorf("failed to save last session: %w", err)
		}
	}

	return sessionID, nil
}