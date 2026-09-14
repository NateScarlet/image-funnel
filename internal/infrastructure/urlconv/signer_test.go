package urlconv

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSignedURL(t *testing.T) {
	signer := NewSigner("test-secret-key", t.TempDir())

	tempFile := filepath.Join(t.TempDir(), "test.jpg")
	err := os.WriteFile(tempFile, []byte("test"), 0644)
	require.NoError(t, err)

	signedURL, err := signer.GenerateSignedURL(tempFile)
	require.NoError(t, err)
	require.NotEmpty(t, signedURL.String())

	assert.Contains(t, signedURL.String(), "image?")
	assert.Contains(t, signedURL.String(), "path=")
	assert.Contains(t, signedURL.String(), "t=")
	assert.Contains(t, signedURL.String(), "s=")
	assert.Contains(t, signedURL.String(), "sig=")
}

func TestValidateSignedURL(t *testing.T) {
	rootDir := t.TempDir()
	signer := NewSigner("test-secret-key", rootDir)

	relPath := "test.jpg"
	tempFile := filepath.Join(rootDir, relPath)
	err := os.WriteFile(tempFile, []byte("test"), 0644)
	require.NoError(t, err)

	signedURL, err := signer.GenerateSignedURL(relPath)
	require.NoError(t, err)

	path, err := signer.ValidateSignedURL(signedURL.String())
	require.NoError(t, err)
	assert.Equal(t, relPath, path)
}

func TestToRelativePath(t *testing.T) {
	signer := NewSigner("test-secret-key", t.TempDir())

	tests := []struct {
		input    string
		expected string
	}{
		{"test.jpg", "test.jpg"},
		{"subdir/test.jpg", "subdir/test.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := signer.toRelativePath(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToRelativePath_Absolute(t *testing.T) {
	signer := NewSigner("test-secret-key", t.TempDir())

	absPath := filepath.Join(t.TempDir(), "test.jpg")
	result, err := signer.toRelativePath(absPath)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestValidateSignedURL_TamperedParamsFails(t *testing.T) {
	rootDir := t.TempDir()
	signer := NewSigner("test-secret-key", rootDir)

	relPath := "test.jpg"
	tempFile := filepath.Join(rootDir, relPath)
	err := os.WriteFile(tempFile, []byte("test"), 0644)
	require.NoError(t, err)

	signedURL, err := signer.GenerateSignedURL(relPath)
	require.NoError(t, err)

	// 篡改 w 参数
	parsed, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := parsed.Query()
	q.Set("w", "9999")
	parsed.RawQuery = q.Encode()

	_, err = signer.ValidateSignedURL(parsed.String())
	assert.Error(t, err, "tampered width should fail validation")
}

func TestValidateSignedURL_TamperedTimestampFails(t *testing.T) {
	rootDir := t.TempDir()
	signer := NewSigner("test-secret-key", rootDir)

	relPath := "test.jpg"
	tempFile := filepath.Join(rootDir, relPath)
	err := os.WriteFile(tempFile, []byte("test"), 0644)
	require.NoError(t, err)

	signedURL, err := signer.GenerateSignedURL(relPath)
	require.NoError(t, err)

	// 篡改 timestamp
	parsed, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := parsed.Query()
	q.Set("t", "9999999999")
	parsed.RawQuery = q.Encode()

	_, err = signer.ValidateSignedURL(parsed.String())
	assert.Error(t, err, "tampered timestamp should fail validation")
}