package xmpsidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"main/internal/domain/metadata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadExternalSamples(t *testing.T) {
	repo := NewRepository()
	samplesDir := "./samples"

	files, err := os.ReadDir(samplesDir)
	require.NoError(t, err, "Failed to read samples directory")

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		samplePath := filepath.Join(samplesDir, file.Name())
		imagePath := strings.TrimSuffix(samplePath, ".xmp")
		t.Run(file.Name(), func(t *testing.T) {
			data, err := repo.Read(imagePath)
			require.NoError(t, err, "Failed to read %s", file.Name())
			require.NotNil(t, data, "Read returned nil data for %s", file.Name())

			switch file.Name() {
			case "rating_1.xmp":
				assert.Equal(t, 1, data.Rating(), "rating_1.xmp should have rating 1")
			case "red_marker.xmp":
				assert.Equal(t, 0, data.Rating(), "red_marker.xmp should have rating 0")
			case "valid_xmp.xmp":
				assert.Equal(t, 5, data.Rating(), "valid_xmp.xmp should have rating 5")
				assert.Equal(t, "keep", data.Action(), "valid_xmp.xmp should have action keep")
			case "with_unknown_fields.xmp":
				assert.Equal(t, 3, data.Rating(), "with_unknown_fields.xmp should have rating 3")
			case "multiple_rating_sources.xmp":
				assert.Equal(t, 4, data.Rating(), "multiple_rating_sources.xmp should have rating 4")
			}
		})
	}
}

func TestWriteAndRead(t *testing.T) {
	repo := NewRepository()
	testData := metadata.NewXMPData(3, "keep", time.Now())

	tempFile := filepath.Join(os.TempDir(), "test-image.jpg")
	defer os.Remove(tempFile)
	defer os.Remove(tempFile + ".xmp")

	err := repo.Write(tempFile, testData)
	require.NoError(t, err, "Failed to write XMP")

	readData, err := repo.Read(tempFile)
	require.NoError(t, err, "Failed to read XMP")
	assert.Equal(t, testData.Rating(), readData.Rating())
	assert.Equal(t, testData.Action(), readData.Action())
}

func TestReadNonExistentFile(t *testing.T) {
	repo := NewRepository()
	nonExistentFile := filepath.Join(os.TempDir(), "non-existent-image.jpg")

	data, err := repo.Read(nonExistentFile)
	require.NoError(t, err, "Expected no error for non-existent file")
	require.Nil(t, data, "Expected nil data for non-existent file")
}

func TestIsSupportedImage(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"image.jpg", true},
		{"image.jpeg", true},
		{"image.JPG", true},
		{"image.png", true},
		{"image.PNG", true},
		{"image.webp", true},
		{"image.WEBP", true},
		{"image.avif", true},
		{"image.AVIF", true},
		{"image.gif", false},
		{"image.bmp", false},
		{"image.tiff", false},
		{"document.pdf", false},
		{"archive.zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := IsSupportedImage(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}
