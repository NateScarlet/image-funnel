package directory

import (
	"context"
	"iter"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"time"

	"go.uber.org/zap"
)

func NewDirectoryChangedOptions(options ...DirectoryChangedOption) *DirectoryChangedOptions {
	var opts = new(DirectoryChangedOptions)
	opts.throttle = time.Second
	for _, i := range options {
		i(opts)
	}
	return opts
}

type DirectoryChangedOptions struct {
	throttle time.Duration
}

type DirectoryChangedOption func(*DirectoryChangedOptions)

func DirectoryChangedWithThrottle(throttle time.Duration) DirectoryChangedOption {
	return func(opts *DirectoryChangedOptions) {
		opts.throttle = throttle
	}
}

// DirectoryChanged 订阅目录变更事件
// 根据过滤器返回变更的目录信息
func (h *Handler) DirectoryChanged(ctx context.Context, filters shared.DirectoryFilters, options ...DirectoryChangedOption) iter.Seq2[*shared.DirectoryDTO, error] {
	var opts = NewDirectoryChangedOptions(options...)

	h.logger.Info("will subscribe to directory changed",
		zap.Duration("throttle", opts.throttle),
	)

	keyOf := func(event *shared.FileChangedEvent, err error) scalar.ID {
		if err != nil {
			var zero scalar.ID
			return zero
		}
		return event.DirectoryID
	}

	throttledSeq := util.ThrottleBy(h.fileChangedSub.Subscribe(ctx), opts.throttle, keyOf)

	return func(yield func(*shared.DirectoryDTO, error) bool) {
		var filter = h.filterBuilder.Build(filters)
		for event, err := range throttledSeq {
			if err != nil {
				h.logger.Error("directory changed event error", zap.Error(err))
				if !yield(nil, err) {
					return
				}
				continue
			}

			dir, err := h.dirSvc.GetDirectory(ctx, event.DirectoryID)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if filter(dir) {
				if !yield(h.dtoFactory.New(dir), nil) {
					return
				}
			}
		}
	}
}
