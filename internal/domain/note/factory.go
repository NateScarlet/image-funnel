package note

import (
	"errors"
	"main/internal/domain/relpath"
	"strings"
	"time"
)

// Factory 笔记实体工厂
type Factory struct {
	rootDir string
}

// NewFactory 创建笔记实体工厂，注入物理根路径
func NewFactory(rootDir string) *Factory {
	return &Factory{
		rootDir: rootDir,
	}
}

// New 创建一个新的 Note 实体，自理 ID 生成和绝对路径计算，并对输入参数进行校验。
// relPath: 笔记相对于根目录的路径，不能为空，且必须以 .md 结尾。
//
// 「不能为空」是笔记自身的约束而非路径的约束——根目录本身是合法的根相对路径，
// 只是不能作为笔记文件的位置。
// content: 包含 frontmatter 的完整笔记文本内容。
func (f *Factory) New(
	relPath relpath.Path,
	content string,
) (*Note, error) {
	if relPath.IsRoot() {
		return nil, errors.New("relPath is required")
	}
	if !strings.HasSuffix(relPath.String(), ".md") {
		return nil, errors.New("relPath must end with .md")
	}

	// 解析 frontmatter，得到是否隐藏和纯文本正文
	hidden, parsedContent := ParseContent(content)

	return &Note{
		id:         encodeID(relPath),
		relPath:    relPath,
		absPath:    relPath.Abs(f.rootDir),
		content:    parsedContent,
		rawContent: content,
		hidden:     hidden,
		modTime:    time.Now(),
	}, nil
}
