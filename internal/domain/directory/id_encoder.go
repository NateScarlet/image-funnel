package directory

import (
	"fmt"
	"path/filepath"
	"strings"

	"main/internal/apperror"
	"main/internal/scalar"
)

const idPrefix = "dir:"

func encodeID(relPath string) scalar.ID {
	// 统一防御性规整：如果目录相对路径为空字符串，则规范为代表当前目录的 "."
	if relPath == "" {
		relPath = "."
	}
	return scalar.ToID(idPrefix + relPath)
}

func decodeID(id scalar.ID) (string, error) {
	idStr := id.String()
	if idStr == "" {
		return "", apperror.New("INVALID_ID", "id must not be empty", "ID 不能为空")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return "", apperror.New("INVALID_DIRECTORY_ID", "invalid directory ID format", "目录 ID 格式无效")
	}

	relPath := strings.TrimPrefix(idStr, idPrefix)
	// 校验以确保路径不是绝对路径，防御性拒绝绝对路径的入参
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("absolute path not allowed in directory ID: %s", relPath)
	}

	return relPath, nil
}
