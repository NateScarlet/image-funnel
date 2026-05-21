package image

import (
	"context"
	"fmt"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

type Handler struct {
	imageService *image.Service
	imageFactory *image.Factory
	dtoFactory   *ImageDTOFactory
	logger       *zap.Logger
	rootDir      string
}

func NewHandler(
	imageService *image.Service,
	imageFactory *image.Factory,
	dtoFactory *ImageDTOFactory,
	logger *zap.Logger,
	rootDir string,
) *Handler {
	return &Handler{
		imageService: imageService,
		imageFactory: imageFactory,
		dtoFactory:   dtoFactory,
		logger:       logger,
		rootDir:      rootDir,
	}
}

// UpdateImageMetadata 更新单个图片的元数据（评星和颜色标签），操作即时写入 XMP 伴随文件
func (h *Handler) UpdateImageMetadata(
	ctx context.Context,
	id scalar.ID,
	rating *int,
	label *string,
) (err error) {
	h.logger.Info("will update image metadata", zap.Stringer("id", id))
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("did update image metadata",
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
	// 解析出图片的绝对路径
	absPath, _, err := image.DecodeID(id)
	if err != nil {
		return nil, err
	}

	// 读取文件信息
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	// 计算目录ID
	relPath, err := filepath.Rel(h.rootDir, absPath)
	if err != nil {
		return nil, err
	}
	dirRelPath := filepath.Dir(relPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}
	directoryID := directory.EncodeID(dirRelPath)

	img, err := h.imageFactory.CreateFromInfo(ctx, info, absPath, directoryID)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("failed to load image: %s", absPath)
	}

	return h.dtoFactory.New(img)
}
