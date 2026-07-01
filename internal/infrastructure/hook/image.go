package hook

import (
	"context"
	"path/filepath"
	"strings"

	"main/internal/apperror"
)

// associatedImageRelPath 从笔记路径推导可能的图片基础路径（去掉 .md 扩展名）
func associatedImageRelPath(noteRelPath string) (string, bool) {
	ext := filepath.Ext(noteRelPath)
	if ext == "" {
		return "", false
	}
	return strings.TrimSuffix(noteRelPath, ext), true
}

// findAssociatedImageEvents 查找笔记配套的图片，构建对应的 hookEvent 列表
func (r *Runner) findAssociatedImageEvents(ctx context.Context, noteRelPath string) ([]hookEvent, error) {
	imgRelPath, ok := associatedImageRelPath(noteRelPath)
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
		Path: filepath.Join(r.rootDir, img.RelPath()),
	}}, nil
}