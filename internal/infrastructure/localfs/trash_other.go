//go:build !windows

package localfs

import (
	"os"
)

// moveToRecycleBin 非 Windows 系统退化为直接删除物理文件
func moveToRecycleBin(paths []string) error {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
