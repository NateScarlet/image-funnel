package memo

import (
	"main/internal/apperror"
	"main/internal/scalar"
	"strings"
)

const idPrefix = "memo:"

// Memo 表示图片的备忘信息
type Memo struct {
	id         scalar.ID
	absPath    string // 绝对路径
	content    string // 剥离后的正文
	rawContent string // 完整的原始内容
	hidden     bool   // 是否被隐藏
}

// NewMemo 创建一个新的备忘信息
// path 必须是绝对路径，content 为包含 frontmatter 的完整内容
func NewMemo(id scalar.ID, absPath string, content string) *Memo {
	hidden, parsedContent := ParseMemoContent(content)
	return &Memo{
		id:         id,
		absPath:    absPath,
		content:    parsedContent,
		rawContent: content,
		hidden:     hidden,
	}
}

// ParseMemoContent 解析备忘录文本，返回是否隐藏以及剔除了 frontmatter 后的纯文本正文
func ParseMemoContent(raw string) (hidden bool, body string) {
	// 统一处理换行符，以便正则或前缀匹配
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")

	// 判断是否以 "---\n" 开头且包含成对的 "---"
	if !strings.HasPrefix(normalized, "---\n") {
		return false, raw
	}

	parts := strings.SplitN(normalized, "---\n", 3)
	if len(parts) < 3 {
		return false, raw
	}

	frontmatter := parts[1]
	body = parts[2]

	// 简单逐行解析 frontmatter
	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 2 {
			k := strings.ToLower(strings.TrimSpace(kv[0]))
			v := strings.ToLower(strings.TrimSpace(kv[1]))
			if k == "hidden" || k == "hide" {
				if v == "true" {
					hidden = true
				}
			}
		}
	}

	// 去除正文首尾多余换行与空白
	body = strings.TrimSpace(body)
	return hidden, body
}

// ID 返回备忘 ID
func (m *Memo) ID() scalar.ID {
	return m.id
}

// Path 返回备忘录关联的图片路径（绝对路径）
func (m *Memo) AbsPath() string {
	return m.absPath
}

// Content 返回备忘内容（纯文本正文）
func (m *Memo) Content() string {
	return m.content
}

// RawContent 返回完整原始内容
func (m *Memo) RawContent() string {
	return m.rawContent
}

// Hidden 返回是否被隐藏
func (m *Memo) Hidden() bool {
	return m.hidden
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
