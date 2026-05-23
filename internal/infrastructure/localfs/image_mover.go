package localfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domainimage "main/internal/domain/image"
	"main/internal/shared"
	"main/internal/util"
)

// ImageMover 专职处理图片及其同名或带有额外扩展名的配套伴随文件物理移动操作
type ImageMover struct {
	rootDir      string
	imageScanner domainimage.Scanner
}

// NewImageMover 创建图片移动实现实例
func NewImageMover(rootDir string, imageScanner domainimage.Scanner) *ImageMover {
	return &ImageMover{
		rootDir:      rootDir,
		imageScanner: imageScanner,
	}
}

// Move 移动满足过滤条件的图片及其配套文件至目标相对路径下
func (s *ImageMover) Move(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
	toDirRelPath string,
) (movedCount int, targetAbsDir string, err error) {
	// 拼接并清理目标目录的相对路径，以支持基于当前目录的相对定位（如 "subdir" 或 "../sibling"）
	finalRelPath := filepath.Clean(filepath.Join(relPath, toDirRelPath))

	// 计算目标物理目录的绝对路径，用于创建和返回
	targetAbsDir = filepath.Join(s.rootDir, finalRelPath)

	// 安全校验：确保最终目标路径在配置 of 根目录范围内，防止目录穿越
	if err := util.EnsurePathInRoot(s.rootDir, finalRelPath); err != nil {
		return 0, "", err
	}

	// 构造过滤器以对图片进行多维度条件筛选
	imgFilter := domainimage.BuildImageFilter(&filterBy)

	// 扫描当前目录并收集满足过滤条件的所有图片的绝对路径
	var matchingPaths []string
	for img, scanErr := range s.imageScanner.Scan(ctx, relPath) {
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

var _ domainimage.Mover = (*ImageMover)(nil)
