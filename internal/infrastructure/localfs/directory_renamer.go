package localfs

import (
	"context"
	"fmt"
	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"os"
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
func (r *DirectoryRenamer) Rename(ctx context.Context, relPath relpath.Path, newName string) (relpath.Path, error) {
	// 新名由用户输入，拼接结果重新围栏以拒绝「../..」之类的目标名
	newRelPath, err := relPath.Dir().Join(r.rootDir, newName)
	if err != nil {
		return relpath.Path{}, err
	}

	dstAbsPath := newRelPath.Abs(r.rootDir)

	// 必须显式检查目标名是否被占用：Windows 的 os.Rename 会用目录直接覆盖同名的文件，
	// Linux 会覆盖空目录，两种平台都不能让重命名静默破坏同级已有条目。
	if _, err := os.Lstat(dstAbsPath); err == nil {
		return relpath.Path{}, apperror.New(
			"DIRECTORY_ALREADY_EXISTS",
			fmt.Sprintf("an entry named %q already exists", newName),
			fmt.Sprintf("同级目录下已存在名为 %q 的条目", newName),
		)
	} else if !os.IsNotExist(err) {
		return relpath.Path{}, fmt.Errorf("failed to stat target directory %s: %w", newRelPath, err)
	}

	if err := os.Rename(relPath.Abs(r.rootDir), dstAbsPath); err != nil {
		return relpath.Path{}, fmt.Errorf("failed to rename directory %s to %s: %w", relPath, newRelPath, err)
	}

	return newRelPath, nil
}

var _ directory.Renamer = (*DirectoryRenamer)(nil)
