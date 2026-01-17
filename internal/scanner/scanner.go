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

type DirectoryInfo struct {
	Path               string
	ImageCount         int
	SubdirectoryCount  int
	LatestImageModTime time.Time
	LatestImagePath    string
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

func (s *Scanner) ScanDirectories(relPath string) ([]*DirectoryInfo, error) {
	absPath := filepath.Join(s.rootDir, relPath)
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var directories []*DirectoryInfo

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		subRelPath := filepath.Join(relPath, entry.Name())
		subAbsPath := filepath.Join(absPath, entry.Name())

		imageCount, subdirectoryCount, latestModTime, latestImagePath, err := s.analyzeDirectory(subAbsPath)
		if err != nil {
			continue
		}

		if imageCount == 0 && subdirectoryCount == 0 {
			continue
		}

		dirInfo := &DirectoryInfo{
			Path:               subRelPath,
			ImageCount:         imageCount,
			SubdirectoryCount:  subdirectoryCount,
			LatestImageModTime: latestModTime,
			LatestImagePath:    latestImagePath,
		}

		directories = append(directories, dirInfo)
	}

	return directories, nil
}

func (s *Scanner) analyzeDirectory(absPath string) (int, int, time.Time, string, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return 0, 0, time.Time{}, "", err
	}

	imageCount := 0
	subdirectoryCount := 0
	var latestModTime time.Time
	var latestImagePath string

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if entry.IsDir() {
			subdirectoryCount++
			continue
		}

		if !xmp.IsSupportedImage(entry.Name()) {
			continue
		}

		imageCount++
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(latestModTime) {
			latestModTime = info.ModTime()
			latestImagePath = filepath.Join(absPath, entry.Name())
		}
	}

	return imageCount, subdirectoryCount, latestModTime, latestImagePath, nil
}

func (s *Scanner) ValidateDirectoryPath(relPath string) error {
	if strings.Contains(relPath, "..") {
		return fmt.Errorf("invalid path: contains parent directory reference")
	}

	if strings.Contains(relPath, ":") {
		return fmt.Errorf("invalid path: contains drive letter")
	}

	if filepath.IsAbs(relPath) {
		return fmt.Errorf("invalid path: absolute path not allowed")
	}

	absPath := filepath.Join(s.rootDir, relPath)
	cleanAbsPath := filepath.Clean(absPath)
	cleanRootDir := filepath.Clean(s.rootDir)

	relFromRoot, err := filepath.Rel(cleanRootDir, cleanAbsPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	if strings.HasPrefix(relFromRoot, "..") {
		return fmt.Errorf("invalid path: escapes root directory")
	}

	_, err = os.Stat(cleanAbsPath)
	if err != nil {
		return fmt.Errorf("directory does not exist: %w", err)
	}

	return nil
}
