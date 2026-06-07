package localfs

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	domainimage "main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
)

const trashDirName = ".io.github.natescarlet.image-funnel.trash"

// 生成基于时间的可排序 ID，格式为 "trash_<timestamp_ns>_<random_suffix>"
// 时间戳部分固定 19 位补零以确保排序稳定，随机后缀保持原样
func newTrashHistoryID() string {
	timestamp := time.Now().UnixNano()
	randomSuffix := rand.Int63()
	return fmt.Sprintf("trash_%019d_%d", timestamp, randomSuffix)
}

type trashedImageMeta struct {
	RelPath string    `json:"relPath"`
	ModTime time.Time `json:"modTime"`
}

type trashMeta struct {
	ID             string             `json:"id"`
	TrashedAt      time.Time          `json:"trashedAt"`
	TotalFileCount int                `json:"totalFileCount"`
	TotalFileSize  int64              `json:"totalFileSize"`
	SrcRelPath     string             `json:"srcRelPath"`
	Images         []trashedImageMeta `json:"images"`
}

// ImageMover 专职处理图片及其同名或带有额外扩展名的配套伴随文件物理移动、暂存与回收站操作
type ImageMover struct {
	rootDir            string
	imageRepo          domainimage.Repository
	imageFilterBuilder *domainimage.FilterBuilder
}

// NewImageMover 创建图片移动实现实例
func NewImageMover(rootDir string, imageRepo domainimage.Repository, imageFilterBuilder *domainimage.FilterBuilder) *ImageMover {
	return &ImageMover{
		rootDir:            rootDir,
		imageRepo:          imageRepo,
		imageFilterBuilder: imageFilterBuilder,
	}
}

// Move 移动满足过滤条件的图片及其配套文件至目标相对路径下
func (s *ImageMover) Move(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
	toDirRelPath string,
) (movedCount int, targetAbsDir string, err error) {
	// 拼接并清理目标目录的相对路径，以支持基于当前目录的相对定位
	finalRelPath := filepath.Clean(filepath.Join(relPath, toDirRelPath))

	// 计算目标物理目录的绝对路径，用于创建和返回
	targetAbsDir = filepath.Join(s.rootDir, finalRelPath)

	// 安全校验：确保最终目标路径在配置的根目录范围内，防止目录穿越
	if err := util.EnsurePathInRoot(s.rootDir, finalRelPath); err != nil {
		return 0, "", err
	}

	// 构造过滤器以对图片进行多维度条件筛选
	imgFilter := s.imageFilterBuilder.Build(filterBy)

	// 扫描当前目录并收集满足过滤条件的所有图片的绝对路径
	var matchingPaths []string
	for img, scanErr := range s.imageRepo.Find(ctx, relPath) {
		if scanErr != nil {
			return 0, "", scanErr
		}
		if imgFilter(img) {
			matchingPaths = append(matchingPaths, filepath.Join(s.rootDir, img.RelPath()))
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

// Trash 将满足过滤条件的图片及其配套伴随文件移动到专属的暂存垃圾箱内，并保存 meta.json
func (s *ImageMover) Trash(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
) (historyId string, totalFileCount int, err error) {
	imgFilter := s.imageFilterBuilder.Build(filterBy)

	var matchingImages []*domainimage.Image
	for img, scanErr := range s.imageRepo.Find(ctx, relPath) {
		if scanErr != nil {
			return "", 0, scanErr
		}
		if imgFilter(img) {
			matchingImages = append(matchingImages, img)
		}
	}

	if len(matchingImages) == 0 {
		return "", 0, nil
	}

	historyId = newTrashHistoryID()
	historyDir := filepath.Join(s.rootDir, trashDirName, historyId)
	filesDir := filepath.Join(historyDir, "files")

	var totalSize int64
	var movedFilesCount int
	var trashedImages []trashedImageMeta

	for _, img := range matchingImages {
		imgAbsPath := filepath.Join(s.rootDir, img.RelPath())
		srcAbsDir := filepath.Dir(imgAbsPath)
		imgName := filepath.Base(imgAbsPath)

		entries, err := os.ReadDir(srcAbsDir)
		if err != nil {
			return "", 0, fmt.Errorf("failed to read source directory: %w", err)
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

			isAssociated := checkName == imgName ||
				strings.HasPrefix(checkName, imgName+".") ||
				(imgBase != "" && checkBase == imgBase)

			if isAssociated {
				srcFilePath := filepath.Join(srcAbsDir, checkName)

				// 获取相对于项目根目录的相对路径
				srcRel, err := filepath.Rel(s.rootDir, srcFilePath)
				if err != nil {
					return "", 0, err
				}

				// 剥离当前删除操作所在的“共同前缀（操作目录相对路径）”
				srcRelClean := filepath.ToSlash(srcRel)
				prefixClean := filepath.ToSlash(relPath)
				tempRelPath := srcRelClean
				if prefixClean != "" && prefixClean != "." {
					tempRelPath = strings.TrimPrefix(srcRelClean, prefixClean)
					tempRelPath = strings.TrimPrefix(tempRelPath, "/")
				}

				targetFilePath := filepath.Join(filesDir, filepath.FromSlash(tempRelPath))

				// 获取文件属性用于统计总大小
				info, err := os.Stat(srcFilePath)
				if err != nil {
					if os.IsNotExist(err) && checkName != imgName {
						continue
					}
					return "", 0, err
				}

				if err := os.MkdirAll(filepath.Dir(targetFilePath), 0755); err != nil {
					return "", 0, err
				}

				if err := os.Rename(srcFilePath, targetFilePath); err != nil {
					if os.IsNotExist(err) && checkName != imgName {
						continue
					}
					return "", 0, fmt.Errorf("failed to stash file %s to %s: %w", srcFilePath, targetFilePath, err)
				}

				totalSize += info.Size()
				movedFilesCount++

				if checkName == imgName {
					trashedImages = append(trashedImages, trashedImageMeta{
						RelPath: filepath.ToSlash(tempRelPath),
						ModTime: img.ModTime(),
					})
				}
			}
		}
	}

	// 按更新时间降序排序，最新的图片排在最前面
	slices.SortFunc(trashedImages, func(a, b trashedImageMeta) int {
		if a.ModTime.After(b.ModTime) {
			return -1
		}
		if a.ModTime.Before(b.ModTime) {
			return 1
		}
		return 0
	})

	// 写入 meta.json
	meta := trashMeta{
		ID:             historyId,
		TrashedAt:      time.Now(),
		TotalFileCount: movedFilesCount,
		TotalFileSize:  totalSize,
		SrcRelPath:     relPath,
		Images:         trashedImages,
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", 0, err
	}

	if err := os.WriteFile(filepath.Join(historyDir, "meta.json"), metaBytes, 0644); err != nil {
		return "", 0, err
	}

	return historyId, movedFilesCount, nil
}

// UndoTrash 撤销垃圾暂存，恢复文件，若冲突则抛错
func (s *ImageMover) UndoTrash(ctx context.Context, historyId string) (restoredCount int, err error) {
	historyDir := filepath.Join(s.rootDir, trashDirName, historyId)
	filesDir := filepath.Join(historyDir, "files")

	metaBytes, err := os.ReadFile(filepath.Join(historyDir, "meta.json"))
	if err != nil {
		return 0, fmt.Errorf("failed to read meta.json: %w", err)
	}

	var meta trashMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return 0, err
	}

	// 遍历 filesDir
	var tempFiles []string
	err = filepath.Walk(filesDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			tempFiles = append(tempFiles, path)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to walk files directory: %w", err)
	}

	type moveTask struct {
		tempAbsPath string
		srcAbsPath  string
		srcRelPath  string
	}
	var tasks []moveTask

	// 第一遍遍历：进行路径冲突检查
	for _, tempAbsPath := range tempFiles {
		rel, err := filepath.Rel(filesDir, tempAbsPath)
		if err != nil {
			return 0, err
		}
		tempRelPath := filepath.ToSlash(rel)
		srcRelPath := filepath.Clean(filepath.Join(meta.SrcRelPath, tempRelPath))
		srcAbsPath := filepath.Join(s.rootDir, srcRelPath)

		if _, err := os.Stat(srcAbsPath); err == nil {
			// 文件已存在，直接报错由用户手动处理
			return 0, fmt.Errorf("conflict: target file already exists: %s", srcRelPath)
		}

		tasks = append(tasks, moveTask{
			tempAbsPath: tempAbsPath,
			srcAbsPath:  srcAbsPath,
			srcRelPath:  srcRelPath,
		})
	}

	// 第二遍遍历：执行还原移动
	for _, task := range tasks {
		if err := os.MkdirAll(filepath.Dir(task.srcAbsPath), 0755); err != nil {
			return 0, err
		}
		if err := os.Rename(task.tempAbsPath, task.srcAbsPath); err != nil {
			return 0, fmt.Errorf("failed to restore file to %s: %w", task.srcRelPath, err)
		}
		restoredCount++
	}

	// 清理已空的暂存文件夹
	if err := os.RemoveAll(historyDir); err != nil {
		return restoredCount, err
	}

	return restoredCount, nil
}

// EmptyTrash 清空早于指定存留期的暂存记录。先放回原位，然后系统级回收站删除，若冲突则报错
func (s *ImageMover) EmptyTrash(ctx context.Context, minAge time.Duration) (clearedCount int, err error) {
	trashRoot := filepath.Join(s.rootDir, trashDirName)
	entries, err := os.ReadDir(trashRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var expiredDirs []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaBytes, err := os.ReadFile(filepath.Join(trashRoot, entry.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var meta trashMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}

		if time.Since(meta.TrashedAt) >= minAge {
			expiredDirs = append(expiredDirs, entry)
		}
	}

	for _, entry := range expiredDirs {
		historyId := entry.Name()
		historyDir := filepath.Join(trashRoot, historyId)
		filesDir := filepath.Join(historyDir, "files")

		metaBytes, err := os.ReadFile(filepath.Join(historyDir, "meta.json"))
		if err != nil {
			return clearedCount, err
		}
		var meta trashMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			return clearedCount, err
		}

		var tempFiles []string
		err = filepath.Walk(filesDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !info.IsDir() {
				tempFiles = append(tempFiles, path)
			}
			return nil
		})
		if err != nil {
			return clearedCount, err
		}

		type restoreToTrashTask struct {
			tempAbsPath string
			srcAbsPath  string
			srcRelPath  string
		}
		var tasks []restoreToTrashTask

		// 检查冲突
		for _, tempAbsPath := range tempFiles {
			rel, err := filepath.Rel(filesDir, tempAbsPath)
			if err != nil {
				return clearedCount, err
			}
			tempRelPath := filepath.ToSlash(rel)
			srcRelPath := filepath.Clean(filepath.Join(meta.SrcRelPath, tempRelPath))
			srcAbsPath := filepath.Join(s.rootDir, srcRelPath)

			if _, err := os.Stat(srcAbsPath); err == nil {
				return clearedCount, fmt.Errorf("conflict: cannot empty trash, target file already exists: %s", srcRelPath)
			}

			tasks = append(tasks, restoreToTrashTask{
				tempAbsPath: tempAbsPath,
				srcAbsPath:  srcAbsPath,
				srcRelPath:  srcRelPath,
			})
		}

		// 移回原始路径
		var srcPaths []string
		for _, task := range tasks {
			if err := os.MkdirAll(filepath.Dir(task.srcAbsPath), 0755); err != nil {
				return clearedCount, err
			}
			if err := os.Rename(task.tempAbsPath, task.srcAbsPath); err != nil {
				return clearedCount, fmt.Errorf("failed to restore file %s to %s for trash: %w", task.tempAbsPath, task.srcRelPath, err)
			}
			srcPaths = append(srcPaths, task.srcAbsPath)
		}

		// 将原始路径下的文件投递到系统回收站
		if err := moveToRecycleBin(srcPaths); err != nil {
			return clearedCount, fmt.Errorf("failed to move files to recycle bin: %w", err)
		}

		// 物理删除整个暂存历史子目录
		if err := os.RemoveAll(historyDir); err != nil {
			return clearedCount, err
		}

		clearedCount++
	}

	return clearedCount, nil
}

// FindTrashHistory 获取历史记录列表迭代器
func (s *ImageMover) FindTrashHistory(ctx context.Context) iter.Seq2[*shared.TrashHistoryItemDTO, error] {
	return func(yield func(*shared.TrashHistoryItemDTO, error) bool) {
		trashRoot := filepath.Join(s.rootDir, trashDirName)
		entries, err := os.ReadDir(trashRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			yield(nil, err)
			return
		}

		// 收集所有有效的 trash 历史目录（以 "trash_" 开头）
		var validDirs []os.DirEntry
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "trash_") {
				validDirs = append(validDirs, entry)
			}
		}

		// 按目录名降序排序（由于目录名包含时间戳，这样最新的记录在最前面）
		slices.SortFunc(validDirs, func(a, b os.DirEntry) int {
			// 字符串比较，字典序降序
			if a.Name() > b.Name() {
				return -1
			}
			if a.Name() < b.Name() {
				return 1
			}
			return 0
		})

		// 逐个处理并流式返回
		for _, entry := range validDirs {
			metaBytes, err := os.ReadFile(filepath.Join(trashRoot, entry.Name(), "meta.json"))
			if err != nil {
				continue
			}
			var meta trashMeta
			if err := json.Unmarshal(metaBytes, &meta); err != nil {
				continue
			}

			imageCount := len(meta.Images)
			associatedCount := meta.TotalFileCount - imageCount
			var coverImageAbsPath string
			if imageCount > 0 {
				coverImageAbsPath = filepath.Join(trashRoot, meta.ID, "files", filepath.FromSlash(meta.Images[0].RelPath))
			}

			item := &shared.TrashHistoryItemDTO{
				ID:                  scalar.ToID(meta.ID),
				TotalFileCount:      meta.TotalFileCount,
				TotalFileSize:       meta.TotalFileSize,
				TrashedAt:           meta.TrashedAt,
				ImageCount:          imageCount,
				AssociatedFileCount: associatedCount,
				CoverImageAbsPath:   coverImageAbsPath,
			}

			if !yield(item, nil) {
				return
			}
		}
	}
}

var _ domainimage.Mover = (*ImageMover)(nil)
var _ domainimage.Trasher = (*ImageMover)(nil)
