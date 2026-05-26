package localfs

import (
	"context"
	"fmt"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/memo"
	"main/internal/util"
	"os"
	"path/filepath"
	"strings"
)

type MemoRepository struct {
	rootDir string
}

func NewMemoRepository(rootDir string) *MemoRepository {
	return &MemoRepository{
		rootDir: rootDir,
	}
}

// isText 检查前 1024 字节是否包含空字符以判定其是否为文本文件
func isText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	limit := len(data)
	if limit > 1024 {
		limit = 1024
	}
	for i := 0; i < limit; i++ {
		if data[i] == 0x00 {
			return false
		}
	}
	return true
}

func (r *MemoRepository) Read(ctx context.Context, relPath string) (*memo.Memo, error) {
	if err := util.EnsurePathInRoot(r.rootDir, relPath); err != nil {
		return nil, err
	}
	absPath := r.absPath(relPath)
	content, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if !isText(content) {
		return nil, apperror.New("NOT_TEXT", "file is not a valid text file", "文件不是有效的文本文件")
	}

	return memo.FromRepository(relPath, absPath, string(content)), nil
}

func (r *MemoRepository) Write(ctx context.Context, relPath string, content string) error {
	if err := util.EnsurePathInRoot(r.rootDir, relPath); err != nil {
		return err
	}
	absPath := r.absPath(relPath)
	if content == "" {
		err := os.Remove(absPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(absPath, []byte(content), 0644)
}

func (r *MemoRepository) Find(ctx context.Context, relPath string) iter.Seq2[*memo.Memo, error] {
	return func(yield func(*memo.Memo, error) bool) {
		absPath := filepath.Join(r.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		for _, entry := range entries {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				continue
			}

			absFilePath := filepath.Join(absPath, entry.Name())
			contentBytes, err := os.ReadFile(absFilePath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			m := memo.FromRepository(filepath.Join(relPath, entry.Name()), absFilePath, string(contentBytes))
			if !yield(m, nil) {
				return
			}
		}
	}
}

func (r *MemoRepository) absPath(relPath string) string {
	return filepath.Join(r.rootDir, relPath)

}

var _ memo.Repository = (*MemoRepository)(nil)