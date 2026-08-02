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

	"main/internal/domain/image"
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
	Message        string             `json:"message,omitempty"`
}

// ImageMover 专职处理图片及其同名或带有额外扩展名的配套伴随文件物理移动、暂存与回收站操作
type ImageMover struct {
	rootDir             string
	repo                image.Repository
	filterBuilder       *image.FilterBuilder
	useSystemRecycleBin bool
}

// NewImageMover 创建图片移动实现实例
func NewImageMover(rootDir string, repo image.Repository, filterBuilder *image.FilterBuilder, useSystemRecycleBin bool) *ImageMover {
	return &ImageMover{
		rootDir:             rootDir,
		repo:                repo,
		filterBuilder:       filterBuilder,
		useSystemRecycleBin: useSystemRecycleBin,
	}
}

// findTargetImages 找出指定目录下所有符合过滤条件图片的映射表（以文件名作为键）
func (s *ImageMover) findTargetImages(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
) (map[string]*image.Image, error) {
	imgFilter := s.filterBuilder.Build(filterBy)
	targetImages := make(map[string]*image.Image)
	for img, scanErr := range s.repo.Find(ctx, relPath) {
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
	err = s.matchAssociatedFiles(ctx, srcAbsDir, toMoveImages, func(entry os.DirEntry, img *image.Image, isImageBody bool) error {
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

// Trash 将满足过滤条件的图片及其配套伴随文件移动到专属的回收站内，并保存 meta.json
func (s *ImageMover) Trash(
	ctx context.Context,
	relPath string,
	filterBy shared.ImageFilters,
	message string,
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
		SrcRelPath:     filepath.ToSlash(relPath),
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
	err = s.matchAssociatedFiles(ctx, srcAbsDir, toDeleteImages, func(entry os.DirEntry, img *image.Image, isImageBody bool) error {
		srcFilePath := filepath.Join(srcAbsDir, entry.Name())
		targetFilePath := filepath.Join(filesDir, entry.Name())

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
				RelPath: filepath.ToSlash(entry.Name()),
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
		SrcRelPath:     filepath.ToSlash(relPath),
		Images:         trashedImages,
		Message:        message,
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

// #region 清空回收站
// EmptyTrash 清空早于指定存留期的暂存记录。通过快速重命名标记删除，并在后台 Goroutine 中进行异步磁盘物理擦除
func (s *ImageMover) EmptyTrash(ctx context.Context, minAge time.Duration) (clearedCount int, err error) {
	trashRoot := filepath.Join(s.rootDir, trashDirName)

	errB := util.NewErrorsBuilder(16)
	var dirsToSweep []string

	// 使用 dirEntries 进行流式分批遍历目录项，避免一次性读入全部条目到内存中
	for entry, scanErr := range dirEntries(ctx, trashRoot) {
		if scanErr != nil {
			return clearedCount, errors.Join(errB.Build(), scanErr)
		}
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		historyDir := filepath.Join(trashRoot, name)

		// 若发现先前未完成的标记删除目录，一并放入后台物理清理队列中
		if strings.HasPrefix(name, "deleting_") {
			dirsToSweep = append(dirsToSweep, historyDir)
			continue
		}

		// 仅处理标准的回收站历史目录
		if !strings.HasPrefix(name, "trash_") {
			continue
		}

		metaBytes, err := os.ReadFile(filepath.Join(historyDir, "meta.json"))
		if err != nil {
			errB.Add(fmt.Errorf("failed to read meta.json for trash history %s: %w", name, err))
			continue
		}
		var meta trashMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			errB.Add(fmt.Errorf("failed to parse meta.json for trash history %s: %w", name, err))
			continue
		}

		// 如果存留时间超过设定时长，将其瞬间重命名为 deleting_<historyId> 进行标记删除
		if time.Since(meta.TrashedAt) >= minAge {
			deletingDir := filepath.Join(trashRoot, "deleting_"+name)
			if err := os.Rename(historyDir, deletingDir); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				errB.Add(fmt.Errorf("failed to mark trash history %s for deletion: %w", name, err))
				continue
			}
			dirsToSweep = append(dirsToSweep, deletingDir)
			clearedCount++
		}
	}

	// 若存在待擦除目录，启动后台 Goroutine 进行异步物理清理，避免阻塞 HTTP/GraphQL 请求
	if len(dirsToSweep) > 0 {
		useSystemRecycleBin := s.useSystemRecycleBin
		go func(paths []string) {
			const batchSize = 64
			for i := 0; i < len(paths); i += batchSize {
				end := i + batchSize
				if end > len(paths) {
					end = len(paths)
				}
				batch := paths[i:end]
				_ = trashOrDelete(batch, useSystemRecycleBin)
			}
		}(dirsToSweep)
	}

	return clearedCount, errB.Build()
}
// #endregion

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

		// 逐个反向处理并流式返回。由于 os.ReadDir 已经对 entries 按名称进行了升序排序，
		// 因而在此处直接从后往前进行反向遍历即是降序（最新的记录在最前面），
		// 由此彻底省去了用于过滤的辅助 validDirs 数组和额外的 slices.SortFunc 排序步骤。
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "trash_") {
				continue
			}

			metaBytes, err := os.ReadFile(filepath.Join(trashRoot, entry.Name(), "meta.json"))
			if err != nil {
				if !yield(nil, fmt.Errorf("failed to read meta.json for trash history %s: %w", entry.Name(), err)) {
					return
				}
				continue
			}
			var meta trashMeta
			if err := json.Unmarshal(metaBytes, &meta); err != nil {
				if !yield(nil, fmt.Errorf("failed to parse meta.json for trash history %s: %w", entry.Name(), err)) {
					return
				}
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
				SrcRelPath:          filepath.ToSlash(meta.SrcRelPath),
				Message:             meta.Message,
			}

			if !yield(item, nil) {
				return
			}
		}
	}
}

var _ image.Mover = (*ImageMover)(nil)
var _ image.Trasher = (*ImageMover)(nil)
