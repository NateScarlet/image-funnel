package xmpsidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"main/internal/domain/metadata"

	"github.com/beevik/etree"
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

func TestWrite_UpdateExistingWithoutNamespace(t *testing.T) {
	repo := NewRepository()
	tempFile := filepath.Join(os.TempDir(), "test-update-no-ns.jpg")
	xmpPath := tempFile + ".xmp"
	defer os.Remove(xmpPath)

	// 创建一个没有 ImageFunnel 命名空间的 XMP
	initialXMP := `<?xml version="1.0" encoding="UTF-8"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/">
      <xmp:Rating>1</xmp:Rating>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>`
	err := os.WriteFile(xmpPath, []byte(initialXMP), 0644)
	require.NoError(t, err)

	testData := metadata.NewXMPData(5, "keep", time.Now().Truncate(time.Second))
	err = repo.Write(tempFile, testData)
	require.NoError(t, err)

	// 验证读取
	readData, err := repo.Read(tempFile)
	require.NoError(t, err)
	assert.Equal(t, 5, readData.Rating())
	assert.Equal(t, "keep", readData.Action())

	// 验证文件内容中包含命名空间
	content, _ := os.ReadFile(xmpPath)
	assert.Contains(t, string(content), ImageFunnelNS)
}

func TestWrite_UpdateExistingAttribute(t *testing.T) {
	repo := NewRepository()
	tempFile := filepath.Join(os.TempDir(), "test-update-attr.jpg")
	xmpPath := tempFile + ".xmp"
	defer os.Remove(xmpPath)

	// 创建一个使用属性存储评分的 XMP
	initialXMP := `<?xml version="1.0" encoding="UTF-8"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmp:Rating="1" />
  </rdf:RDF>
</x:xmpmeta>`
	err := os.WriteFile(xmpPath, []byte(initialXMP), 0644)
	require.NoError(t, err)

	testData := metadata.NewXMPData(4, "later", time.Now().Truncate(time.Second))
	err = repo.Write(tempFile, testData)
	require.NoError(t, err)

	// 验证读取
	readData, err := repo.Read(tempFile)
	require.NoError(t, err)
	assert.Equal(t, 4, readData.Rating())

	// 验证属性被更新而不是添加了新元素（或者至少能正确读取）
	content, _ := os.ReadFile(xmpPath)
	assert.Contains(t, string(content), `xmp:Rating="4"`)
}

func TestWrite_InvalidFileBackup(t *testing.T) {
	repo := NewRepository()
	tempFile := filepath.Join(os.TempDir(), "test-invalid.jpg")
	xmpPath := tempFile + ".xmp"
	defer os.Remove(xmpPath)

	// 写入非法内容
	invalidContent := "<x:xmpmeta xmlns:x=\"adobe:ns:meta/\"><rdf:RDF>" // 未闭合的标签
	err := os.WriteFile(xmpPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	testData := metadata.NewXMPData(3, "keep", time.Now().Truncate(time.Second))
	err = repo.Write(tempFile, testData)
	require.NoError(t, err)

	// 验证原文件被备份（找到以 .broken 开头的文件）
	matches, err := filepath.Glob(xmpPath + ".broken*")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "Should have created a backup file with .broken suffix")

	backupPath := matches[0]
	defer os.Remove(backupPath)

	backupContent, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, invalidContent, string(backupContent))

	// 验证新文件已写入且可读
	readData, err := repo.Read(tempFile)
	require.NoError(t, err)
	assert.Equal(t, 3, readData.Rating())
}

func TestWrite_XMPTKAttribute(t *testing.T) {
	repo := NewRepository()
	tempFile := filepath.Join(os.TempDir(), "test-xmptk.jpg")
	xmpPath := tempFile + ".xmp"
	defer os.Remove(xmpPath)

	testData := metadata.NewXMPData(3, "keep", time.Now())
	err := repo.Write(tempFile, testData)
	require.NoError(t, err)

	content, err := os.ReadFile(xmpPath)
	require.NoError(t, err)

	// 验证 x:xmpmeta 元素包含 x:xmptk="XMP Core 6.0.0"
	doc := etree.NewDocument()
	err = doc.ReadFromBytes(content)
	require.NoError(t, err)

	xmpmeta := doc.FindElement("x:xmpmeta")
	require.NotNil(t, xmpmeta)

	xmptk := xmpmeta.SelectAttr("x:xmptk")
	require.NotNil(t, xmptk, "x:xmptk attribute missing")
	assert.Equal(t, "XMP Core 6.0.0", xmptk.Value)
}
