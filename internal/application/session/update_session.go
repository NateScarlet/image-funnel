package session

import (
	"context"
	"fmt"
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

	err = h.sessionService.Update(ctx, sessionID, options...)
	if err != nil {
		return err
	}

	// 自动同步修改后的会话配置到历史存储中，用作下次创建会话时的回退配置
	sess, release, errAcquire := h.sessionService.Acquire(ctx, sessionID)
	if errAcquire == nil {
		if err := h.lastSessionSaver.SaveLastSession(ctx, sess.DirectoryID(), sessionID, sess.Filter(), sess.TargetKeep()); err != nil {
			release()
			return fmt.Errorf("failed to save last session: %w", err)
		}
		release()
	}

	return nil
}