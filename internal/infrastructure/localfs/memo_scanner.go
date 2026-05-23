package localfs

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"main/internal/domain/memo"
	"main/internal/scalar"
)

// MemoScanner 负责扫描本地物理目录下的备忘录文件并构建领域 Memo 对象
type MemoScanner struct {
	rootDir string
}

// NewMemoScanner 创建备忘录扫描实现实例
func NewMemoScanner(rootDir string) *MemoScanner {
	return &MemoScanner{
		rootDir: rootDir,
	}
}

// Scan 扫描物理目录下的备忘录并返回迭代器
func (s *MemoScanner) Scan(ctx context.Context, relPath string) iter.Seq2[*memo.Memo, error] {
	return func(yield func(*memo.Memo, error) bool) {
		absPath := filepath.Join(s.rootDir, relPath)
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
			// 过滤掉目录和隐藏文件
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			// 过滤非 .md 结尾文件
			if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				continue
			}

			// 推导关联图片相对路径以生成最匹配的 Memo ID
			baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			supportedExtensions := []string{".jpg", ".jpeg", ".png", ".webp", ".avif"}
			imageFound := false
			imageRelPath := ""
			for _, ext := range supportedExtensions {
				imageFilename := baseName + ext
				imageAbsPath := filepath.Join(absPath, imageFilename)
				if _, err := os.Stat(imageAbsPath); err == nil {
					imageFound = true
					imageRelPath = filepath.Join(relPath, imageFilename)
					break
				}
			}

			var memoID scalar.ID
			if imageFound {
				memoID = memo.EncodeID(imageRelPath)
			} else {
				memoID = memo.EncodeID(filepath.Join(relPath, entry.Name()))
			}

			absFilePath := filepath.Join(absPath, entry.Name())
			contentBytes, err := os.ReadFile(absFilePath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			m := memo.NewMemo(memoID, absFilePath, string(contentBytes))
			if !yield(m, nil) {
				return
			}
		}
	}
}

var _ memo.Scanner = (*MemoScanner)(nil)
