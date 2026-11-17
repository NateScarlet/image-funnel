package localfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"main/internal/domain/image"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"

	"go.uber.org/zap"
)

const trashDirName = ".io.github.natescarlet.image-funnel.trash"

// #region 磁盘格式

// trashedImageMeta 记录一张被暂存图片的相对路径与修改时间。
type trashedImageMeta struct {
	RelPath string    `json:"relPath"`
	ModTime time.Time `json:"modTime"`
}

// trashMeta 是回收站历史的磁盘格式，仅在本 module 内被解释与读写。
// TotalFileCount / TotalFileSize 的 -1 表示历史仍在写入中（占位 meta）。
type trashMeta struct {
	ID             string             `json:"id"`
	TrashedAt      time.Time          `json:"trashedAt"`
	TotalFileCount int                `json:"totalFileCount"`
	TotalFileSize  int64              `json:"totalFileSize"`
	SrcRelPath     string             `json:"srcRelPath"`
	Images         []trashedImageMeta `json:"images"`
	Message        string             `json:"message,omitempty"`
}

// trashWriteState 表达从磁盘读取到的历史写入状态，由 -1 哨兵在读取边界翻译而来。
type trashWriteState uint8

const (
	// trashWriteStateComplete 表示 meta 已完整写入，计数字段可信。
	trashWriteStateComplete trashWriteState = iota
	// trashWriteStateWriting 表示磁盘上是「写入中」占位 meta，计数字段尚未确定。
	trashWriteStateWriting
)

// readTrashMeta 在读取边界解析磁盘 meta：
// 把 -1 哨兵翻译成显式的写入状态，并归一化计数字段，确保哨兵值不泄漏到 module 外。
// 各字段独立归一化，使「size 有效但 count 为 -1」的历史仍如实计入释放空间。
func readTrashMeta(metaBytes []byte) (trashMeta, trashWriteState, error) {
	var meta trashMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return trashMeta{}, 0, err
	}
	// 写入中哨兵以 size 为准：它是 EmptyTrash 统计释放空间的依据。
	writing := meta.TotalFileSize < 0
	if meta.TotalFileCount < 0 {
		meta.TotalFileCount = 0
	}
	if meta.TotalFileSize < 0 {
		meta.TotalFileSize = 0
	}
	if writing {
		return meta, trashWriteStateWriting, nil
	}
	return meta, trashWriteStateComplete, nil
}

// 生成基于时间的可排序 ID，格式为 "trash_<timestamp_ns>_<random_suffix>"
// 时间戳部分固定 19 位补零以确保排序稳定，随机后缀保持原样
func newTrashHistoryID() string {
	timestamp := time.Now().UnixNano()
	randomSuffix := rand.Int63()
	return fmt.Sprintf("trash_%019d_%d", timestamp, randomSuffix)
}

// #endregion

// #region 后台清扫

// sweepTarget 描述一个待后台物理清扫的目录及其来源历史 ID。
type sweepTarget struct {
	historyID string
	path      string
}

// #endregion

// ImageTrasher 专职回收站暂存、还原、清空与历史遍历。
// 它是 trashMeta 磁盘格式及其 -1 哨兵的唯一所有者，也是 image.Trasher 的唯一实现。
type ImageTrasher struct {
	rootDir       string
	repo          image.Repository
	filterBuilder *image.FilterBuilder
	sweeper       TrashSweeper
	logger        *zap.Logger
	emptyTrashMu  sync.Mutex
}

// NewImageTrasher 创建回收站 module 实例。
func NewImageTrasher(rootDir string, repo image.Repository, filterBuilder *image.FilterBuilder, sweeper TrashSweeper, logger *zap.Logger) *ImageTrasher {
	return &ImageTrasher{
		rootDir:       rootDir,
		repo:          repo,
		filterBuilder: filterBuilder,
		sweeper:       sweeper,
		logger:        logger,
	}
}

// Trash 将满足过滤条件的图片及其配套伴随文件移动到专属的回收站内，并保存 meta.json
func (s *ImageTrasher) Trash(
	ctx context.Context,
	relPath relpath.Path,
	filterBy shared.ImageFilters,
	message string,
) (historyId string, totalFileCount int, err error) {
	// 扫描符合过滤条件的图片集
	toDeleteImages, err := findTargetImages(ctx, s.repo, s.filterBuilder, relPath, filterBy)
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
		SrcRelPath:     relPath.String(),
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

	srcAbsDir := relPath.Abs(s.rootDir)

	// 2. 匹配并移动到回收暂存目录
	err = matchAssociatedFiles(ctx, srcAbsDir, toDeleteImages, func(entry os.DirEntry, img *image.Image, isImageBody bool) error {
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
		SrcRelPath:     relPath.String(),
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
func (s *ImageTrasher) UndoTrash(ctx context.Context, historyId string) (*shared.UndoTrashResultDTO, error) {
	historyDir := filepath.Join(s.rootDir, trashDirName, historyId)
	filesDir := filepath.Join(historyDir, "files")

	metaBytes, err := os.ReadFile(filepath.Join(historyDir, "meta.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read meta.json: %w", err)
	}

	meta, _, err := readTrashMeta(metaBytes)
	if err != nil {
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

// EmptyTrash 清空早于指定存留期的暂存记录。通过快速重命名标记删除，并在后台 Goroutine 中进行异步磁盘物理擦除。
// 释放空间大小基于各历史 meta 中已持久化的总文件大小求和（物理擦除异步执行，标记删除后无法再实测）
func (s *ImageTrasher) EmptyTrash(ctx context.Context, minAge time.Duration) (*shared.EmptyTrashResultDTO, error) {
	s.emptyTrashMu.Lock()
	defer s.emptyTrashMu.Unlock()

	var clearedCount int
	var clearedSize int64

	trashRoot := filepath.Join(s.rootDir, trashDirName)

	errB := util.NewErrorsBuilder(16)
	var targets []sweepTarget

	// 使用 dirEntries 进行流式分批遍历目录项，避免一次性读入全部条目到内存中
	for entry, scanErr := range dirEntries(ctx, trashRoot) {
		if scanErr != nil {
			return nil, errors.Join(errB.Build(), scanErr)
		}
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		historyDir := filepath.Join(trashRoot, name)

		// 若发现先前未完成的标记删除目录，一并放入后台物理清理队列中
		if strings.HasPrefix(name, "deleting_") {
			targets = append(targets, sweepTarget{
				historyID: strings.TrimPrefix(name, "deleting_"),
				path:      historyDir,
			})
			continue
		}

		// 仅处理标准的回收站历史目录
		if !strings.HasPrefix(name, "trash_") {
			continue
		}

		metaBytes, err := os.ReadFile(filepath.Join(historyDir, "meta.json"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errB.Add(fmt.Errorf("failed to read meta.json for trash history %s: %w", name, err))
			continue
		}
		meta, state, err := readTrashMeta(metaBytes)
		if err != nil {
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
			targets = append(targets, sweepTarget{historyID: name, path: deletingDir})
			clearedCount++
			// 写入中占位 meta 的计数字段已在读取边界归一化为 0
			if state == trashWriteStateComplete {
				clearedSize += meta.TotalFileSize
			}
		}
	}

	// 若存在待擦除目录，启动后台 Goroutine 进行异步物理清理，避免阻塞 HTTP/GraphQL 请求
	s.sweepAsync(targets)

	return &shared.EmptyTrashResultDTO{
		ClearedCount: clearedCount,
		ClearedSize:  clearedSize,
	}, errB.Build()
}

// sweepAsync 分批后台清扫，失败时通过注入的 logger 记录历史 ID 与路径，不丢弃错误。
func (s *ImageTrasher) sweepAsync(targets []sweepTarget) {
	go func() {
		const batchSize = 64
		for i := 0; i < len(targets); i += batchSize {
			end := i + batchSize
			if end > len(targets) {
				end = len(targets)
			}
			batch := targets[i:end]
			paths := make([]string, len(batch))
			for j, target := range batch {
				paths[j] = target.path
			}
			if err := s.sweeper.Sweep(paths); err != nil {
				for _, target := range batch {
					s.logger.Error("failed to sweep trash history",
						zap.String("historyID", target.historyID),
						zap.String("path", target.path),
						zap.Error(err),
					)
				}
			}
		}
	}()
}

// #endregion

// FindTrashHistory 获取历史记录列表迭代器
func (s *ImageTrasher) FindTrashHistory(ctx context.Context) iter.Seq2[*shared.TrashHistoryItemDTO, error] {
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
			meta, _, err := readTrashMeta(metaBytes)
			if err != nil {
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

var _ image.Trasher = (*ImageTrasher)(nil)
