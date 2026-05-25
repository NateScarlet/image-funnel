package image

import (
	"context"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

type Handler struct {
	imageService *image.Service
	dtoFactory   *DTOFactory
	logger       *zap.Logger
}

func NewHandler(
	imageService *image.Service,
	dtoFactory *DTOFactory,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		imageService: imageService,
		dtoFactory:   dtoFactory,
		logger:       logger,
	}
}

// UpdateImageMetadata 更新单个图片的元数据（评星和颜色标签），操作即时写入 XMP 伴随文件
func (h *Handler) UpdateImageMetadata(
	ctx context.Context,
	id scalar.ID,
	rating *int,
	label *string,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update image metadata failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did update image metadata",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.imageService.UpdateImageMetadata(ctx, id, rating, label)
}

// Image 通过 ID 获取图片
func (h *Handler) Image(
	ctx context.Context,
	id scalar.ID,
) (*shared.ImageDTO, error) {
	img, err := h.imageService.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}
	return h.dtoFactory.New(img)
}

// ComfyUIWorkflow 通过图片 ID 获取 ComfyUI 工作流
func (h *Handler) ComfyUIWorkflow(
	ctx context.Context,
	id scalar.ID,
) (_ *string, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("get ComfyUI workflow failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Any("err", err),
			)
		} else {
			h.logger.Debug("did get ComfyUI workflow",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	img, err := h.imageService.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(img.Filename()))
	if ext != ".png" {
		return nil, nil
	}

	workflow, err := ExtractComfyUIWorkflow(img.AbsPath())
	if err != nil {
		return nil, err
	}

	return workflow, nil
}
