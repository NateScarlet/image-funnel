package localfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScanner(t *testing.T) {
	scanner := NewScanner(t.TempDir(), newMockMetadataRepository())
	assert.NotNil(t, scanner)
	assert.NotEmpty(t, scanner.rootDir)
}

func TestScan(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	testFile := filepath.Join(tmpDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	images, err := scanner.Scan(".")
	require.NoError(t, err)
	assert.Len(t, images, 1)
	assert.Equal(t, "test.jpg", images[0].Filename())
}

func TestScan_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	images, err := scanner.Scan(".")
	require.NoError(t, err)
	assert.Empty(t, images)
}

func TestScanDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	subDir := filepath.Join(tmpDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(subDir, "test.jpg")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	dirs, err := scanner.ScanDirectories(".")
	require.NoError(t, err)
	assert.Len(t, dirs, 1)
	assert.Equal(t, "subdir", dirs[0].Path())
}

func TestAnalyzeDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	testFile := filepath.Join(tmpDir, "test.jpg")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	imageCount, subdirectoryCount, latestModTime, latestImagePath, ratingCounts, err := scanner.AnalyzeDirectory(".")
	require.NoError(t, err)
	assert.Equal(t, 1, imageCount)
	assert.Equal(t, 0, subdirectoryCount)
	assert.NotZero(t, latestModTime)
	assert.Equal(t, testFile, latestImagePath)
	assert.Equal(t, 1, ratingCounts[0])
}

func TestValidateDirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	err := scanner.ValidateDirectoryPath(".")
	require.NoError(t, err)
}

func TestValidateDirectoryPath_Invalid(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	err := scanner.ValidateDirectoryPath("../test")
	assert.Error(t, err)
}

func TestValidateDirectoryPath_Absolute(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	err := scanner.ValidateDirectoryPath("/absolute/path")
	assert.Error(t, err)
}

func TestValidateDirectoryPath_WithDriveLetter(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	err := scanner.ValidateDirectoryPath("C:\\Windows\\System32")
	assert.Error(t, err)
}

func TestValidateDirectoryPath_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

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
			err := scanner.ValidateDirectoryPath(tc)
			assert.Error(t, err, "path traversal should be rejected: %s", tc)
		})
	}
}

func TestAnalyzeDirectory_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	_, _, _, _, _, err := scanner.AnalyzeDirectory("../escape")
	assert.Error(t, err)
}

func TestAnalyzeDirectory_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	_, _, _, _, _, err := scanner.AnalyzeDirectory("/absolute/path")
	assert.Error(t, err)
}

func TestScanDirectories_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	_, err := scanner.ScanDirectories("../escape")
	assert.Error(t, err)
}

func TestScanDirectories_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := NewScanner(tmpDir, newMockMetadataRepository())

	_, err := scanner.ScanDirectories("/absolute/path")
	assert.Error(t, err)
}
