package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"main/internal/domain/image"
	"main/internal/domain/relpath"
	"main/internal/shared"
)

// dirEntries 分批流式读取目录下的文件，避免在大量文件目录下导致内存暴涨
func dirEntries(ctx context.Context, dirAbsPath string) iter.Seq2[os.DirEntry, error] {
	const batchSize = 256
	return func(yield func(os.DirEntry, error) bool) {
		f, err := os.Open(dirAbsPath)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			yield(nil, err)
			return
		}
		defer f.Close()
		for {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}
			// 每次读取 batchSize 个条目
			entries, err := f.ReadDir(batchSize)
			for _, entry := range entries {
				// 忽略以点开头的隐藏文件或目录
				if strings.HasPrefix(entry.Name(), ".") {
					continue
				}
				if !yield(entry, nil) {
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					yield(nil, err)
				}
				return
			}
		}
	}
}

// findTargetImages 找出指定目录下所有符合过滤条件图片的映射表（以文件名作为键）
func findTargetImages(
	ctx context.Context,
	repo image.Repository,
	filterBuilder *image.FilterBuilder,
	relPath relpath.Path,
	filterBy shared.ImageFilters,
) (map[string]*image.Image, error) {
	imgFilter := filterBuilder.Build(filterBy)
	targetImages := make(map[string]*image.Image)
	for img, scanErr := range repo.Find(ctx, relPath) {
		if scanErr != nil {
			return nil, scanErr
		}
		if imgFilter(img) {
			targetImages[img.RelPath().Base()] = img
		}
	}
	return targetImages, nil
}

// matchAssociatedFiles 扫描源目录中的文件，匹配并找出属于目标图片集及其伴随文件，并通过回调函数处理
func matchAssociatedFiles(
	ctx context.Context,
	srcAbsDir string,
	targetImages map[string]*image.Image,
	onMatch func(entry os.DirEntry, img *image.Image, isImageBody bool) error,
) error {
	for entry, err := range dirEntries(ctx, srcAbsDir) {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			continue
		}

		var associatedImg *image.Image
		var isImageBody bool

		// 1. 优先进行文件名全匹配（对应图片本体）
		if img, ok := targetImages[entry.Name()]; ok {
			associatedImg = img
			isImageBody = true
		} else {
			// 2. 逐步剥离扩展名，匹配同名伴随文件（如 XMP、RAW 关联文件等）
			curr := entry.Name()
			for {
				ext := filepath.Ext(curr)
				if ext == "" {
					break
				}
				curr = strings.TrimSuffix(curr, ext)
				if img, ok := targetImages[curr]; ok {
					associatedImg = img
					break
				}
			}
		}

		if associatedImg != nil {
			if err := onMatch(entry, associatedImg, isImageBody); err != nil {
				return err
			}
		}
	}
	return nil
}

// ImageMover 专职处理图片及其同名或带有额外扩展名的配套伴随文件的物理移动
type ImageMover struct {
	rootDir       string
	repo          image.Repository
	filterBuilder *image.FilterBuilder
}

// NewImageMover 创建图片移动实现实例
func NewImageMover(rootDir string, repo image.Repository, filterBuilder *image.FilterBuilder) *ImageMover {
	return &ImageMover{
		rootDir:       rootDir,
		repo:          repo,
		filterBuilder: filterBuilder,
	}
}

// Move 移动满足过滤条件的图片及其配套文件至目标根相对路径下
func (s *ImageMover) Move(
	ctx context.Context,
	relPath relpath.Path,
	filterBy shared.ImageFilters,
	toDirRelPath relpath.Path,
) (movedCount int, targetAbsDir string, err error) {
	// 目标目录已是围栏过的根相对路径，可直接换算物理绝对目录
	targetAbsDir = toDirRelPath.Abs(s.rootDir)

	// 扫描符合过滤条件的图片集
	toMoveImages, err := findTargetImages(ctx, s.repo, s.filterBuilder, relPath, filterBy)
	if err != nil {
		return 0, "", err
	}

	// 若无可移动图片，快速返回
	if len(toMoveImages) == 0 {
		return 0, targetAbsDir, nil
	}

	// 按需创建目标物理目录
	if err := os.MkdirAll(targetAbsDir, 0755); err != nil {
		return 0, "", fmt.Errorf("failed to create target directory: %w", err)
	}

	srcAbsDir := relPath.Abs(s.rootDir)

	// 匹配并移动匹配的文件及其伴随文件
	err = matchAssociatedFiles(ctx, srcAbsDir, toMoveImages, func(entry os.DirEntry, img *image.Image, isImageBody bool) error {
		srcFilePath := filepath.Join(srcAbsDir, entry.Name())
		targetFilePath := filepath.Join(targetAbsDir, entry.Name())

		if err := os.Rename(srcFilePath, targetFilePath); err != nil {
			// 容错处理：若伴随文件已被先行移走，且其不是图片本体，则允许跳过
			if os.IsNotExist(err) && !isImageBody {
				return nil
			}
			return fmt.Errorf("failed to move file %s to %s: %w", srcFilePath, targetFilePath, err)
		}

		// 仅当移动的是图片文件本身时，增加计数
		if isImageBody {
			movedCount++
		}
		return nil
	})
	if err != nil {
		return 0, "", err
	}

	return movedCount, targetAbsDir, nil
}

var _ image.Mover = (*ImageMover)(nil)
