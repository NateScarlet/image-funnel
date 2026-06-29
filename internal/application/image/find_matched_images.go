package image

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/shared"
)

// FindMatchedImages 根据筛选条件查询所有匹配的图片列表
func (h *Handler) FindMatchedImages(
	ctx context.Context,
	filterBy shared.ImageFilters,
) ([]*image.Image, error) {
	imgFilter := h.imageFilterBuilder.Build(filterBy)

	// 情况 1: 如果提供了具体的 ID 列表，我们直接获取这些图片，避免目录扫描
	if len(filterBy.ID) > 0 {
		var matched []*image.Image
		for _, id := range filterBy.ID {
			img, err := h.imageService.GetImage(ctx, id)
			if err != nil {
				return nil, err
			}
			if imgFilter(img) {
				matched = append(matched, img)
			}
		}
		return matched, nil
	}

	// 情况 2: 如果提供了 DirectoryID 列表，我们扫描这些目录
	if len(filterBy.DirectoryID) > 0 {
		var matched []*image.Image
		for _, dirID := range filterBy.DirectoryID {
			dirInfo, err := h.dirSvc.GetDirectory(ctx, dirID)
			if err != nil {
				return nil, err
			}
			relPath := dirInfo.RelPath()
			for img, scanErr := range h.imgRepo.Find(ctx, relPath) {
				if scanErr != nil {
					return nil, scanErr
				}
				if imgFilter(img) {
					matched = append(matched, img)
				}
			}
		}
		return matched, nil
	}

	return nil, apperror.New(
		"INVALID_ARGUMENT",
		"either id or directoryId must be provided in filterBy",
		"过滤条件必须包含具体的图片 ID 列表或目录 ID",
	)
}
