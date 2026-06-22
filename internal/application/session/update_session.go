package session

import (
	"context"
	"main/internal/domain/session"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// UpdateSession 更新会话配置
func (h *Handler) UpdateSession(
	ctx context.Context,
	sessionID scalar.ID,
	targetKeep *int,
	filter *shared.ImageFilters,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update session failed",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did update session",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	var options []session.UpdateOption

	if targetKeep != nil {
		options = append(options, session.WithTargetKeep(*targetKeep))
	}

	if filter != nil {
		options = append(options, session.WithFilter(filter))
	}

	return h.sessionService.Update(ctx, sessionID, options...)
}