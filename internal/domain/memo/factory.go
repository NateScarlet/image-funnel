package memo

import (
	"errors"
	"main/internal/util"
	"path/filepath"
	"strings"
)

// Factory 备忘录实体工厂
type Factory struct {
	rootDir string
}

// NewFactory 创建备忘录实体工厂，注入物理根路径
func NewFactory(rootDir string) *Factory {
	return &Factory{
		rootDir: rootDir,
	}
}

// New 创建一个新的 Memo 实体，自理 ID 生成和绝对路径计算，并对输入参数进行校验。
// relPath: 备忘录相对于根目录的路径，不能为空，且不能是绝对路径，且必须以 .md 结尾。
// content: 包含 frontmatter 的完整备忘录文本内容。
func (f *Factory) New(
	relPath string,
	content string,
) (*Memo, error) {
	if relPath == "" {
		return nil, errors.New("relPath is required")
	}
	if err := util.EnsurePathInRoot(f.rootDir, relPath); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(relPath, ".md") {
		return nil, errors.New("relPath must end with .md")
	}

	absPath := filepath.Join(f.rootDir, relPath)
	id := encodeID(relPath)

	// 解析 frontmatter，得到是否隐藏和纯文本正文
	hidden, parsedContent := ParseContent(content)

	return &Memo{
		id:         id,
		relPath:    relPath,
		absPath:    absPath,
		content:    parsedContent,
		rawContent: content,
		hidden:     hidden,
	}, nil
}
