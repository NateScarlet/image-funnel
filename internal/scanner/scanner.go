package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"main/internal/xmp"
)

type ImageInfo struct {
	ID            string
	Filename      string
	Path          string
	Size          int64
	CurrentRating int
	XMPExists     bool
}

type Scanner struct {
	rootDir string
}

func NewScanner(rootDir string) *Scanner {
	return &Scanner{rootDir: rootDir}
}

func (s *Scanner) Scan() ([]*ImageInfo, error) {
	entries, err := os.ReadDir(s.rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var images []*ImageInfo

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if !xmp.IsSupportedImage(entry.Name()) {
			continue
		}

		path := filepath.Join(s.rootDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		imageInfo := &ImageInfo{
			ID:        generateID(path),
			Filename:  entry.Name(),
			Path:      path,
			Size:      info.Size(),
			XMPExists: s.xmpExists(path),
		}

		if imageInfo.XMPExists {
			xmpData, err := xmp.Read(path)
			if err == nil {
				imageInfo.CurrentRating = xmpData.Rating
			}
		}

		images = append(images, imageInfo)
	}

	return images, nil
}

func (s *Scanner) xmpExists(imagePath string) bool {
	_, err := os.Stat(imagePath + ".xmp")
	return err == nil
}

func generateID(path string) string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
