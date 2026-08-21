package image

import (
	"context"

	"main/internal/scalar"
	"main/internal/shared"
)

// CopyContent 通过复制增强钩子获取应写入剪贴板的内容：
// GetImage → 注入的 hook.Runner 端口（执行 [copy] 脚本）→ DTOFactory 转 DTO。
// 未配置 [copy] 钩子或脚本对当前图片不适用时返回 nil，由接口层/前端降级为复制文件。
func (h *Handler) CopyContent(
	ctx context.Context,
	id scalar.ID,
) (*shared.CopyContentDTO, error) {
	img, err := h.imageService.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}

	content, err := h.hookRunner.CopyContent(ctx, id, img.RelPath())
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, nil
	}

	return h.dtoFactory.NewCopyContent(content), nil
}
