package localfs

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestScanner(t *testing.T) *Scanner {
	return NewScanner(t.TempDir(), newMockMetadataRepository(), nil)
}

func TestNewScanner(t *testing.T) {
	scanner := newTestScanner(t)
	assert.NotNil(t, scanner)
	assert.NotEmpty(t, scanner.rootDir)
}

func TestScan(t *testing.T) {
	scanner := newTestScanner(t)

	testFile := filepath.Join(scanner.rootDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	images := collectImages(scanner.Scan("."))
	require.Len(t, images, 1)
	assert.Equal(t, "test.jpg", images[0].Filename())
}

func TestScan_EmptyDirectory(t *testing.T) {
	scanner := newTestScanner(t)

	images := collectImages(scanner.Scan("."))
	require.Empty(t, images)
}

func TestScanDirectories(t *testing.T) {
	scanner := newTestScanner(t)

	subDir := filepath.Join(scanner.rootDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(subDir, "test.jpg")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	dirs := collectDirInfos(scanner.ScanDirectories("."))
	require.Len(t, dirs, 1)
	assert.Equal(t, "subdir", dirs[0].Path())
}

func TestAnalyzeDirectory(t *testing.T) {
	scanner := newTestScanner(t)

	testFile := filepath.Join(scanner.rootDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	stats, err := scanner.AnalyzeDirectory(context.Background(), ".")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ImageCount())
	assert.Equal(t, 0, stats.SubdirectoryCount())
	assert.NotNil(t, stats.LatestImage())
	assert.Equal(t, testFile, stats.LatestImage().Path())
	assert.Equal(t, 1, stats.RatingCounts()[0])
}

func TestValidateDirectoryPath(t *testing.T) {
	scanner := newTestScanner(t)

	err := scanner.validateDirectoryPath(".")
	require.NoError(t, err)
}

func TestValidateDirectoryPath_Invalid(t *testing.T) {
	scanner := newTestScanner(t)

	err := scanner.validateDirectoryPath("../test")
	assert.Error(t, err)
}

func TestValidateDirectoryPath_Absolute(t *testing.T) {
	scanner := newTestScanner(t)

	err := scanner.validateDirectoryPath("/absolute/path")
	assert.Error(t, err)
}

func TestValidateDirectoryPath_WithDriveLetter(t *testing.T) {
	scanner := newTestScanner(t)

	err := scanner.validateDirectoryPath("C:\\Windows\\System32")
	assert.Error(t, err)
}

func TestValidateDirectoryPath_PathTraversal(t *testing.T) {
	scanner := newTestScanner(t)

	testCases := []string{
		"../escape",
		"../../escape",
		"./../escape",
		"subdir/../../escape",
		"..\\escape",
		"..\\..\\escape",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			err := scanner.validateDirectoryPath(tc)
			assert.Error(t, err, "path traversal should be rejected: %s", tc)
		})
	}
}

func TestAnalyzeDirectory_PathTraversal(t *testing.T) {
	scanner := newTestScanner(t)

	_, err := scanner.AnalyzeDirectory(context.Background(), "../escape")
	assert.Error(t, err)
}

func TestAnalyzeDirectory_AbsolutePath(t *testing.T) {
	scanner := newTestScanner(t)

	_, err := scanner.AnalyzeDirectory(context.Background(), "/absolute/path")
	assert.Error(t, err)
}

func TestScanDirectories_PathTraversal(t *testing.T) {
	scanner := newTestScanner(t)

	_, err := collectDirInfosWithError(scanner.ScanDirectories("../escape"))
	assert.Error(t, err)
}

func TestScanDirectories_AbsolutePath(t *testing.T) {
	scanner := newTestScanner(t)

	_, err := collectDirInfosWithError(scanner.ScanDirectories("/absolute/path"))
	assert.Error(t, err)
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

func collectDirInfos(seq iter.Seq2[*directory.DirectoryInfo, error]) []*directory.DirectoryInfo {
	var dirs []*directory.DirectoryInfo
	for dir, err := range seq {
		if err != nil {
			return nil
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

func collectDirInfosWithError(seq iter.Seq2[*directory.DirectoryInfo, error]) ([]*directory.DirectoryInfo, error) {
	var dirs []*directory.DirectoryInfo
	for dir, err := range seq {
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}
