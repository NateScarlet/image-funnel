package hook

import (
	"path/filepath"

	"main/internal/domain/relpath"
)

// relFromAbsPath 从绝对路径提取根相对路径。
// 文件系统遍历与外部传入的绝对路径是构造根相对路径的出生点之一，
// 因此围栏只在这里做一次，调用方拿到围栏内的 relpath.Path。
func relFromAbsPath(rootDir string, absPath string) (relpath.Path, error) {
	relStr, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return relpath.Path{}, err
	}
	return relpath.New(rootDir, relStr)
}
