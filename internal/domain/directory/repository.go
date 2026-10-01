package directory

import (
	"context"
	"iter"
	"main/internal/shared"
)

type Repository interface {
	Get(ctx context.Context, relPath string) (*Directory, error)
	// Find 迭代扫描目标目录下的直接子目录结构
	Find(ctx context.Context, relPath string) iter.Seq2[*Directory, error]
	ReadState(ctx context.Context, relPath string) (*shared.DirectoryStateDTO, error)
	WriteState(ctx context.Context, relPath string, state *shared.DirectoryStateDTO) error
}

// Renamer 负责目录的物理重命名。
// 与只读加状态读写的 Repository 分离，因为重命名是写操作，且目标名冲突需要由文件系统判定。
type Renamer interface {
	// Rename 将 relPath 指向的目录重命名为同一父目录下的 newName，
	// 返回重命名后目录相对于根目录的路径。目标名冲突时返回错误。
	Rename(ctx context.Context, relPath string, newName string) (newRelPath string, err error)
}
