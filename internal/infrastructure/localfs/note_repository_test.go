package localfs

import (
	"context"
	"os"
	"strings"
	"testing"

	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoteRepository_Write 笔记写入的落盘契约：非空内容原子落位，空内容等价于删除
func TestNoteRepository_Write(t *testing.T) {
	ctx := context.Background()

	t.Run("should replace whole file atomically", func(t *testing.T) {
		rootDir := t.TempDir()
		repo := NewNoteRepository(rootDir)
		relPath := testutil.RelIn(t, rootDir, "note.md")
		absPath := relPath.Abs(rootDir)

		oldContent := strings.Repeat("old\n", 1000)
		newContent := strings.Repeat("new\n", 1000)
		require.NoError(t, os.WriteFile(absPath, []byte(oldContent), 0644))

		testutil.AssertAtomicReplace(t, absPath, oldContent, newContent, func() error {
			return repo.Write(ctx, relPath, newContent)
		})
	})

	t.Run("should delete file for empty content", func(t *testing.T) {
		rootDir := t.TempDir()
		repo := NewNoteRepository(rootDir)
		relPath := testutil.RelIn(t, rootDir, "note.md")
		absPath := relPath.Abs(rootDir)

		require.NoError(t, os.WriteFile(absPath, []byte("content"), 0644))

		require.NoError(t, repo.Write(ctx, relPath, ""))
		assert.NoFileExists(t, absPath)
	})

	t.Run("should not fail for empty content on missing file", func(t *testing.T) {
		rootDir := t.TempDir()
		repo := NewNoteRepository(rootDir)
		relPath := testutil.RelIn(t, rootDir, "note.md")

		require.NoError(t, repo.Write(ctx, relPath, ""))
		assert.NoFileExists(t, relPath.Abs(rootDir))
	})
}
