package note

import (
	"main/internal/apperror"
	"main/internal/scalar"
	"path/filepath"
	"strings"
)

const idPrefix = "note:"

// Note 表示笔记信息
type Note struct {
	id         scalar.ID
	relPath    string // 相对路径
	absPath    string // 绝对路径
	content    string // 剥离后的正文
	rawContent string // 完整的原始内容
	hidden     bool   // 是否被隐藏
}

// FromRepository 从仓库加载笔记信息，ID 由领域层根据相对路径自动生成
// 仅由 Repository 实现调用，外部不得直接构造。此处直接通过结构体字面量实例化，信任持久层数据。
func FromRepository(relPath string, absPath string, content string) *Note {
	hidden, parsedContent := ParseContent(content)
	return &Note{
		id:         encodeID(relPath),
		relPath:    relPath,
		absPath:    absPath,
		content:    parsedContent,
		rawContent: content,
		hidden:     hidden,
	}
}

// ParseContent 解析笔记文本，返回是否隐藏以及剔除了 frontmatter 后的纯文本正文
func ParseContent(raw string) (hidden bool, body string) {
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
	for line := range strings.SplitSeq(frontmatter, "\n") {
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

// ID 返回笔记 ID
func (n *Note) ID() scalar.ID {
	return n.id
}

// RelPath 返回笔记相对路径
func (n *Note) RelPath() string {
	return n.relPath
}

// Path 返回笔记绝对路径
func (n *Note) AbsPath() string {
	return n.absPath
}

// Content 返回笔记内容（纯文本正文）
func (n *Note) Content() string {
	return n.content
}

// RawContent 返回完整原始内容
func (n *Note) RawContent() string {
	return n.rawContent
}

// Hidden 返回是否被隐藏
func (n *Note) Hidden() bool {
	return n.hidden
}

// encodeID 根据图片相对路径生成笔记 ID
func encodeID(relPath string) scalar.ID {
	return scalar.ToID(idPrefix + strings.TrimSuffix(filepath.ToSlash(relPath), ".md"))
}

// decodeID 从笔记 ID 中提取图片相对路径
func decodeID(id scalar.ID) (string, error) {
	idStr := id.String()
	if idStr == "" {
		return "", apperror.New("INVALID_ID", "id must not be empty", "ID 不能为空")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return "", apperror.New("INVALID_NOTE_ID", "invalid note ID format", "笔记 ID 格式无效")
	}

	return strings.TrimPrefix(idStr, idPrefix) + ".md", nil
}
