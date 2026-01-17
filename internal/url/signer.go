package url

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"path/filepath"
	"time"
)

// Signer is a utility for generating and validating signed URLs
type Signer struct {
	secretKey []byte
	rootDir   string
}

// NewSigner creates a new Signer with the given secret key and root directory
func NewSigner(secretKey, rootDir string) *Signer {
	return &Signer{
		secretKey: []byte(secretKey),
		rootDir:   rootDir,
	}
}

// GenerateSignedURL creates a signed URL for the given image path
func (s *Signer) GenerateSignedURL(path string) (string, error) {
	relativePath, err := s.toRelativePath(path)
	if err != nil {
		return "", err
	}

	timestamp := time.Now().Unix()

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
		return absPath, nil
	}

	relPath, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return absPath, nil
	}

	return relPath, nil
}

// ValidateSignedURL validates the signature in the given URL
func (s *Signer) ValidateSignedURL(urlStr string) (string, error) {
	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %v", err)
	}

	// Get the query parameters
	params := parsedURL.Query()

	// Get the required parameters
	path := params.Get("path")
	timestampStr := params.Get("t")
	signature := params.Get("sig")

	if path == "" || timestampStr == "" || signature == "" {
		return "", fmt.Errorf("missing required parameters")
	}

	// Create the message to verify
	message := fmt.Sprintf("%s|%s", path, timestampStr)

	// Create the expected signature
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(message))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	// Compare the signatures
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return "", fmt.Errorf("invalid signature")
	}

	return path, nil
}

// ValidateRequest validates the signature from request parameters
func (s *Signer) ValidateRequest(path, timestamp, signature string) (bool, error) {
	if path == "" || timestamp == "" || signature == "" {
		return false, fmt.Errorf("missing required parameters")
	}

	// Create the message to verify
	message := fmt.Sprintf("%s|%s", path, timestamp)

	// Create the expected signature
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(message))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	// Compare the signatures
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return false, fmt.Errorf("invalid signature")
	}

	return true, nil
}
