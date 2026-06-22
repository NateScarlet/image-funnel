package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// SuggestDirectories 获取用于自动完成的目录建议列表，限制返回前 50 条并转换为 DTO
func (h *Handler) SuggestDirectories(ctx context.Context, directoryID scalar.ID, input shared.PathInput) ([]*shared.DirectoryDTO, error) {
	dir, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return nil, err
	}

	var results []*shared.DirectoryDTO
	for matchedDir, err := range h.dirSvc.SuggestDirectories(ctx, dir.RelPath(), input) {
		if err != nil {
			return nil, err
		}
		results = append(results, h.dtoFactory.New(matchedDir))
		if len(results) >= 50 {
			break
		}
	}

	return results, nil
}