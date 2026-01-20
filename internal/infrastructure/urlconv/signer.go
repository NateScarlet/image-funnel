package urlconv

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"main/internal/application/image"
)

type Signer struct {
	secretKey []byte
	rootDir   string
}

func NewSigner(secretKey, rootDir string) *Signer {
	return &Signer{
		secretKey: []byte(secretKey),
		rootDir:   rootDir,
	}
}

func (s *Signer) GenerateSignedURL(path string, opts ...image.SignOption) (string, error) {
	relativePath, err := s.toRelativePath(path)
	if err != nil {
		return "", err
	}

	absPath := filepath.Join(s.rootDir, relativePath)
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %v", err)
	}

	timestamp := fileInfo.ModTime().Unix()
	size := fileInfo.Size()

	params := url.Values{}
	for _, opt := range opts {
		opt(params)
	}

	params.Set("path", relativePath)
	params.Set("t", fmt.Sprintf("%d", timestamp))
	params.Set("s", fmt.Sprintf("%d", size))

	signature := s.calculateSignature(relativePath, fmt.Sprintf("%d", timestamp), fmt.Sprintf("%d", size), params.Get("w"), params.Get("q"))
	params.Set("sig", signature)

	return fmt.Sprintf("image?%s", params.Encode()), nil
}

func (s *Signer) calculateSignature(path, timestamp, size, w, q string) string {
	mac := hmac.New(sha256.New, s.secretKey)
	fmt.Fprintf(mac, "%s|%s|%s|%s|%s", path, timestamp, size, w, q)
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Signer) toRelativePath(absPath string) (string, error) {
	absPath = filepath.Clean(absPath)
	rootDir := filepath.Clean(s.rootDir)

	if !filepath.IsAbs(absPath) {
		return filepath.ToSlash(absPath), nil
	}

	relPath, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return filepath.ToSlash(absPath), nil
	}

	relPath = filepath.Clean(relPath)
	if relPath == "." {
		relPath = filepath.Base(absPath)
	}

	return filepath.ToSlash(relPath), nil
}

func (s *Signer) ValidateSignedURL(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %v", err)
	}

	params := parsedURL.Query()
	path := params.Get("path")
	return path, s.ValidateRequestFromValues(params)
}

func (s *Signer) ValidateRequestFromValues(params url.Values) error {
	path := params.Get("path")
	timestampStr := params.Get("t")
	sizeStr := params.Get("s")
	signature := params.Get("sig")
	w := params.Get("w")
	q := params.Get("q")

	if path == "" || timestampStr == "" || sizeStr == "" || signature == "" {
		return fmt.Errorf("missing required parameters")
	}

	expectedSignature := s.calculateSignature(path, timestampStr, sizeStr, w, q)

	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}
