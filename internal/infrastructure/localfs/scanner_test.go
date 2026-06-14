package localfs

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testContext struct {
	rootDir     string
	imageRepo   *ImageRepository
	imageMover  *ImageMover
	memoRepo    *MemoRepository
	dirRepo     directory.Repository
	dirAnalyzer *DirectoryAnalyzer
}

func newTestContext(t *testing.T) *testContext {
	rootDir := t.TempDir()
	factory := domainimage.NewFactory(newMockMetadataRepository(), nil, rootDir)
	dirRepo := NewDirectoryRepository(rootDir)
	imgRepo := NewImageRepository(rootDir, factory, dirRepo)
	return &testContext{
		rootDir:     rootDir,
		imageRepo:   imgRepo,
		imageMover:  NewImageMover(rootDir, imgRepo, domainimage.NewFilterBuilder()),
		memoRepo:    NewMemoRepository(rootDir),
		dirRepo:     dirRepo,
		dirAnalyzer: NewDirectoryAnalyzer(rootDir, factory, dirRepo),
	}
}

func TestNewRepository(t *testing.T) {
	ctx := newTestContext(t)
	assert.NotNil(t, ctx.imageRepo)
	assert.NotEmpty(t, ctx.rootDir)
}

func TestFindImages(t *testing.T) {
	ctx := newTestContext(t)

	testFile := filepath.Join(ctx.rootDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	images := collectImages(ctx.imageRepo.Find(context.Background(), "."))
	require.Len(t, images, 1)
	assert.Equal(t, "test.jpg", images[0].Filename())
}

func TestFindImages_EmptyDirectory(t *testing.T) {
	ctx := newTestContext(t)

	images := collectImages(ctx.imageRepo.Find(context.Background(), "."))
	require.Empty(t, images)
}

func TestFindDirectories(t *testing.T) {
	ctx := newTestContext(t)

	subDir := filepath.Join(ctx.rootDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(subDir, "test.jpg")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	dirs := collectDirInfos(ctx.dirRepo.Find(context.Background(), "."))
	require.Len(t, dirs, 1)
	assert.Equal(t, "subdir", dirs[0].RelPath())
}

func TestAnalyzeDirectory(t *testing.T) {
	ctx := newTestContext(t)

	testFile := filepath.Join(ctx.rootDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	stats, err := ctx.dirAnalyzer.Analyze(context.Background(), ".")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ImageCount())
	assert.Equal(t, 0, stats.SubdirectoryCount())
	assert.NotNil(t, stats.LatestImage())
	assert.Equal(t, testFile, filepath.Join(ctx.rootDir, stats.LatestImage().RelPath()))
	assert.Equal(t, 1, stats.RatingCounts()[0])
}

func collectImages(seq iter.Seq2[*domainimage.Image, error]) []*domainimage.Image {
	var images []*domainimage.Image
	for img, err := range seq {
		if err != nil {
			return nil
		}
		images = append(images, img)
	}
	return images
}

func collectDirInfos(seq iter.Seq2[*directory.Directory, error]) []*directory.Directory {
	var dirs []*directory.Directory
	for dir, err := range seq {
		if err != nil {
			return nil
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

func TestMoveImages(t *testing.T) {
	ctx := newTestContext(t)

	// 创建源子目录
	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	// 写入测试图片与伴随文件
	// 1. img1.jpg (带主同名扩展伴随文件 img1.jpg.txt 与 img1.jpg.json)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg.txt"), []byte("prompt1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg.json"), []byte("meta1"), 0644)
	require.NoError(t, err)

	// 2. img2.jpg (不带伴随文件)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img2.jpg"), []byte("img2"), 0644)
	require.NoError(t, err)

	// 3. 另一张无关文件
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "other.txt"), []byte("other"), 0644)
	require.NoError(t, err)

	// 测试一：正常过滤与移动
	filter := shared.ImageFilters{
		Rating: []int{0},
	}

	// 移动到相对于根目录的新目录
	moved, targetAbsDir, err := ctx.imageMover.Move(context.Background(), srcDir, filter, "target-dir")
	require.NoError(t, err)
	assert.Equal(t, 2, moved) // 应该移动了 img1.jpg 和 img2.jpg 两张图片

	// 验证文件是否已物理移到目标目录中
	targetPath := filepath.Join(ctx.rootDir, "target-dir")
	assert.Equal(t, targetPath, targetAbsDir)
	assert.FileExists(t, filepath.Join(targetPath, "img1.jpg"))
	assert.FileExists(t, filepath.Join(targetPath, "img1.jpg.txt"))
	assert.FileExists(t, filepath.Join(targetPath, "img1.jpg.json"))
	assert.FileExists(t, filepath.Join(targetPath, "img2.jpg"))

	// 验证原目录下的文件已被清空，且无关文件 other.txt 仍被保留在原处
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg.txt"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg.json"))
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "other.txt"))

	// 测试二：安全边界校验越界（尝试移动到根目录外部）
	_, _, err = ctx.imageMover.Move(context.Background(), srcDir, filter, "../outside")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes root directory")
}

func TestImageRepository_AbsolutePaths(t *testing.T) {
	ctx := newTestContext(t)

	// 写入测试图片
	testFileName := "test.jpg"
	testFileAbsPath := filepath.Join(ctx.rootDir, testFileName)
	err := os.WriteFile(testFileAbsPath, []byte("test"), 0644)
	require.NoError(t, err)

	// 测试一：Get 接口接收绝对路径，应该返回错误
	img, err := ctx.imageRepo.Get(context.Background(), testFileAbsPath)
	require.Error(t, err)
	assert.Nil(t, img)
	assert.Contains(t, err.Error(), "absolute path not allowed")

	// 测试二：Find 接口接收绝对路径，迭代器应该返回错误
	var findErr error
	for _, err := range ctx.imageRepo.Find(context.Background(), ctx.rootDir) {
		if err != nil {
			findErr = err
			break
		}
	}
	require.Error(t, findErr)
	assert.Contains(t, findErr.Error(), "absolute path not allowed")
}

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
	historyId, fileCount, err := ctx.imageMover.Trash(context.Background(), srcDir, filter)
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
	for h, err := range ctx.imageMover.FindTrashHistory(context.Background()) {
		require.NoError(t, err)
		historyList = append(historyList, h)
	}
	require.Len(t, historyList, 1)
	assert.Equal(t, historyId, historyList[0].ID.String())
	assert.Equal(t, 3, historyList[0].TotalFileCount)
	assert.True(t, historyList[0].TotalFileSize > 0)

	// 3. 测试 UndoTrash 还原
	restored, err := ctx.imageMover.UndoTrash(context.Background(), historyId)
	require.NoError(t, err)
	assert.Equal(t, 3, restored)

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

	filter := shared.ImageFilters{
		Rating: []int{0},
	}

	historyId, fileCount, err := ctx.imageMover.Trash(context.Background(), srcDir, filter)
	require.NoError(t, err)
	assert.Equal(t, 1, fileCount)

	// 1. 测试 EmptyTrash，设置保留期为 10 分钟，此时刚删的文件不应该被清理
	cleared, err := ctx.imageMover.EmptyTrash(context.Background(), 10*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 0, cleared)
	assert.FileExists(t, filepath.Join(ctx.rootDir, trashDirName, historyId, "meta.json"))

	// 2. 测试 EmptyTrash，设置保留期为 0 秒（或负值，强制清空）
	cleared, err = ctx.imageMover.EmptyTrash(context.Background(), -1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, cleared)

	// 验证暂存区已清空
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, trashDirName, historyId))
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
	for img, err := range ctx.imageRepo.Find(context.Background(), srcDir) {
		require.NoError(t, err)
		if filepath.Base(img.RelPath()) == "img1.png" {
			img1PngID = img.ID()
		}
	}
	require.NotEmpty(t, img1PngID)

	// 过滤条件：仅匹配 img1.png
	filter := shared.ImageFilters{
		ID: []scalar.ID{img1PngID},
	}

	// 执行 Trash
	historyId, fileCount, err := ctx.imageMover.Trash(context.Background(), srcDir, filter)
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

