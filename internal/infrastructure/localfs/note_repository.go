package localfs

import (
	"context"
	"fmt"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/note"
	"main/internal/domain/relpath"
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

func (r *NoteRepository) Read(ctx context.Context, relPath relpath.Path) (*note.Note, error) {
	absPath := relPath.Abs(r.rootDir)
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

func (r *NoteRepository) Write(ctx context.Context, relPath relpath.Path, content string) error {
	absPath := relPath.Abs(r.rootDir)
	if content == "" {
		err := os.Remove(absPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(absPath, []byte(content), 0644)
}

func (r *NoteRepository) Find(ctx context.Context, relPath relpath.Path) iter.Seq2[*note.Note, error] {
	return func(yield func(*note.Note, error) bool) {
		absPath := relPath.Abs(r.rootDir)
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

			// 笔记项来自文件系统遍历，在此处构造根相对路径（出生点之一）并围栏
			fileRelPath, err := relPath.Join(r.rootDir, entry.Name())
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			n := note.FromRepository(fileRelPath, absFilePath, string(contentBytes), info.ModTime())
			if !yield(n, nil) {
				return
			}
		}
	}
}

var _ note.Repository = (*NoteRepository)(nil)
