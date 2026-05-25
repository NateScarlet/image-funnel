package directory

import (
	"strings"

	"main/internal/apperror"
	"main/internal/scalar"
)

const idPrefix = "dir:"

func encodeID(relPath string) scalar.ID {
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

	return strings.TrimPrefix(idStr, idPrefix), nil
}
