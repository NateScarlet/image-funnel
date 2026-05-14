package memo

import (
	"main/internal/apperror"
	"main/internal/scalar"
	"strings"
)

const idPrefix = "memo:"

// Memo 表示图片的备忘信息
type Memo struct {
	id      scalar.ID
	absPath string // 绝对路径
	content string
}

// NewMemo 创建一个新的备忘信息
// path 必须是绝对路径
func NewMemo(id scalar.ID, path string, content string) *Memo {
	return &Memo{
		id:      id,
		absPath: path,
		content: content,
	}
}

// ID 返回备忘 ID
func (m *Memo) ID() scalar.ID {
	return m.id
}

// Path 返回备忘录关联的图片路径（绝对路径）
func (m *Memo) AbsPath() string {
	return m.absPath
}

// Content 返回备忘内容
func (m *Memo) Content() string {
	return m.content
}

// EncodeID 根据图片相对路径生成备忘 ID
func EncodeID(relPath string) scalar.ID {
	return scalar.ToID(idPrefix + relPath)
}

// DecodeID 从备忘 ID 中提取图片相对路径
func DecodeID(id scalar.ID) (string, error) {
	idStr := id.String()
	if idStr == "" {
		return "", apperror.New("INVALID_ID", "id must not be empty", "ID 不能为空")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return "", apperror.New("INVALID_MEMO_ID", "invalid memo ID format", "备忘录 ID 格式无效")
	}

	return strings.TrimPrefix(idStr, idPrefix), nil
}
