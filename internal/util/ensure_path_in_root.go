package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

func EnsurePathInRoot(rootDir, relPath string) error {
	relPath = filepath.Clean(relPath)
	if filepath.IsAbs(relPath) {
		return fmt.Errorf("absolute path not allowed")
	}
	absPath := filepath.Join(rootDir, relPath)
	if !strings.HasPrefix(absPath, rootDir) {
		return fmt.Errorf("path escapes root directory")
	}
	relPath2, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if relPath2 != relPath {
		return fmt.Errorf("not a relative path: %s", relPath2)
	}
	return nil
}
