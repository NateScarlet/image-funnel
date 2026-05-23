package localfs

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/domain/memo"
	"main/internal/iterator"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
)

type Scanner struct {
	rootDir      string
	imageFactory *domainimage.Factory
	dirRepo      directory.Repository
}

func NewScanner(rootDir string, imageFactory *domainimage.Factory, dirRepo directory.Repository) *Scanner {
	return &Scanner{
		rootDir:      rootDir,
		imageFactory: imageFactory,
		dirRepo:      dirRepo,
	}
}

func (s *Scanner) Scan(ctx context.Context, relPath string) iter.Seq2[*domainimage.Image, error] {
	return func(yield func(*domainimage.Image, error) bool) {
		absPath := filepath.Join(s.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		// 计算当前目录的ID
		directoryID := directory.EncodeID(relPath)

		limit := runtime.NumCPU()

		iterator.ParallelConcatMapTo2(
			ctx,
			limit,
			slices.Values(entries),
			yield,
		)(
			func(ctx context.Context, yield func(*domainimage.Image, error) bool, entry os.DirEntry) bool {
				if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
					return true
				}

				absFilePath := filepath.Join(absPath, entry.Name())
				info, err := entry.Info()
				if err != nil {
					return yield(nil, err)
				}

				img, err := s.imageFactory.CreateFromInfo(ctx, info, absFilePath, directoryID)
				if err != nil {
					return yield(nil, err)
				}
				if img == nil {
					return true // Not supported or skipped
				}

				return yield(img, nil)
			},
		)
	}
}

func (s *Scanner) ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {
		if relPath != "" {
			if err := util.EnsurePathInRoot(s.rootDir, relPath); err != nil {
				yield(nil, err)
				return
			}
		}

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
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			subRelPath := filepath.Join(relPath, entry.Name())
			dirInfo, err := s.dirRepo.GetByRelPath(ctx, subRelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(dirInfo, nil) {
				break
			}
		}
	}
}

func (s *Scanner) AnalyzeDirectory(ctx context.Context, relPath string) (*directory.DirectoryStats, error) {
	if err := util.EnsurePathInRoot(s.rootDir, relPath); err != nil {
		return nil, err
	}

	absPath := filepath.Join(s.rootDir, relPath)
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	subdirectoryCount := 0
	var files []os.DirEntry

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			subdirectoryCount++
			continue
		}
		files = append(files, entry)
	}

	imageCount := 0
	var latestImage *domainimage.Image
	ratingCounts := make(map[int]int)

	// 计算当前目录的ID
	directoryID := directory.EncodeID(relPath)

	limit := runtime.NumCPU()

	iterator.ParallelConcatMapTo2(
		ctx,
		limit,
		slices.Values(files),
		func(img *domainimage.Image, err error) bool {
			if err != nil || img == nil {
				return true
			}
			imageCount++
			if latestImage == nil || img.ModTime().After(latestImage.ModTime()) {
				latestImage = img
			}
			ratingCounts[img.Rating()]++
			return true
		},
	)(
		func(ctx context.Context, yield func(*domainimage.Image, error) bool, entry os.DirEntry) bool {
			if ctx.Err() != nil {
				return false
			}

			info, err := entry.Info()
			if err != nil {
				return yield(nil, err)
			}

			imagePath := filepath.Join(absPath, entry.Name())
			img, err := s.imageFactory.CreateFromInfo(ctx, info, imagePath, directoryID)
			return yield(img, err)
		},
	)

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return directory.NewDirectoryStats(imageCount, subdirectoryCount, latestImage, ratingCounts), nil
}

func (s *Scanner) LookupImage(ctx context.Context, relPath string) (*domainimage.Image, error) {
	// 计算目录ID
	dirRelPath := filepath.Dir(relPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}
	directoryID := directory.EncodeID(dirRelPath)
	return s.imageFactory.Create(ctx, relPath, s.rootDir, directoryID)
}

// ScanMemos 扫描物理目录下的备忘录
func (s *Scanner) ScanMemos(ctx context.Context, relPath string) iter.Seq2[*memo.Memo, error] {
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

// #region 移动筛选匹配图片及配套文件

// MoveImages 移动满足过滤条件的图片及其同名或带有额外扩展名的配套文件
func (s *Scanner) MoveImages(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
	toDirRelPath string,
) (movedCount int, targetAbsDir string, err error) {
	// 拼接并清理目标目录的相对路径，以支持基于当前目录的相对定位（如 "subdir" 或 "../sibling"）
	finalRelPath := filepath.Clean(filepath.Join(relPath, toDirRelPath))

	// 计算目标物理目录的绝对路径，用于创建和返回
	targetAbsDir = filepath.Join(s.rootDir, finalRelPath)

	// 安全校验：确保最终目标路径在配置的根目录范围内，防止目录穿越
	if err := util.EnsurePathInRoot(s.rootDir, finalRelPath); err != nil {
		return 0, "", err
	}

	// 构造过滤器以对图片进行多维度条件筛选
	imgFilter := domainimage.BuildImageFilter(&filterBy)

	// 扫描当前目录并收集满足过滤条件的所有图片的绝对路径
	var matchingPaths []string
	for img, scanErr := range s.Scan(ctx, relPath) {
		if scanErr != nil {
			return 0, "", scanErr
		}
		if imgFilter(img) {
			matchingPaths = append(matchingPaths, img.AbsPath())
		}
	}

	// 若无可移动图片，快速返回
	if len(matchingPaths) == 0 {
		return 0, targetAbsDir, nil
	}

	// 按需创建目标物理目录
	if err := os.MkdirAll(targetAbsDir, 0755); err != nil {
		return 0, "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// 遍历符合条件的图片，识别并物理移动该图片及所有的相关伴随文件
	for _, imgAbsPath := range matchingPaths {
		srcAbsDir := filepath.Dir(imgAbsPath)
		imgName := filepath.Base(imgAbsPath)

		// 扫描图片所在物理目录的所有文件以抓取配套伴随文件
		entries, err := os.ReadDir(srcAbsDir)
		if err != nil {
			return movedCount, "", fmt.Errorf("failed to read source directory: %w", err)
		}

		imgExt := filepath.Ext(imgName)
		imgBase := strings.TrimSuffix(imgName, imgExt)

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			checkName := entry.Name()
			checkExt := filepath.Ext(checkName)
			checkBase := strings.TrimSuffix(checkName, checkExt)

			// 配套文件判断逻辑：
			// 1. 文件名完全一致（图片本体本身）
			// 2. 以当前图片名加点为前缀的文件（例如 aaa.png.json, aaa.png.txt）
			// 3. 主文件名完全相同的文件（例如 aaa.json, aaa.txt，通常为 Prompt 描述或伴随元数据）
			isAssociated := checkName == imgName ||
				strings.HasPrefix(checkName, imgName+".") ||
				(imgBase != "" && checkBase == imgBase)

			if isAssociated {
				srcFilePath := filepath.Join(srcAbsDir, checkName)
				targetFilePath := filepath.Join(targetAbsDir, checkName)

				if err := os.Rename(srcFilePath, targetFilePath); err != nil {
					// 容错处理：若伴随文件由于之前的图片移动已被先行移走，允许直接跳过
					if os.IsNotExist(err) && checkName != imgName {
						continue
					}
					return movedCount, "", fmt.Errorf("failed to move file %s to %s: %w", srcFilePath, targetFilePath, err)
				}
			}
		}
		movedCount++
	}

	return movedCount, targetAbsDir, nil
}

// #endregion

var _ directory.Scanner = (*Scanner)(nil)
