//go:build !windows

package localfs

import (
	"fmt"
	"os"
)

// trashOrDelete 非 Windows 系统只支持直接删除物理文件（当 useSystemRecycleBin 为 false 时）。
// 如果用户显式将 useSystemRecycleBin 设为 true，则返回平台不支持的错误。
func trashOrDelete(paths []string, useSystemRecycleBin bool) error {
	if len(paths) == 0 {
		return nil
	}

	if useSystemRecycleBin {
		return fmt.Errorf("system recycle bin is not supported on this platform. please set IMAGE_FUNNEL_USE_SYSTEM_RECYCLE_BIN=false to use direct physical deletion")
	}

	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
