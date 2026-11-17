//go:build !windows

package localfs

import "fmt"

// systemTrashSweeper 非 Windows 平台不支持系统回收站，
// 保持既有行为：不在启动时拒绝服务，而是把错误推迟到实际清扫时返回。
type systemTrashSweeper struct{}

// NewSystemTrashSweeper 创建非 Windows 平台的系统回收站清扫 adapter。
func NewSystemTrashSweeper() TrashSweeper {
	return systemTrashSweeper{}
}

func (systemTrashSweeper) Sweep(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return fmt.Errorf("system recycle bin is not supported on this platform. please set IMAGE_FUNNEL_USE_SYSTEM_RECYCLE_BIN=false to use direct physical deletion")
}
