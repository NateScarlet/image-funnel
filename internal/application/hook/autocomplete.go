package hook

import (
	"context"

	"main/internal/scalar"
	"main/internal/shared"
)

// Autocomplete 通过钩子脚本获取指令参数自动完成建议
func (h *Handler) Autocomplete(
	ctx context.Context,
	hookID scalar.ID,
	noteRelPath string,
	linePrefix string,
	query string,
) ([]*shared.AutocompleteSuggestionDTO, error) {
	suggestions, err := h.runner.Autocomplete(ctx, hookID, noteRelPath, linePrefix, query)
	if err != nil {
		return nil, err
	}
	var dtos []*shared.AutocompleteSuggestionDTO
	for _, s := range suggestions {
		dtos = append(dtos, h.dtoFactory.NewAutocompleteSuggestion(s))
	}
	return dtos, nil
}
