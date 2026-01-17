package xmp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadExternalSamples(t *testing.T) {
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
			data, err := Read(imagePath)
			require.NoError(t, err, "Failed to read %s", file.Name())
			require.NotNil(t, data, "Read returned nil data for %s", file.Name())

			switch file.Name() {
			case "rating_1.xmp":
				assert.Equal(t, 1, data.Rating, "rating_1.xmp should have rating 1")
			case "red_marker.xmp":
				assert.Equal(t, 0, data.Rating, "red_marker.xmp should have rating 0")
			case "valid_xmp.xmp":
				assert.Equal(t, 5, data.Rating, "valid_xmp.xmp should have rating 5")
				assert.Equal(t, "keep", data.Action, "valid_xmp.xmp should have action keep")
				assert.Equal(t, "test-session-123", data.SessionID, "valid_xmp.xmp should have session ID test-session-123")
				assert.Equal(t, "default", data.Preset, "valid_xmp.xmp should have preset default")
			case "with_unknown_fields.xmp":
				assert.Equal(t, 3, data.Rating, "with_unknown_fields.xmp should have rating 3")
			case "multiple_rating_sources.xmp":
				assert.Equal(t, 4, data.Rating, "multiple_rating_sources.xmp should have rating 4")
			case "roundtrip_unknown_fields.xmp":
				assert.Equal(t, 2, data.Rating, "roundtrip_unknown_fields.xmp should have rating 2")
			}
		})
	}
}

func TestWriteAndRead(t *testing.T) {
	testData := &XMPData{
		Rating:    3,
		Action:    "keep",
		SessionID: "test-session-123",
		Timestamp: time.Now(),
		Preset:    "default",
	}

	tempFile := filepath.Join(os.TempDir(), "test-image.jpg")
	defer os.Remove(tempFile)
	defer os.Remove(tempFile + ".xmp")

	err := Write(tempFile, testData)
	require.NoError(t, err, "Failed to write XMP")

	readData, err := Read(tempFile)
	require.NoError(t, err, "Failed to read XMP")

	assert.Equal(t, testData.Rating, readData.Rating)
	assert.Equal(t, testData.Action, readData.Action)
	assert.Equal(t, testData.SessionID, readData.SessionID)
	assert.Equal(t, testData.Preset, readData.Preset)
}

func TestBatchWrite(t *testing.T) {
	testData1 := &XMPData{Rating: 1}
	testData2 := &XMPData{Rating: 2}

	tempFile1 := filepath.Join(os.TempDir(), "test-image-1.jpg")
	tempFile2 := filepath.Join(os.TempDir(), "test-image-2.jpg")

	defer os.Remove(tempFile1)
	defer os.Remove(tempFile1 + ".xmp")
	defer os.Remove(tempFile2)
	defer os.Remove(tempFile2 + ".xmp")

	dataMap := map[string]*XMPData{
		tempFile1: testData1,
		tempFile2: testData2,
	}

	success, errors := BatchWrite([]string{tempFile1, tempFile2}, dataMap)

	assert.Equal(t, 2, success)
	assert.Empty(t, errors)

	readData1, err := Read(tempFile1)
	require.NoError(t, err, "Failed to read XMP for file 1")
	assert.Equal(t, 1, readData1.Rating)

	readData2, err := Read(tempFile2)
	require.NoError(t, err, "Failed to read XMP for file 2")
	assert.Equal(t, 2, readData2.Rating)
}

func TestReadNonExistentFile(t *testing.T) {
	nonExistentFile := filepath.Join(os.TempDir(), "non-existent-image.jpg")

	data, err := Read(nonExistentFile)
	require.NoError(t, err, "Expected no error for non-existent file")
	require.NotNil(t, data, "Expected non-nil data for non-existent file")
	assert.Equal(t, 0, data.Rating, "Expected rating 0 for non-existent file")
}

func TestDebugWriteAndRead(t *testing.T) {
	testData := &XMPData{
		Rating:    3,
		Action:    "keep",
		SessionID: "test-session-123",
		Timestamp: time.Now(),
		Preset:    "default",
	}

	tempFile := filepath.Join(os.TempDir(), "debug-image.jpg")
	defer os.Remove(tempFile)
	defer os.Remove(tempFile + ".xmp")

	err := Write(tempFile, testData)
	require.NoError(t, err, "Failed to write XMP")

	data, err := os.ReadFile(tempFile + ".xmp")
	require.NoError(t, err, "Failed to read written XMP")

	t.Logf("Written XMP content:\n%s", string(data))

	readData, err := Read(tempFile)
	require.NoError(t, err, "Failed to read XMP")

	t.Logf("Read data: Rating=%d, Action=%s, SessionID=%s, Preset=%s",
		readData.Rating, readData.Action, readData.SessionID, readData.Preset)
}

func TestRequiredFields(t *testing.T) {
	testData := &XMPData{
		Rating: 5,
	}

	tempFile := filepath.Join(os.TempDir(), "test-required-fields.jpg")
	defer os.Remove(tempFile)
	defer os.Remove(tempFile + ".xmp")

	err := Write(tempFile, testData)
	require.NoError(t, err, "Failed to write XMP")

	data, err := os.ReadFile(tempFile + ".xmp")
	require.NoError(t, err, "Failed to read written XMP")

	xmpContent := string(data)

	assert.True(t, strings.Contains(xmpContent, "<xmp:Rating>5</xmp:Rating>"), "XMP file does not contain xmp:Rating field")
	assert.True(t, strings.Contains(xmpContent, "<MicrosoftPhoto:Rating>5</MicrosoftPhoto:Rating>"), "XMP file does not contain MicrosoftPhoto:Rating field")
}

func TestRead_ValidXMP(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test.jpg")
	xmpPath := imagePath + ".xmp"

	xmpContent, err := os.ReadFile("./samples/valid_xmp.xmp")
	require.NoError(t, err)

	err = os.WriteFile(xmpPath, xmpContent, 0644)
	require.NoError(t, err)

	data, err := Read(imagePath)
	require.NoError(t, err)
	assert.Equal(t, 5, data.Rating)
	assert.Equal(t, "keep", data.Action)
	assert.Equal(t, "test-session-123", data.SessionID)
	assert.Equal(t, "default", data.Preset)
	assert.Equal(t, "2024-01-15T10:30:00Z", data.Timestamp.Format(time.RFC3339))
}

func TestRead_WithUnknownFields(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test.jpg")
	xmpPath := imagePath + ".xmp"

	xmpContent, err := os.ReadFile("./samples/with_unknown_fields.xmp")
	require.NoError(t, err)

	err = os.WriteFile(xmpPath, xmpContent, 0644)
	require.NoError(t, err)

	data, err := Read(imagePath)
	require.NoError(t, err)
	assert.Equal(t, 3, data.Rating)
}

func TestRead_MultipleRatingSources(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test.jpg")
	xmpPath := imagePath + ".xmp"

	xmpContent, err := os.ReadFile("./samples/multiple_rating_sources.xmp")
	require.NoError(t, err)

	err = os.WriteFile(xmpPath, xmpContent, 0644)
	require.NoError(t, err)

	data, err := Read(imagePath)
	require.NoError(t, err)
	assert.Equal(t, 4, data.Rating)
}

func TestWrite_CreateNewXMP(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test.jpg")
	xmpPath := imagePath + ".xmp"

	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	data := &XMPData{
		Rating:    5,
		Action:    "keep",
		SessionID: "test-session",
		Timestamp: timestamp,
		Preset:    "default",
	}

	err := Write(imagePath, data)
	require.NoError(t, err)

	assert.FileExists(t, xmpPath)

	content, err := os.ReadFile(xmpPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `<xmp:Rating>5</xmp:Rating>`)
	assert.Contains(t, string(content), `<imagefunnel:Action>keep</imagefunnel:Action>`)
	assert.Contains(t, string(content), `<imagefunnel:SessionID>test-session</imagefunnel:SessionID>`)
	assert.Contains(t, string(content), `<imagefunnel:Preset>default</imagefunnel:Preset>`)
}

func TestWrite_UpdateExistingXMP_PreserveUnknownFields(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test.jpg")
	xmpPath := imagePath + ".xmp"

	originalContent := `<?xml version="1.0" encoding="UTF-8"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <Description rdf:about=""
   xmlns:xmp="http://ns.adobe.com/xap/1.0/"
   xmlns:dc="http://purl.org/dc/elements/1.1/"
   xmlns:exif="http://ns.adobe.com/exif/1.0/"
   xmp:Rating="2"
   dc:title="Original Title"
   exif:GPSLatitude="40.7128"
   exif:GPSLongitude="-74.0060"/>
 </RDF>
</x:xmpmeta>`

	err := os.WriteFile(xmpPath, []byte(originalContent), 0644)
	require.NoError(t, err)

	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	data := &XMPData{
		Rating:    5,
		Action:    "keep",
		SessionID: "test-session",
		Timestamp: timestamp,
		Preset:    "default",
	}

	err = Write(imagePath, data)
	require.NoError(t, err)

	updatedContent, err := os.ReadFile(xmpPath)
	require.NoError(t, err)
	updatedStr := string(updatedContent)

	assert.Contains(t, updatedStr, `xmp:Rating="5"`)
	assert.True(t, strings.Contains(updatedStr, `imagefunnel:Action="keep"`) || strings.Contains(updatedStr, `<imagefunnel:Action>keep</imagefunnel:Action>`), "imagefunnel:Action should be present")
	assert.Contains(t, updatedStr, `dc:title="Original Title"`)
	assert.Contains(t, updatedStr, `exif:GPSLatitude="40.7128"`)
	assert.Contains(t, updatedStr, `exif:GPSLongitude="-74.0060"`)
}

func TestWrite_UpdateExistingXMP_WithImageFunnelFields(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test.jpg")
	xmpPath := imagePath + ".xmp"

	originalContent := `<?xml version="1.0" encoding="UTF-8"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <Description rdf:about=""
   xmlns:xmp="http://ns.adobe.com/xap/1.0/"
   xmlns:imagefunnel="http://ns.imagefunnel.app/1.0/"
   xmp:Rating="2"
   imagefunnel:Action="discard"
   imagefunnel:SessionID="old-session"/>
 </RDF>
</x:xmpmeta>`

	err := os.WriteFile(xmpPath, []byte(originalContent), 0644)
	require.NoError(t, err)

	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	data := &XMPData{
		Rating:    5,
		Action:    "keep",
		SessionID: "new-session",
		Timestamp: timestamp,
		Preset:    "default",
	}

	err = Write(imagePath, data)
	require.NoError(t, err)

	updatedContent, err := os.ReadFile(xmpPath)
	require.NoError(t, err)
	updatedStr := string(updatedContent)

	assert.Contains(t, updatedStr, `xmp:Rating="5"`)
	assert.Contains(t, updatedStr, `imagefunnel:Action="keep"`)
	assert.Contains(t, updatedStr, `imagefunnel:SessionID="new-session"`)
	assert.NotContains(t, updatedStr, `imagefunnel:Action="discard"`)
	assert.NotContains(t, updatedStr, `imagefunnel:SessionID="old-session"`)
}

func TestBatchWrite_PartialFailure(t *testing.T) {
	tmpDir := t.TempDir()

	imagePaths := []string{
		filepath.Join(tmpDir, "image1.jpg"),
		filepath.Join(tmpDir, "invalid/path/image2.jpg"),
	}

	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	dataMap := map[string]*XMPData{
		imagePaths[0]: {
			Rating:    5,
			Action:    "keep",
			SessionID: "session1",
			Timestamp: timestamp,
			Preset:    "default",
		},
		imagePaths[1]: {
			Rating:    3,
			Action:    "maybe",
			SessionID: "session2",
			Timestamp: timestamp,
			Preset:    "default",
		},
	}

	success, errors := BatchWrite(imagePaths, dataMap)
	assert.Equal(t, 1, success)
	assert.Len(t, errors, 1)
}

func TestGetXMPPath(t *testing.T) {
	tests := []struct {
		imagePath string
		expected  string
	}{
		{"image.jpg", "image.jpg.xmp"},
		{"path/to/image.png", "path/to/image.png.xmp"},
		{"image.webp", "image.webp.xmp"},
	}

	for _, tt := range tests {
		t.Run(tt.imagePath, func(t *testing.T) {
			result := GetXMPPath(tt.imagePath)
			assert.Equal(t, tt.expected, result)
		})
	}
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

func TestReadWriteRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test.jpg")

	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	originalData := &XMPData{
		Rating:    5,
		Action:    "keep",
		SessionID: "test-session-123",
		Timestamp: timestamp,
		Preset:    "custom-preset",
	}

	err := Write(imagePath, originalData)
	require.NoError(t, err)

	readData, err := Read(imagePath)
	require.NoError(t, err)

	assert.Equal(t, originalData.Rating, readData.Rating)
	assert.Equal(t, originalData.Action, readData.Action)
	assert.Equal(t, originalData.SessionID, readData.SessionID)
	assert.Equal(t, originalData.Timestamp, readData.Timestamp)
	assert.Equal(t, originalData.Preset, readData.Preset)
}

func TestReadWriteRoundTrip_WithUnknownFields(t *testing.T) {
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "roundtrip_unknown_fields.jpg")
	xmpPath := imagePath + ".xmp"

	originalContent, err := os.ReadFile("./samples/roundtrip_unknown_fields.xmp")
	require.NoError(t, err)

	err = os.WriteFile(xmpPath, originalContent, 0644)
	require.NoError(t, err)

	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	updateData := &XMPData{
		Rating:    5,
		Action:    "keep",
		SessionID: "test-session",
		Timestamp: timestamp,
		Preset:    "default",
	}

	err = Write(imagePath, updateData)
	require.NoError(t, err)

	readData, err := Read(imagePath)
	require.NoError(t, err)

	assert.Equal(t, updateData.Rating, readData.Rating)
	assert.Equal(t, updateData.Action, readData.Action)

	updatedContent, err := os.ReadFile(xmpPath)
	require.NoError(t, err)
	updatedStr := string(updatedContent)
	assert.True(t, strings.Contains(updatedStr, `dc:title="Original Title"`) || strings.Contains(updatedStr, `<dc:title>Original Title</dc:title>`), "dc:title should be preserved as attribute or element")
}
