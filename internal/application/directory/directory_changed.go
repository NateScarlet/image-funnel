package directory

import (
	"context"
	"iter"
	"main/internal/shared"
)

// DirectoryChanged 订阅目录变更事件
// 根据过滤器返回变更的目录信息
func (h *Handler) DirectoryChanged(ctx context.Context, filters shared.DirectoryFilters) iter.Seq2[*shared.DirectoryDTO, error] {
	return func(yield func(*shared.DirectoryDTO, error) bool) {
		var filter = h.filterBuilder.Build(filters)
		for event, err := range h.fileChangedSub.Subscribe(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				dir, err := h.dirSvc.GetDirectory(ctx, event.DirectoryID)
				if err != nil {
					return yield(nil, err)
				}
				if filter(dir) {
					return yield(h.dtoFactory.New(dir), nil)
				}
				return true
			}() {
				return
			}

		}
	}
}