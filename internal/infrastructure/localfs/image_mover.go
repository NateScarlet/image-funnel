package localfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// findTargetImages 找出指定目录下所有符合过滤条件图片的映射表（以文件名作为键）
func (s *ImageMover) findTargetImages(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
) (map[string]*domainimage.Image, error) {
	imgFilter := s.imageFilterBuilder.Build(filterBy)
	targetImages := make(map[string]*domainimage.Image)
	for img, scanErr := range s.imageRepo.Find(ctx, relPath) {
		if scanErr != nil {
			return nil, scanErr
		}
		if imgFilter(img) {
			imgName := filepath.Base(img.RelPath())
			targetImages[imgName] = img
		}
	}
	return targetImages, nil
}

// matchAssociatedFiles 扫描源目录中的文件，匹配并找出属于目标图片集及其伴随文件，并通过回调函数处理
func (s *ImageMover) matchAssociatedFiles(
	ctx context.Context,
	srcAbsDir string,
	targetImages map[string]*domainimage.Image,
	onMatch func(entry os.DirEntry, checkName string, img *domainimage.Image, isImageBody bool) error,
) error {
	for entry, err := range dirEntries(ctx, srcAbsDir) {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			continue
		}

		checkName := entry.Name()
		var associatedImg *domainimage.Image
		var isImageBody bool

		// 1. 优先进行文件名全匹配（对应图片本体）
		if img, ok := targetImages[checkName]; ok {
			associatedImg = img
			isImageBody = true
		} else {
			// 2. 逐步剥离扩展名，匹配同名伴随文件（如 XMP、RAW 关联文件等）
			curr := checkName
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
			if err := onMatch(entry, checkName, associatedImg, isImageBody); err != nil {
				return err
			}
		}
	}
	return nil
}

// Move 移动满足过滤条件的图片及其配套文件至目标相对路径下
func (s *ImageMover) Move(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
	toDirRelPath string,
) (movedCount int, targetAbsDir string, err error) {
	// 目标目录已经是相对于项目根目录的路径，进行规范化
	finalRelPath := filepath.Clean(toDirRelPath)

	// 计算目标物理目录的绝对路径，用于创建和返回
	targetAbsDir = filepath.Join(s.rootDir, finalRelPath)

	// 安全校验：确保最终目标路径在配置的根目录范围内，防止目录穿越
	if err := util.EnsurePathInRoot(s.rootDir, finalRelPath); err != nil {
		return 0, "", err
	}

	// 扫描符合过滤条件的图片集
	toMoveImages, err := s.findTargetImages(ctx, relPath, filterBy)
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

	srcAbsDir := filepath.Join(s.rootDir, relPath)

	// 匹配并移动匹配的文件及其伴随文件
	err = s.matchAssociatedFiles(ctx, srcAbsDir, toMoveImages, func(entry os.DirEntry, checkName string, img *domainimage.Image, isImageBody bool) error {
		srcFilePath := filepath.Join(srcAbsDir, checkName)
		targetFilePath := filepath.Join(targetAbsDir, checkName)

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

// Trash 将满足过滤条件的图片及其配套伴随文件移动到专属的回收站内，并保存 meta.json
func (s *ImageMover) Trash(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
) (historyId string, totalFileCount int, err error) {
	// 扫描符合过滤条件的图片集
	toDeleteImages, err := s.findTargetImages(ctx, relPath, filterBy)
	if err != nil {
		return "", 0, err
	}

	if len(toDeleteImages) == 0 {
		return "", 0, nil
	}

	historyId = newTrashHistoryID()
	historyDir := filepath.Join(s.rootDir, trashDirName, historyId)
	filesDir := filepath.Join(historyDir, "files")
	metaPath := filepath.Join(historyDir, "meta.json")

	// 1. 写入占位 meta.json
	placeholderMeta := trashMeta{
		ID:             historyId,
		TrashedAt:      time.Now(),
		TotalFileCount: -1,
		TotalFileSize:  -1,
		SrcRelPath:     relPath,
		Images:         nil,
	}

	metaBytes, err := json.MarshalIndent(placeholderMeta, "", "  ")
	if err != nil {
		return "", 0, err
	}

	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create history directory: %w", err)
	}

	err = util.AtomicSave(metaPath, func(f *os.File) error {
		_, err := f.Write(metaBytes)
		return err
	})
	if err != nil {
		return "", 0, fmt.Errorf("failed to write placeholder meta.json: %w", err)
	}

	var totalSize int64
	var movedFilesCount int
	var trashedImages []trashedImageMeta
	var filesDirCreated bool

	srcAbsDir := filepath.Join(s.rootDir, relPath)

	// 2. 匹配并移动到回收暂存目录
	err = s.matchAssociatedFiles(ctx, srcAbsDir, toDeleteImages, func(entry os.DirEntry, checkName string, img *domainimage.Image, isImageBody bool) error {
		srcFilePath := filepath.Join(srcAbsDir, checkName)
		targetFilePath := filepath.Join(filesDir, checkName)

		// 获取文件属性用于统计总大小
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) && !isImageBody {
				return nil
			}
			return err
		}

		// 延迟创建暂存目录，防止空操作产生空目录
		if !filesDirCreated {
			if err := os.MkdirAll(filesDir, 0755); err != nil {
				return err
			}
			filesDirCreated = true
		}

		if err := os.Rename(srcFilePath, targetFilePath); err != nil {
			if os.IsNotExist(err) && !isImageBody {
				return nil
			}
			return fmt.Errorf("failed to stash file %s to %s: %w", srcFilePath, targetFilePath, err)
		}

		totalSize += info.Size()
		movedFilesCount++

		if isImageBody {
			trashedImages = append(trashedImages, trashedImageMeta{
				RelPath: filepath.ToSlash(checkName),
				ModTime: img.ModTime(),
			})
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}

	// 防御性处理：如果实际没有移动任何文件，清理暂存历史目录并返回
	if movedFilesCount == 0 {
		os.RemoveAll(historyDir)
		return "", 0, nil
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

	// 3. 重写完整的 meta.json
	meta := trashMeta{
		ID:             historyId,
		TrashedAt:      time.Now(),
		TotalFileCount: movedFilesCount,
		TotalFileSize:  totalSize,
		SrcRelPath:     relPath,
		Images:         trashedImages,
	}

	metaBytes, err = json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", 0, err
	}

	err = util.AtomicSave(metaPath, func(f *os.File) error {
		_, err := f.Write(metaBytes)
		return err
	})
	if err != nil {
		return "", 0, fmt.Errorf("failed to write final meta.json: %w", err)
	}

	return historyId, movedFilesCount, nil
}

// UndoTrash 撤销回收站，恢复文件，若冲突则移入 UNDO_TRASH_CONFLICT_<随机字符> 目录下
func (s *ImageMover) UndoTrash(ctx context.Context, historyId string) (*shared.UndoTrashResultDTO, error) {
	historyDir := filepath.Join(s.rootDir, trashDirName, historyId)
	filesDir := filepath.Join(historyDir, "files")

	metaBytes, err := os.ReadFile(filepath.Join(historyDir, "meta.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read meta.json: %w", err)
	}

	var meta trashMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, err
	}

	// 生成包含 6 位随机大写字符的冲突目录名
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	conflictDirName := fmt.Sprintf("UNDO_TRASH_CONFLICT_%s", string(b))

	var restoredCount int
	var conflictCount int

	if _, err := os.Stat(filesDir); err == nil {
		err = filepath.Walk(filesDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(filesDir, path)
			if err != nil {
				return err
			}
			tempRelPath := filepath.ToSlash(rel)
			srcRelPath := filepath.Clean(filepath.Join(meta.SrcRelPath, tempRelPath))
			srcAbsPath := filepath.Join(s.rootDir, srcRelPath)

			// 检查目标路径是否已存在同名文件
			if _, statErr := os.Stat(srcAbsPath); statErr == nil {
				// 冲突：放到目标父目录下的 UNDO_TRASH_CONFLICT_<随机字符> 目录中
				conflictAbsDir := filepath.Join(filepath.Dir(srcAbsPath), conflictDirName)
				if err := os.MkdirAll(conflictAbsDir, 0755); err != nil {
					return err
				}
				conflictAbsPath := filepath.Join(conflictAbsDir, filepath.Base(srcAbsPath))
				if err := os.Rename(path, conflictAbsPath); err != nil {
					return fmt.Errorf("failed to move conflict file %s to %s: %w", path, conflictAbsPath, err)
				}
				conflictCount++
			} else {
				// 无冲突：还原到原位
				if err := os.MkdirAll(filepath.Dir(srcAbsPath), 0755); err != nil {
					return err
				}
				if err := os.Rename(path, srcAbsPath); err != nil {
					return fmt.Errorf("failed to restore file %s to %s: %w", path, srcRelPath, err)
				}
				restoredCount++
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk and restore files: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat files directory: %w", err)
	}

	// 清理已空的暂存历史文件夹
	if err := os.RemoveAll(historyDir); err != nil {
		return nil, err
	}

	return &shared.UndoTrashResultDTO{
		RestoredCount:   restoredCount,
		ConflictCount:   conflictCount,
		ConflictDirName: conflictDirName,
	}, nil
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
		if _, err := os.Stat(filesDir); err == nil {
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
		} else if !os.IsNotExist(err) {
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
				SrcRelPath:          meta.SrcRelPath,
			}

			if !yield(item, nil) {
				return
			}
		}
	}
}

var _ domainimage.Mover = (*ImageMover)(nil)
var _ domainimage.Trasher = (*ImageMover)(nil)
