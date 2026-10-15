package note

import (
	"main/internal/apperror"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"strings"
	"time"
)

const idPrefix = "note:"

// Note 表示笔记信息
type Note struct {
	id         scalar.ID
	relPath    relpath.Path // 根相对路径
	absPath    string       // 绝对路径
	content    string       // 剥离后的正文
	rawContent string       // 完整的原始内容
	hidden     bool         // 是否被隐藏
	modTime    time.Time    // 最后修改时间
}

// FromRepository 从仓库加载笔记信息，ID 由领域层根据相对路径自动生成
// 仅由 Repository 实现调用，外部不得直接构造。此处直接通过结构体字面量实例化，信任持久层数据。
func FromRepository(relPath relpath.Path, absPath string, content string, modTime time.Time) *Note {
	hidden, parsedContent := ParseContent(content)
	return &Note{
		id:         encodeID(relPath),
		relPath:    relPath,
		absPath:    absPath,
		content:    parsedContent,
		rawContent: content,
		hidden:     hidden,
		modTime:    modTime,
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

// RelPath 返回笔记的根相对路径，构造时即保证其位于根目录内
func (n *Note) RelPath() relpath.Path {
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

// ModTime 返回最后修改时间
func (n *Note) ModTime() time.Time {
	return n.modTime
}

// encodeID 根据笔记根相对路径生成笔记 ID
func encodeID(relPath relpath.Path) scalar.ID {
	return scalar.ToID(idPrefix + strings.TrimSuffix(relPath.String(), ".md"))
}

// decodeID 从笔记 ID 中提取笔记根相对路径。
// rootDir 仅用于还原时重新围栏：ID 由服务端签发，但仍是不可信输入
func decodeID(rootDir string, id scalar.ID) (relpath.Path, error) {
	idStr := id.String()
	if idStr == "" {
		return relpath.Path{}, apperror.New("INVALID_ID", "id must not be empty", "ID 不能为空")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return relpath.Path{}, apperror.New("INVALID_NOTE_ID", "invalid note ID format", "笔记 ID 格式无效")
	}

	rel, err := relpath.New(rootDir, strings.TrimPrefix(idStr, idPrefix)+".md")
	if err != nil {
		return relpath.Path{}, apperror.New("INVALID_NOTE_ID", "invalid note ID path", "笔记 ID 中的路径无效")
	}
	return rel, nil
}
