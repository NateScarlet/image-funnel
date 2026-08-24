package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// UpdateLabel 更新图片的标签
func (h *Handler) UpdateLabel(ctx context.Context, sessionID scalar.ID, imageID scalar.ID, label string) (dto *shared.ImageDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update label faield", zap.Stringer("sessionID", sessionID), zap.Stringer("imageID", imageID), zap.Duration("duration", time.Since(startTime)), zap.Error(err))
		} else {
			h.logger.Info("did update label", zap.Stringer("sessionID", sessionID), zap.Stringer("imageID", imageID), zap.Duration("duration", time.Since(startTime)))
		}
	}()

	img, err := h.sessionService.UpdateLabel(ctx, sessionID, imageID, label)
	if err != nil {
		return nil, err
	}

	return h.imageDTOFactory.New(img)
}
