package hook

import (
	"context"
	"fmt"
	"path"

	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/relpath"
)

// noteContext 笔记触发路径的公共上下文解析：从笔记路径推导配套图片，
// 从绝对路径解析目录实体。多条触发路径共用同一套推导规则，避免各自拼装。
type noteContext struct {
	rootDir string
	imgRepo image.Repository
	dirRepo directory.Repository
}

func newNoteContext(rootDir string, imgRepo image.Repository, dirRepo directory.Repository) *noteContext {
	return &noteContext{rootDir: rootDir, imgRepo: imgRepo, dirRepo: dirRepo}
}

// AssociatedImagePath 从笔记路径推导可能的图片基础路径（去掉扩展名）。
// 判定与裁剪同源于 TrimExtSuffix，避免大小写不一致的后缀漏裁
func (c *noteContext) AssociatedImagePath(noteRelPath relpath.Path) (relpath.Path, bool, error) {
	ext := path.Ext(noteRelPath.String())
	if ext == "" {
		return relpath.Path{}, false, nil
	}
	imgRelPath, _, err := noteRelPath.TrimExtSuffix(c.rootDir, ext)
	if err != nil {
		return relpath.Path{}, false, err
	}
	return imgRelPath, true, nil
}

// Image 按根相对路径查询图片实体；图片不存在时返回 (nil, nil)——笔记允许没有配套图片
func (c *noteContext) Image(ctx context.Context, imgRelPath relpath.Path) (*image.Image, error) {
	img, err := c.imgRepo.Get(ctx, imgRelPath)
	if err != nil {
		return apperror.IgnoreNotFound(img, err)
	}
	return img, nil
}

// AssociatedImageEvents 查找笔记配套的图片，构建对应的 hookEvent 列表
func (c *noteContext) AssociatedImageEvents(ctx context.Context, noteRelPath relpath.Path) ([]hookEvent, error) {
	imgRelPath, ok, err := c.AssociatedImagePath(noteRelPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	img, err := c.Image(ctx, imgRelPath)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}
	return []hookEvent{{
		ID:   img.ID().String(),
		Path: img.RelPath().Abs(c.rootDir),
	}}, nil
}

// DirOf 从绝对文件路径解析其所在目录的实体
func (c *noteContext) DirOf(ctx context.Context, absPath string) (*directory.Directory, error) {
	rel, err := relFromAbsPath(c.rootDir, absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory for %q: %w", absPath, err)
	}
	return c.DirOfRelPath(ctx, rel.Dir())
}

// DirOfRelPath 从目录的根相对路径解析目录实体
func (c *noteContext) DirOfRelPath(ctx context.Context, dirRelPath relpath.Path) (*directory.Directory, error) {
	dir, err := c.dirRepo.Get(ctx, dirRelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory for %q: %w", dirRelPath, err)
	}
	return dir, nil
}
