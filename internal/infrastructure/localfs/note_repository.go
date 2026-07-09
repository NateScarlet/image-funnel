package localfs

import (
	"context"
	"fmt"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/note"
	"main/internal/util"
	"os"
	"path/filepath"
	"strings"
)

type NoteRepository struct {
	rootDir string
}

func NewNoteRepository(rootDir string) *NoteRepository {
	return &NoteRepository{
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

func (r *NoteRepository) Read(ctx context.Context, relPath string) (*note.Note, error) {
	if err := util.EnsurePathInRoot(r.rootDir, relPath); err != nil {
		return nil, err
	}
	absPath := r.absPath(relPath)
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
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

	return note.FromRepository(relPath, absPath, string(content), info.ModTime()), nil
}

func (r *NoteRepository) Write(ctx context.Context, relPath string, content string) error {
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

func (r *NoteRepository) Find(ctx context.Context, relPath string) iter.Seq2[*note.Note, error] {
	return func(yield func(*note.Note, error) bool) {
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
			info, err := entry.Info()
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			contentBytes, err := os.ReadFile(absFilePath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			n := note.FromRepository(filepath.Join(relPath, entry.Name()), absFilePath, string(contentBytes), info.ModTime())
			if !yield(n, nil) {
				return
			}
		}
	}
}

func (r *NoteRepository) absPath(relPath string) string {
	return filepath.Join(r.rootDir, relPath)
}

var _ note.Repository = (*NoteRepository)(nil)