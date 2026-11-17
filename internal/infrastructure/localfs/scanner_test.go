package localfs

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/domain/relpath"
	"main/internal/shared"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type testContext struct {
	rootDir      string
	imageRepo    *ImageRepository
	imageMover   *ImageMover
	imageTrasher *ImageTrasher
	noteRepo     *NoteRepository
	dirRepo      directory.Repository
	dirAnalyzer  *DirectoryAnalyzer
}

func newTestContext(t *testing.T) *testContext {
	rootDir := t.TempDir()
	factory := domainimage.NewFactory(newMockMetadataRepository(), nil, rootDir)
	dirRepo := NewDirectoryRepository(rootDir)
	imgRepo := NewImageRepository(rootDir, factory, dirRepo)
	filterBuilder := domainimage.NewFilterBuilder()
	return &testContext{
		rootDir:      rootDir,
		imageRepo:    imgRepo,
		imageMover:   NewImageMover(rootDir, imgRepo, filterBuilder),
		imageTrasher: NewImageTrasher(rootDir, imgRepo, filterBuilder, NewPhysicalDeleteSweeper(), zap.NewNop()),
		noteRepo:     NewNoteRepository(rootDir),
		dirRepo:      dirRepo,
		dirAnalyzer:  NewDirectoryAnalyzer(rootDir, factory, dirRepo),
	}
}

// rel 构造测试用根相对路径，构造失败即测试用例本身有问题
func (c *testContext) rel(t *testing.T, relPath string) relpath.Path {
	t.Helper()
	p, err := relpath.New(c.rootDir, relPath)
	if err != nil {
		t.Fatalf("构造根相对路径 %q 失败: %v", relPath, err)
	}
	return p
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

	images := collectImages(ctx.imageRepo.Find(context.Background(), relpath.Path{}))
	require.Len(t, images, 1)
	assert.Equal(t, "test.jpg", images[0].Filename())
}

func TestFindImages_EmptyDirectory(t *testing.T) {
	ctx := newTestContext(t)

	images := collectImages(ctx.imageRepo.Find(context.Background(), relpath.Path{}))
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

	dirs := collectDirInfos(ctx.dirRepo.Find(context.Background(), relpath.Path{}))
	require.Len(t, dirs, 1)
	assert.Equal(t, "subdir", dirs[0].RelPath().String())
}

func TestAnalyzeDirectory(t *testing.T) {
	ctx := newTestContext(t)

	testFile := filepath.Join(ctx.rootDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	stats, err := ctx.dirAnalyzer.Analyze(context.Background(), relpath.Path{})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ImageCount())
	assert.Equal(t, 0, stats.SubdirectoryCount())
	assert.NotNil(t, stats.LatestImage())
	assert.Equal(t, testFile, stats.LatestImage().RelPath().Abs(ctx.rootDir))
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
	moved, targetAbsDir, err := ctx.imageMover.Move(context.Background(), ctx.rel(t, srcDir), filter, ctx.rel(t, "target-dir"))
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
}
