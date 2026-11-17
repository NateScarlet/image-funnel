package localfs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainimage "main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// newTestTrasher 构造带可注入清扫器与日志的回收站 module，覆盖其公开接缝。
func newTestTrasher(t *testing.T, sweeper TrashSweeper, logger *zap.Logger) (*ImageTrasher, string) {
	t.Helper()
	rootDir := t.TempDir()
	factory := domainimage.NewFactory(newMockMetadataRepository(), nil, rootDir)
	dirRepo := NewDirectoryRepository(rootDir)
	imgRepo := NewImageRepository(rootDir, factory, dirRepo)
	return NewImageTrasher(rootDir, imgRepo, domainimage.NewFilterBuilder(), sweeper, logger), rootDir
}

// failingSweeper 在清扫时返回固定错误，用于验证后台清扫错误不会被静默丢弃。
type failingSweeper struct {
	err error
}

func (s failingSweeper) Sweep([]string) error { return s.err }

func TestTrashAndUndo(t *testing.T) {
	ctx := newTestContext(t)

	// 创建源子目录
	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	// 写入测试图片与伴随文件
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg.txt"), []byte("prompt1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img2.jpg"), []byte("img2"), 0644)
	require.NoError(t, err)

	filter := shared.ImageFilters{
		Rating: []int{0},
	}

	// 1. 测试 Trash 暂存
	historyId, fileCount, err := ctx.imageTrasher.Trash(context.Background(), ctx.rel(t, srcDir), filter, "")
	require.NoError(t, err)
	assert.NotEmpty(t, historyId)
	assert.Equal(t, 3, fileCount) // img1.jpg, img1.jpg.txt, img2.jpg

	// 验证原路径不存在这些文件了
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg.txt"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img2.jpg"))

	// 验证暂存区有这些文件
	trashDir := filepath.Join(ctx.rootDir, trashDirName, historyId)
	assert.FileExists(t, filepath.Join(trashDir, "meta.json"))
	assert.FileExists(t, filepath.Join(trashDir, "files", "img1.jpg"))
	assert.FileExists(t, filepath.Join(trashDir, "files", "img1.jpg.txt"))
	assert.FileExists(t, filepath.Join(trashDir, "files", "img2.jpg"))

	// 2. 测试 FindTrashHistory
	var historyList []*shared.TrashHistoryItemDTO
	for h, err := range ctx.imageTrasher.FindTrashHistory(context.Background()) {
		require.NoError(t, err)
		historyList = append(historyList, h)
	}
	require.Len(t, historyList, 1)
	assert.Equal(t, historyId, historyList[0].ID.String())
	assert.Equal(t, 3, historyList[0].TotalFileCount)
	assert.True(t, historyList[0].TotalFileSize > 0)
	assert.Equal(t, srcDir, historyList[0].SrcRelPath)

	// 3. 测试 UndoTrash 还原
	res, err := ctx.imageTrasher.UndoTrash(context.Background(), historyId)
	require.NoError(t, err)
	assert.Equal(t, 3, res.RestoredCount)
	assert.Equal(t, 0, res.ConflictCount)

	// 验证原路径重新出现了文件
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg.txt"))
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img2.jpg"))

	// 验证暂存区已被删除
	assert.NoFileExists(t, trashDir)
}

func TestEmptyTrash(t *testing.T) {
	ctx := newTestContext(t)

	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1"), 0644)
	require.NoError(t, err)
	// 伴随文件一并写入，验证统计口径含图片本体与伴随文件
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg.txt"), []byte("sidecar"), 0644)
	require.NoError(t, err)

	filter := shared.ImageFilters{
		Rating: []int{0},
	}

	historyId, fileCount, err := ctx.imageTrasher.Trash(context.Background(), ctx.rel(t, srcDir), filter, "")
	require.NoError(t, err)
	assert.Equal(t, 2, fileCount)

	// 1. 测试 EmptyTrash，设置保留期为 10 分钟，此时刚删的文件不应该被清理
	res, err := ctx.imageTrasher.EmptyTrash(context.Background(), 10*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 0, res.ClearedCount)
	assert.Equal(t, int64(0), res.ClearedSize)
	assert.FileExists(t, filepath.Join(ctx.rootDir, trashDirName, historyId, "meta.json"))

	// 2. 测试 EmptyTrash，设置保留期为 0 秒（或负值，强制清空）
	res, err = ctx.imageTrasher.EmptyTrash(context.Background(), -1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, res.ClearedCount)
	// 释放大小应等于写入测试文件的字节总和（图片本体 4 字节 + 伴随文件 7 字节）
	assert.Equal(t, int64(len("img1")+len("sidecar")), res.ClearedSize)

	// 验证 FindTrashHistory 无法再次查到该已标记删除的记录
	var historyList []*shared.TrashHistoryItemDTO
	for h, err := range ctx.imageTrasher.FindTrashHistory(context.Background()) {
		require.NoError(t, err)
		historyList = append(historyList, h)
	}
	assert.Empty(t, historyList)

	// 验证暂存区已清空（由于后台异步物理清理，使用 Eventually 等待磁盘擦除完成）
	assert.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(ctx.rootDir, trashDirName, historyId))
		return os.IsNotExist(err)
	}, 2*time.Second, 20*time.Millisecond)
}

// 占位 meta（历史正在写入时的过渡态，TotalFileSize 为 -1）应按 0 字节计入统计，
// 且不影响同批正常历史的字节数累计
func TestEmptyTrash_PlaceholderMetaCountsAsZero(t *testing.T) {
	ctx := newTestContext(t)

	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1"), 0644)
	require.NoError(t, err)

	filter := shared.ImageFilters{
		Rating: []int{0},
	}

	// 正常历史：完整写入 meta，总大小为 img1 的 4 字节
	_, _, err = ctx.imageTrasher.Trash(context.Background(), ctx.rel(t, srcDir), filter, "")
	require.NoError(t, err)

	// 占位历史：模拟断电残留的占位 meta（大小 -1），保留期设为过去以强制清空
	placeholderId := "trash_placeholder"
	placeholderDir := filepath.Join(ctx.rootDir, trashDirName, placeholderId)
	err = os.MkdirAll(placeholderDir, 0755)
	require.NoError(t, err)

	placeholderMeta := trashMeta{
		ID:             placeholderId,
		TrashedAt:      time.Now().Add(-1 * time.Hour),
		TotalFileCount: -1,
		TotalFileSize:  -1,
		SrcRelPath:     srcDir,
	}
	metaBytes, err := json.Marshal(placeholderMeta)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(placeholderDir, "meta.json"), metaBytes, 0644)
	require.NoError(t, err)

	res, err := ctx.imageTrasher.EmptyTrash(context.Background(), -1*time.Second)
	require.NoError(t, err)

	// 两条历史均被清理，但释放大小只包含正常历史的 4 字节，占位的 -1 不污染统计
	assert.Equal(t, 2, res.ClearedCount)
	assert.Equal(t, int64(len("img1")), res.ClearedSize)
}

func TestTrash_ExcludeSameBaseDifferentExt(t *testing.T) {
	ctx := newTestContext(t)

	// 创建源子目录
	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	// 写入测试文件
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.png"), []byte("png"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.png.xmp"), []byte("png-xmp"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("jpg"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg.xmp"), []byte("jpg-xmp"), 0644)
	require.NoError(t, err)

	// 找出 img1.png 的 ID
	var img1PngID scalar.ID
	for img, err := range ctx.imageRepo.Find(context.Background(), ctx.rel(t, srcDir)) {
		require.NoError(t, err)
		if img.RelPath().Base() == "img1.png" {
			img1PngID = img.ID()
		}
	}
	require.NotEmpty(t, img1PngID)

	// 过滤条件：仅匹配 img1.png
	filter := shared.ImageFilters{
		ID: []scalar.ID{img1PngID},
	}

	// 执行 Trash
	historyId, fileCount, err := ctx.imageTrasher.Trash(context.Background(), ctx.rel(t, srcDir), filter, "")
	require.NoError(t, err)
	assert.NotEmpty(t, historyId)
	assert.Equal(t, 2, fileCount) // 仅移动 img1.png 和 img1.png.xmp

	// 验证移走的文件的存在性
	trashDir := filepath.Join(ctx.rootDir, trashDirName, historyId)
	assert.FileExists(t, filepath.Join(trashDir, "files", "img1.png"))
	assert.FileExists(t, filepath.Join(trashDir, "files", "img1.png.xmp"))

	// 验证未被移走的文件依然在原处
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg.xmp"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.png"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.png.xmp"))
}

func TestIncompleteTrashRecovery(t *testing.T) {
	ctx := newTestContext(t)

	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	// 写入测试文件
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img2.jpg"), []byte("img2"), 0644)
	require.NoError(t, err)

	// 模拟断电场景 1：写入了占位 meta.json，但 filesDir 甚至还没被创建就断电了
	historyId1 := "trash_incomplete_1"
	historyDir1 := filepath.Join(ctx.rootDir, trashDirName, historyId1)
	err = os.MkdirAll(historyDir1, 0755)
	require.NoError(t, err)

	meta1 := trashMeta{
		ID:             historyId1,
		TrashedAt:      time.Now().Add(-1 * time.Hour), // 设为 1 小时前
		TotalFileCount: -1,
		TotalFileSize:  -1,
		SrcRelPath:     srcDir,
	}
	metaBytes1, err := json.Marshal(meta1)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(historyDir1, "meta.json"), metaBytes1, 0644)
	require.NoError(t, err)

	// 尝试 UndoTrash，它应该允许 filesDir 缺失，静默删除该历史目录并返回 0
	res1, err := ctx.imageTrasher.UndoTrash(context.Background(), historyId1)
	require.NoError(t, err)
	assert.Equal(t, 0, res1.RestoredCount)
	assert.Equal(t, 0, res1.ConflictCount)
	assert.NoFileExists(t, historyDir1)

	// 模拟断电场景 2：写入了占位 meta.json，开始移入文件，移走了 img1.jpg，但在移 img2.jpg 前断电了
	historyId2 := "trash_incomplete_2"
	historyDir2 := filepath.Join(ctx.rootDir, trashDirName, historyId2)
	filesDir2 := filepath.Join(historyDir2, "files")
	err = os.MkdirAll(filesDir2, 0755)
	require.NoError(t, err)

	meta2 := trashMeta{
		ID:             historyId2,
		TrashedAt:      time.Now().Add(-1 * time.Hour),
		TotalFileCount: -1,
		TotalFileSize:  -1,
		SrcRelPath:     srcDir,
	}
	metaBytes2, err := json.Marshal(meta2)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(historyDir2, "meta.json"), metaBytes2, 0644)
	require.NoError(t, err)

	// 手动把 img1.jpg 移入 filesDir2 以模拟它已经被成功 stashed 了
	err = os.Rename(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), filepath.Join(filesDir2, "img1.jpg"))
	require.NoError(t, err)

	// 此时，源目录里仅存在 img2.jpg。filesDir2 里有 img1.jpg
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img2.jpg"))
	assert.FileExists(t, filepath.Join(filesDir2, "img1.jpg"))

	// 尝试 UndoTrash。它应当只还原移到暂存区的文件，并且不会因为源目录有 img2.jpg 而产生冲突
	res2, err := ctx.imageTrasher.UndoTrash(context.Background(), historyId2)
	require.NoError(t, err)
	assert.Equal(t, 1, res2.RestoredCount) // 成功还原 1 个文件
	assert.Equal(t, 0, res2.ConflictCount)
	assert.NoFileExists(t, historyDir2)

	// 验证最终两个文件都在源目录处
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img2.jpg"))

	// 模拟断电场景 3：已移走 img1.jpg，但随后要测试 EmptyTrash（清空回收站）
	historyId3 := "trash_incomplete_3"
	historyDir3 := filepath.Join(ctx.rootDir, trashDirName, historyId3)
	filesDir3 := filepath.Join(historyDir3, "files")
	err = os.MkdirAll(filesDir3, 0755)
	require.NoError(t, err)

	meta3 := trashMeta{
		ID:             historyId3,
		TrashedAt:      time.Now().Add(-1 * time.Hour), // 设为 1 小时前
		TotalFileCount: -1,
		TotalFileSize:  -1,
		SrcRelPath:     srcDir,
	}
	metaBytes3, err := json.Marshal(meta3)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(historyDir3, "meta.json"), metaBytes3, 0644)
	require.NoError(t, err)

	// 手动移入 img1.jpg 到 filesDir3，img2.jpg 留在源目录
	err = os.Rename(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), filepath.Join(filesDir3, "img1.jpg"))
	require.NoError(t, err)

	// 尝试 EmptyTrash
	res, err := ctx.imageTrasher.EmptyTrash(context.Background(), -1*time.Second)
	require.NoError(t, err)
	// 该历史的占位 meta 大小为 -1（过渡态），应按 0 字节计入
	assert.Equal(t, 1, res.ClearedCount)
	assert.Equal(t, int64(0), res.ClearedSize)
	assert.NoFileExists(t, historyDir3)

	// 验证被移入 filesDir 的文件被清理了，而之前留在源目录的文件依然完好
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img2.jpg"))
}

func TestUndoTrashConflict(t *testing.T) {
	ctx := newTestContext(t)

	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1_original"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img2.jpg"), []byte("img2_original"), 0644)
	require.NoError(t, err)

	filter := shared.ImageFilters{}

	// 1. 将两个图片移入回收站
	historyId, fileCount, err := ctx.imageTrasher.Trash(context.Background(), ctx.rel(t, srcDir), filter, "")
	require.NoError(t, err)
	assert.Equal(t, 2, fileCount)

	// 2. 在源目录中写入一个全新的 img1.jpg 模拟文件冲突
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1_new_conflicting"), 0644)
	require.NoError(t, err)

	// 3. 执行 UndoTrash 还原
	res, err := ctx.imageTrasher.UndoTrash(context.Background(), historyId)
	require.NoError(t, err)

	// 验证结果统计
	assert.Equal(t, 1, res.RestoredCount) // img2.jpg 应当还原成功
	assert.Equal(t, 1, res.ConflictCount) // img1.jpg 应当产生冲突
	assert.True(t, strings.HasPrefix(res.ConflictDirName, "UNDO_TRASH_CONFLICT_"))

	// 验证非冲突文件 img2.jpg 成功还原
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img2.jpg"))
	content2, err := os.ReadFile(filepath.Join(ctx.rootDir, srcDir, "img2.jpg"))
	require.NoError(t, err)
	assert.Equal(t, "img2_original", string(content2))

	// 验证冲突的新文件 img1.jpg 依然留在原位且未被覆盖
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	content1New, err := os.ReadFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	require.NoError(t, err)
	assert.Equal(t, "img1_new_conflicting", string(content1New))

	// 验证原 img1.jpg 被成功移入 UNDO_TRASH_CONFLICT_<随机字符> 目录中
	conflictDirAbs := filepath.Join(ctx.rootDir, srcDir, res.ConflictDirName)
	assert.DirExists(t, conflictDirAbs)
	assert.FileExists(t, filepath.Join(conflictDirAbs, "img1.jpg"))
	content1Conflict, err := os.ReadFile(filepath.Join(conflictDirAbs, "img1.jpg"))
	require.NoError(t, err)
	assert.Equal(t, "img1_original", string(content1Conflict))

	// 验证暂存历史目录被彻底清理
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, trashDirName, historyId))
}

func TestEmptyTrash_Concurrent(t *testing.T) {
	ctx := newTestContext(t)

	// 创建 20 个回收站目录
	for i := 0; i < 20; i++ {
		srcDir := fmt.Sprintf("source-dir-%d", i)
		err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
		require.NoError(t, err)

		imgFile := filepath.Join(ctx.rootDir, srcDir, fmt.Sprintf("img%d.jpg", i))
		err = os.WriteFile(imgFile, []byte("test content"), 0644)
		require.NoError(t, err)

		_, _, err = ctx.imageTrasher.Trash(context.Background(), ctx.rel(t, srcDir), shared.ImageFilters{}, "")
		require.NoError(t, err)
	}

	const workerCount = 10
	var wg sync.WaitGroup
	errs := make([]error, workerCount)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = ctx.imageTrasher.EmptyTrash(context.Background(), -1*time.Second)
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "worker %d returned error", i)
	}
}

// 占位 meta 的 -1 哨兵只允许在 module 内部被解释：读取历史时归一化为 0，不得泄漏到 DTO
func TestFindTrashHistory_PlaceholderMetaSentinelNormalized(t *testing.T) {
	trasher, rootDir := newTestTrasher(t, NewPhysicalDeleteSweeper(), zap.NewNop())

	const historyID = "trash_placeholder_history"
	historyDir := filepath.Join(rootDir, trashDirName, historyID)
	require.NoError(t, os.MkdirAll(historyDir, 0755))
	// 模拟历史遗留的「写入中」占位 meta：-1 哨兵
	metaBytes := []byte(`{"id":"trash_placeholder_history","trashedAt":"2020-01-01T00:00:00Z","totalFileCount":-1,"totalFileSize":-1,"srcRelPath":"src","images":null}`)
	require.NoError(t, os.WriteFile(filepath.Join(historyDir, "meta.json"), metaBytes, 0644))

	var items []*shared.TrashHistoryItemDTO
	for item, err := range trasher.FindTrashHistory(context.Background()) {
		require.NoError(t, err)
		items = append(items, item)
	}
	require.Len(t, items, 1)
	assert.Equal(t, historyID, items[0].ID.String())
	assert.Equal(t, 0, items[0].TotalFileCount)
	assert.Equal(t, int64(0), items[0].TotalFileSize)
	assert.Equal(t, 0, items[0].ImageCount)
	assert.Equal(t, 0, items[0].AssociatedFileCount)
}

// 旧版本写入的完整 meta 仍可正常遍历，且派生字段计算正确
func TestFindTrashHistory_LegacyMetaReadable(t *testing.T) {
	trasher, rootDir := newTestTrasher(t, NewPhysicalDeleteSweeper(), zap.NewNop())

	const historyID = "trash_legacy"
	historyDir := filepath.Join(rootDir, trashDirName, historyID)
	require.NoError(t, os.MkdirAll(filepath.Join(historyDir, "files"), 0755))
	metaBytes := []byte(`{"id":"trash_legacy","trashedAt":"2020-01-01T00:00:00Z","totalFileCount":2,"totalFileSize":100,"srcRelPath":"src","images":[{"relPath":"a.jpg","modTime":"2020-01-01T00:00:00Z"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(historyDir, "meta.json"), metaBytes, 0644))

	var items []*shared.TrashHistoryItemDTO
	for item, err := range trasher.FindTrashHistory(context.Background()) {
		require.NoError(t, err)
		items = append(items, item)
	}
	require.Len(t, items, 1)
	assert.Equal(t, historyID, items[0].ID.String())
	assert.Equal(t, 2, items[0].TotalFileCount)
	assert.Equal(t, int64(100), items[0].TotalFileSize)
	assert.Equal(t, 1, items[0].ImageCount)
	assert.Equal(t, 1, items[0].AssociatedFileCount)
	assert.Contains(t, items[0].CoverImageAbsPath, filepath.Join(historyID, "files", "a.jpg"))
}

// 后台清扫失败必须记录可定位的错误（历史 ID 与路径），且不得静默丢弃
func TestEmptyTrash_SweepErrorLogged(t *testing.T) {
	core, observed := observer.New(zap.ErrorLevel)
	trasher, rootDir := newTestTrasher(t, failingSweeper{err: fmt.Errorf("disk failure")}, zap.New(core))

	const historyID = "trash_sweep_failure"
	historyDir := filepath.Join(rootDir, trashDirName, historyID)
	require.NoError(t, os.MkdirAll(historyDir, 0755))
	meta := trashMeta{
		ID:             historyID,
		TrashedAt:      time.Now().Add(-time.Hour),
		TotalFileCount: 1,
		TotalFileSize:  10,
		SrcRelPath:     "src",
	}
	metaBytes, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(historyDir, "meta.json"), metaBytes, 0644))

	res, err := trasher.EmptyTrash(context.Background(), -1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, res.ClearedCount)

	assert.Eventually(t, func() bool {
		return len(observed.FilterMessage("failed to sweep trash history").All()) >= 1
	}, 2*time.Second, 10*time.Millisecond)

	entry := observed.FilterMessage("failed to sweep trash history").All()[0]
	fields := entry.ContextMap()
	assert.Equal(t, historyID, fields["historyID"])
	assert.Equal(t, filepath.Join(rootDir, trashDirName, "deleting_"+historyID), fields["path"])
	assert.Equal(t, "disk failure", fields["error"])
}
