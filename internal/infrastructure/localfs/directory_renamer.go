package localfs

import (
	"context"
	"fmt"
	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/util"
	"os"
	"path/filepath"
)

// DirectoryRenamer 专职处理目录的物理重命名
type DirectoryRenamer struct {
	rootDir string
}

// NewDirectoryRenamer 创建目录重命名实现实例
func NewDirectoryRenamer(rootDir string) *DirectoryRenamer {
	return &DirectoryRenamer{
		rootDir: rootDir,
	}
}

// Rename 将 relPath 指向的目录重命名为同一父目录下的 newName。
// 目标名被占用时返回业务错误，重命名保持原状。
func (r *DirectoryRenamer) Rename(ctx context.Context, relPath string, newName string) (string, error) {
	srcAbsPath := filepath.Join(r.rootDir, relPath)
	newRelPath := filepath.Join(filepath.Dir(relPath), newName)
	dstAbsPath := filepath.Join(r.rootDir, newRelPath)

	// 安全校验：确保目标路径仍在配置的根目录范围内，防止目录穿越
	if err := util.EnsurePathInRoot(r.rootDir, newRelPath); err != nil {
		return "", err
	}

	// 必须显式检查目标名是否被占用：Windows 的 os.Rename 会用目录直接覆盖同名的文件，
	// Linux 会覆盖空目录，两种平台都不能让重命名静默破坏同级已有条目。
	if _, err := os.Lstat(dstAbsPath); err == nil {
		return "", apperror.New(
			"DIRECTORY_ALREADY_EXISTS",
			fmt.Sprintf("an entry named %q already exists", newName),
			fmt.Sprintf("同级目录下已存在名为 %q 的条目", newName),
		)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to stat target directory %s: %w", newRelPath, err)
	}

	if err := os.Rename(srcAbsPath, dstAbsPath); err != nil {
		return "", fmt.Errorf("failed to rename directory %s to %s: %w", relPath, newRelPath, err)
	}

	return filepath.ToSlash(newRelPath), nil
}

var _ directory.Renamer = (*DirectoryRenamer)(nil)
