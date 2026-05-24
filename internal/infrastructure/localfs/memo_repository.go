package localfs

import (
	"context"
	"main/internal/domain/memo"
	"main/internal/scalar"
	"main/internal/util"
	"os"
	"path/filepath"
)

type MemoRepository struct {
	rootDir string
}

func NewMemoRepository(rootDir string) *MemoRepository {
	return &MemoRepository{
		rootDir: rootDir,
	}
}

func (r *MemoRepository) Read(ctx context.Context, id scalar.ID) (*memo.Memo, error) {
	relPath, err := memo.DecodeID(id)
	if err != nil {
		return nil, err
	}
	absPath := filepath.Join(r.rootDir, relPath)
	memoPath := r.memoPath(relPath)
	content, err := os.ReadFile(memoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return memo.NewMemo(id, absPath, string(content)), nil
}

func (r *MemoRepository) Write(ctx context.Context, id scalar.ID, content string) error {
	relPath, err := memo.DecodeID(id)
	if err != nil {
		return err
	}
	if err := util.EnsurePathInRoot(r.rootDir, relPath); err != nil {
		return err
	}
	memoPath := r.memoPath(relPath)
	if content == "" {
		err := os.Remove(memoPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(memoPath, []byte(content), 0644)
}

func (r *MemoRepository) memoPath(relPath string) string {
	return filepath.Join(r.rootDir, relPath)

}

var _ memo.Repository = (*MemoRepository)(nil)
