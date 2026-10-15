package hook

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/relpath"
	"path"
)

// associatedImagePath 从笔记路径推导可能的图片基础路径（去掉扩展名）。
// 判定与裁剪同源于 TrimExtSuffix，避免大小写不一致的后缀漏裁
func (r *Runner) associatedImagePath(noteRelPath relpath.Path) (relpath.Path, bool, error) {
	ext := path.Ext(noteRelPath.String())
	if ext == "" {
		return relpath.Path{}, false, nil
	}
	imgRelPath, _, err := noteRelPath.TrimExtSuffix(r.rootDir, ext)
	if err != nil {
		return relpath.Path{}, false, err
	}
	return imgRelPath, true, nil
}

// findAssociatedImageEvents 查找笔记配套的图片，构建对应的 hookEvent 列表
func (r *Runner) findAssociatedImageEvents(ctx context.Context, noteRelPath relpath.Path) ([]hookEvent, error) {
	imgRelPath, ok, err := r.associatedImagePath(noteRelPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	img, err := r.imgRepo.Get(ctx, imgRelPath)
	if err != nil {
		img, err = apperror.IgnoreNotFound(img, err)
		if err != nil {
			return nil, err
		}
	}
	if img == nil {
		return nil, nil
	}
	return []hookEvent{{
		ID:   img.ID().String(),
		Path: img.RelPath().Abs(r.rootDir),
	}}, nil
}
