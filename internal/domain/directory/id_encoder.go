package directory

import (
	"strings"

	"main/internal/apperror"
	"main/internal/scalar"
)

const idPrefix = "dir:"

func EncodeID(path string) scalar.ID {
	return scalar.ToID(idPrefix + path)
}

func DecodeID(id scalar.ID) (string, error) {
	idStr := id.String()
	if idStr == "" {
		return "", apperror.New("INVALID_ID", "id must not be empty", "ID 不能为空")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return "", apperror.New("INVALID_DIRECTORY_ID", "invalid directory ID format", "目录 ID 格式无效")
	}

	return strings.TrimPrefix(idStr, idPrefix), nil
}
