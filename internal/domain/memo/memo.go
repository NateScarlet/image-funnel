package memo

import (
	"main/internal/apperror"
	"main/internal/scalar"
	"path/filepath"
	"strings"
)

const idPrefix = "memo:"

// Memo 表示图片的备忘信息
type Memo struct {
	id         scalar.ID
	relPath    string // 相对路径
	absPath    string // 绝对路径
	content    string // 剥离后的正文
	rawContent string // 完整的原始内容
	hidden     bool   // 是否被隐藏
}

// FromRepository 从仓库加载备忘信息，ID 由领域层根据相对路径自动生成
// 仅由 Repository 实现调用，外部不得直接构造。此处直接通过结构体字面量实例化，信任持久层数据。
func FromRepository(relPath string, absPath string, content string) *Memo {
	hidden, parsedContent := ParseContent(content)
	return &Memo{
		id:         encodeID(relPath),
		relPath:    relPath,
		absPath:    absPath,
		content:    parsedContent,
		rawContent: content,
		hidden:     hidden,
	}
}


// ParseContent 解析备忘录文本，返回是否隐藏以及剔除了 frontmatter 后的纯文本正文
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

// RelPath 返回备忘录相对路径
func (m *Memo) RelPath() string {
	return m.relPath
}

// Path 返回备忘录绝对路径
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

// encodeID 根据图片相对路径生成备忘 ID
func encodeID(relPath string) scalar.ID {
	return scalar.ToID(idPrefix + strings.TrimSuffix(filepath.ToSlash(relPath), ".md"))
}

// decodeID 从备忘 ID 中提取图片相对路径
func decodeID(id scalar.ID) (string, error) {
	idStr := id.String()
	if idStr == "" {
		return "", apperror.New("INVALID_ID", "id must not be empty", "ID 不能为空")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return "", apperror.New("INVALID_MEMO_ID", "invalid memo ID format", "备忘录 ID 格式无效")
	}

	return strings.TrimPrefix(idStr, idPrefix) + ".md", nil
}
