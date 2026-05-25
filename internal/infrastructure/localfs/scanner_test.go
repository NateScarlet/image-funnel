package localfs

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/infrastructure/inmem"
	"main/internal/shared"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testScannerContext struct {
	rootDir      string
	imageScanner *ImageScanner
	imageMover   *ImageMover
	memoScanner  *MemoScanner
	dirScanner   *DirectoryScanner
	dirAnalyzer  *DirectoryAnalyzer
}

func newTestScannerContext(t *testing.T) *testScannerContext {
	factory := domainimage.NewFactory(newMockMetadataRepository(), nil)
	rootDir := t.TempDir()
	dirRepo := inmem.NewDirectoryRepository(rootDir)
	imgScanner := NewImageScanner(rootDir, factory, dirRepo)
	return &testScannerContext{
		rootDir:      rootDir,
		imageScanner: imgScanner,
		imageMover:   NewImageMover(rootDir, imgScanner, domainimage.NewFilterBuilder()),
		memoScanner:  NewMemoScanner(rootDir),
		dirScanner:   NewDirectoryScanner(rootDir, dirRepo),
		dirAnalyzer:  NewDirectoryAnalyzer(rootDir, factory, dirRepo),
	}
}

func TestNewScanner(t *testing.T) {
	ctx := newTestScannerContext(t)
	assert.NotNil(t, ctx.imageScanner)
	assert.NotEmpty(t, ctx.rootDir)
}

func TestScan(t *testing.T) {
	ctx := newTestScannerContext(t)

	testFile := filepath.Join(ctx.rootDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	images := collectImages(ctx.imageScanner.Scan(context.Background(), "."))
	require.Len(t, images, 1)
	assert.Equal(t, "test.jpg", images[0].Filename())
}

func TestScan_EmptyDirectory(t *testing.T) {
	ctx := newTestScannerContext(t)

	images := collectImages(ctx.imageScanner.Scan(context.Background(), "."))
	require.Empty(t, images)
}

func TestScanDirectories(t *testing.T) {
	ctx := newTestScannerContext(t)

	subDir := filepath.Join(ctx.rootDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(subDir, "test.jpg")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	dirs := collectDirInfos(ctx.dirScanner.Scan(context.Background(), "."))
	require.Len(t, dirs, 1)
	assert.Equal(t, "subdir", dirs[0].RelPath())
}

func TestAnalyzeDirectory(t *testing.T) {
	ctx := newTestScannerContext(t)

	testFile := filepath.Join(ctx.rootDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	stats, err := ctx.dirAnalyzer.Analyze(context.Background(), ".")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ImageCount())
	assert.Equal(t, 0, stats.SubdirectoryCount())
	assert.NotNil(t, stats.LatestImage())
	assert.Equal(t, testFile, stats.LatestImage().AbsPath())
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

func TestScanner_MoveImages(t *testing.T) {
	ctx := newTestScannerContext(t)

	// 创建源子目录
	srcDir := "source-dir"
	err := os.Mkdir(filepath.Join(ctx.rootDir, srcDir), 0755)
	require.NoError(t, err)

	// 写入测试图片与伴随文件
	// 1. img1.jpg (带主同名扩展伴随文件 img1.txt 与 img1.jpg.json)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.jpg"), []byte("img1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(ctx.rootDir, srcDir, "img1.txt"), []byte("prompt1"), 0644)
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

	// 移动到同级下的新目录
	moved, targetAbsDir, err := ctx.imageMover.Move(context.Background(), srcDir, filter, "../target-dir")
	require.NoError(t, err)
	assert.Equal(t, 2, moved) // 应该移动了 img1.jpg 和 img2.jpg 两张图片

	// 验证文件是否已物理移到目标目录中
	targetPath := filepath.Join(ctx.rootDir, "target-dir")
	assert.Equal(t, targetPath, targetAbsDir)
	assert.FileExists(t, filepath.Join(targetPath, "img1.jpg"))
	assert.FileExists(t, filepath.Join(targetPath, "img1.txt"))
	assert.FileExists(t, filepath.Join(targetPath, "img1.jpg.json"))
	assert.FileExists(t, filepath.Join(targetPath, "img2.jpg"))

	// 验证原目录下的文件已被清空，且无关文件 other.txt 仍被保留在原处
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.txt"))
	assert.NoFileExists(t, filepath.Join(ctx.rootDir, srcDir, "img1.jpg.json"))
	assert.FileExists(t, filepath.Join(ctx.rootDir, srcDir, "other.txt"))

	// 测试二：安全边界校验越界（尝试移动到根目录外部）
	_, _, err = ctx.imageMover.Move(context.Background(), srcDir, filter, "../../outside")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes root directory")
}
