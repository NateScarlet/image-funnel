package hook

import (
	"context"

	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// Autocomplete 通过钩子脚本获取指令参数自动完成建议
func (h *Handler) Autocomplete(
	ctx context.Context,
	hookID scalar.ID,
	noteRelPath string,
	linePrefix string,
	query string,
) (dtos []*shared.AutocompleteSuggestionDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("autocomplete failed",
				zap.Stringer("hookID", hookID),
				zap.String("noteRelPath", noteRelPath),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did autocomplete",
				zap.Stringer("hookID", hookID),
				zap.String("noteRelPath", noteRelPath),
				zap.Int("count", len(dtos)),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	suggestions, err := h.runner.Autocomplete(ctx, hookID, noteRelPath, linePrefix, query)
	if err != nil {
		return nil, err
	}
	var results []*shared.AutocompleteSuggestionDTO
	for _, s := range suggestions {
		results = append(results, h.dtoFactory.NewAutocompleteSuggestion(s))
	}
	return results, nil
}
