package urlconv

import (
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
	require.NotEmpty(t, signedURL)

	assert.Contains(t, signedURL, "image?")
	assert.Contains(t, signedURL, "path=")
	assert.Contains(t, signedURL, "t=")
	assert.Contains(t, signedURL, "s=")
	assert.Contains(t, signedURL, "sig=")
}

func TestValidateRequest(t *testing.T) {
	signer := NewSigner("test-secret-key", t.TempDir())

	tempFile := filepath.Join(t.TempDir(), "test.jpg")
	err := os.WriteFile(tempFile, []byte("test"), 0644)
	require.NoError(t, err)

	_, err = signer.GenerateSignedURL(tempFile)
	require.NoError(t, err)

	valid, err := signer.ValidateRequest("test.jpg", "123", "4", "test", "", "")
	require.Error(t, err)
	assert.False(t, valid)
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

	path, err := signer.ValidateSignedURL(signedURL)
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
