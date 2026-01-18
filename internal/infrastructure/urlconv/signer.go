package urlconv

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"main/internal/application/session"
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

func (s *Signer) GenerateSignedURL(path string) (string, error) {
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

	message := fmt.Sprintf("%s|%d", relativePath, timestamp)

	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(message))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	params := url.Values{}
	params.Add("path", relativePath)
	params.Add("t", fmt.Sprintf("%d", timestamp))
	params.Add("sig", signature)

	return fmt.Sprintf("image?%s", params.Encode()), nil
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
	timestampStr := params.Get("t")
	signature := params.Get("sig")

	if path == "" || timestampStr == "" || signature == "" {
		return "", fmt.Errorf("missing required parameters")
	}

	message := fmt.Sprintf("%s|%s", path, timestampStr)

	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(message))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return "", fmt.Errorf("invalid signature")
	}

	return path, nil
}

func (s *Signer) ValidateRequest(path, timestamp, signature string) (bool, error) {
	if path == "" || timestamp == "" || signature == "" {
		return false, fmt.Errorf("missing required parameters")
	}

	message := fmt.Sprintf("%s|%s", path, timestamp)

	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(message))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return false, fmt.Errorf("invalid signature")
	}

	return true, nil
}

var _ session.URLSigner = (*Signer)(nil)
