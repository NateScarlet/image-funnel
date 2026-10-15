package directory

import (
	"main/internal/apperror"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"strings"
)

const idPrefix = "dir:"

func encodeID(relPath relpath.Path) scalar.ID {
	// 根目录的 ID 字面形式为 "dir:."，保持其与既有客户端缓存兼容
	raw := relPath.String()
	if raw == "" {
		raw = "."
	}
	return scalar.ToID(idPrefix + raw)
}

// decodeID 从目录 ID 中提取根相对路径。
// rootDir 仅用于还原时重新围栏：ID 由服务端签发，但仍是不可信输入
func decodeID(rootDir string, id scalar.ID) (relpath.Path, error) {
	idStr := id.String()
	if idStr == "" {
		return relpath.Path{}, apperror.New("INVALID_ID", "id must not be empty", "ID 不能为空")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return relpath.Path{}, apperror.New("INVALID_DIRECTORY_ID", "invalid directory ID format", "目录 ID 格式无效")
	}

	rel, err := relpath.New(rootDir, strings.TrimPrefix(idStr, idPrefix))
	if err != nil {
		return relpath.Path{}, apperror.New("INVALID_DIRECTORY_ID", "invalid directory ID path", "目录 ID 中的路径无效")
	}
	return rel, nil
}
