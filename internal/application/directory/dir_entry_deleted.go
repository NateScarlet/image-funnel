package directory

import (
	"context"
	"iter"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// DirEntryDeleted 订阅目录内的文件/目录被删除或移走（重命名为其他名字）的事件
// 合并 500ms 内的删除事件作为一批响应，避免批量删除时产生巨量响应
func (h *Handler) DirEntryDeleted(ctx context.Context, directoryID *scalar.ID) iter.Seq2[[]*shared.DirEntryDeletedDTO, error] {
	h.logger.Info("will subscribe to dir entry deleted")
	return func(yield func([]*shared.DirEntryDeletedDTO, error) bool) {
		// 将 iter.Seq2 转换为 channel，以便与 select 一起实现非阻塞的批量排空
		type eventItem struct {
			event *shared.FileChangedEvent
			err   error
		}
		eventCh := make(chan eventItem, 128)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		go func() {
			defer close(eventCh)
			for event, err := range h.fileChangedSub.Subscribe(ctx) {
				select {
				case eventCh <- eventItem{event, err}:
				case <-ctx.Done():
					return
				}
			}
		}()

		batchWindow := h.dirEntryDeletedBatchWindow
		if batchWindow == 0 {
			batchWindow = 500 * time.Millisecond
		}

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-eventCh:
				if !ok {
					return
				}
				if item.err != nil {
					h.logger.Error("dir entry deleted event error", zap.Error(item.err))
					yield(nil, item.err)
					return
				}

				var batch []*shared.DirEntryDeletedDTO

				filterAndAppend := func(it eventItem) {
					if it.event.Action != shared.FileActionRemove && it.event.Action != shared.FileActionRename {
						return
					}
					if directoryID != nil && it.event.DirectoryID != *directoryID {
						return
					}
					batch = append(batch, &shared.DirEntryDeletedDTO{
						RelPath: it.event.RelPath,
					})
				}

				filterAndAppend(item)

				// 尝试非阻塞排空当前 channel 里积压的其他事件
			drainLoop:
				for {
					select {
					case nextItem, ok := <-eventCh:
						if !ok {
							break drainLoop
						}
						if nextItem.err != nil {
								h.logger.Error("dir entry deleted event error", zap.Error(nextItem.err))
								yield(nil, nextItem.err)
								return
							}
						filterAndAppend(nextItem)
					default:
						break drainLoop
					}
				}

				// 如果 batch 过滤完后有数据，则立即发送并睡眠限流
				if len(batch) > 0 {
					if !yield(batch, nil) {
						return
					}

					// 睡眠限流周期，监听 ctx.Done() 以便在被取消时立刻退出
					select {
					case <-ctx.Done():
						return
					case <-time.After(batchWindow):
					}
				}
			}
		}
	}
}