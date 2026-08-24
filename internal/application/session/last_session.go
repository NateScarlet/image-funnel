package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// LastSession 获取指定目录下最后更新的会话 DTO
func (h *Handler) LastSession(ctx context.Context, directoryID scalar.ID) (dto *shared.SessionDTO, err error) {
	sess, release, err := h.sessionService.LastSession(ctx, directoryID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil
	}
	defer release()

	return h.dtoFactory.New(sess)
}
