package image

import (
	"context"
	"iter"
	"main/internal/shared"
	"main/internal/util"
)

// ImageSaved 订阅目录内图片新增/更新事件（CREATE/WRITE）
// 文件创建或写入时，扫描该路径并返回完整 ImageDTO
func (h *Handler) ImageSaved(ctx context.Context, filter *shared.ImageFilters) iter.Seq2[*shared.ImageDTO, error] {
	return func(yield func(*shared.ImageDTO, error) bool) {
		imageFilter := h.imageFilterBuilder.Build(util.UnwrapPointer(filter))
		for event, err := range h.fileChangedSub.Subscribe(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				// XMP 是图片的配套文件，去掉后缀后即图片本体路径；
				// 不是配套文件时原样返回事件路径。判定与裁剪同源于 TrimExtSuffix，
				// 避免大小写不一致的后缀漏裁
				relPath, isXMP, err := event.RelPath.TrimExtSuffix(h.rootDir, ".xmp")
				if err != nil {
					return yield(nil, err)
				}
				isTargetAction := event.Action == shared.FileActionCreate || event.Action == shared.FileActionWrite || (event.Action == shared.FileActionRemove && isXMP)
				if !isTargetAction {
					return true
				}
				img, err := h.imgRepo.Get(ctx, relPath)
				if err != nil || img == nil {
					return true
				}
				if !imageFilter(img) {
					return true
				}
				dto, err := h.dtoFactory.New(img)
				if err != nil {
					return yield(nil, err)
				}
				return yield(dto, nil)
			}() {
				return
			}
		}
	}
}
