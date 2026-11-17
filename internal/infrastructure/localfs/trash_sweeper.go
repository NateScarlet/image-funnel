package localfs

import (
	"fmt"
	"os"

	"main/internal/util"
)

// TrashSweeper 负责物理清扫已标记删除的回收站目录。
// 具体语义由注入的 adapter 决定：或投递系统回收站，或直接物理删除。
type TrashSweeper interface {
	// Sweep 清扫给定路径，失败时返回调用者可见的错误。
	Sweep(paths []string) error
}

// physicalDeleteSweeper 直接物理删除路径，不依赖任何平台回收站能力。
type physicalDeleteSweeper struct{}

// NewPhysicalDeleteSweeper 创建直接物理删除的清扫 adapter。
func NewPhysicalDeleteSweeper() TrashSweeper {
	return physicalDeleteSweeper{}
}

func (physicalDeleteSweeper) Sweep(paths []string) error {
	// 各路径互不依赖：逐项删除并合并错误，避免首错中断导致后续路径未被尝试。
	errB := util.NewErrorsBuilder(len(paths))
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			errB.Add(fmt.Errorf("failed to remove %s: %w", p, err))
		}
	}
	return errB.Build()
}
