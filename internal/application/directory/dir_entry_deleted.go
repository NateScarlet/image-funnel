package directory

import (
	"context"
	"iter"
	"main/internal/scalar"
	"main/internal/shared"
)

// DirEntryDeleted 订阅目录内的文件/目录被删除或移走（重命名为其他名字）的事件
func (h *Handler) DirEntryDeleted(ctx context.Context, directoryID *scalar.ID) iter.Seq2[*shared.DirEntryDeletedDTO, error] {
	return func(yield func(*shared.DirEntryDeletedDTO, error) bool) {
		for event, err := range h.fileChangedSub.Subscribe(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				if event.Action != shared.FileActionRemove && event.Action != shared.FileActionRename {
					return true
				}
				if directoryID != nil && event.DirectoryID != *directoryID {
					return true
				}

				return yield(&shared.DirEntryDeletedDTO{
					RelPath: event.RelPath,
				}, nil)
			}() {
				return
			}
		}
	}
}