package image

import (
	"context"
	"fmt"
	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

type Handler struct {
	xmpRepo      metadata.Repository
	imageFactory *image.Factory
	urlSigner    URLSigner
	logger       *zap.Logger
	rootDir      string
}

func NewHandler(
	xmpRepo metadata.Repository,
	imageFactory *image.Factory,
	urlSigner URLSigner,
	logger *zap.Logger,
	rootDir string,
) *Handler {
	return &Handler{
		xmpRepo:      xmpRepo,
		imageFactory: imageFactory,
		urlSigner:    urlSigner,
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
) (dto *shared.ImageDTO, err error) {
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

	// 解析出图片的绝对路径与编码时的修改时间
	absPath, expectedModTime, err := image.DecodeID(id)
	if err != nil {
		return nil, err
	}

	// 校验修改时间以防止对过时版本的图片进行修改
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if info.ModTime().UnixNano() != expectedModTime.UnixNano() {
		return nil, apperror.New(
			"VERSION_CONFLICT",
			"image file has been modified on disk",
			"图片在磁盘上已被修改，操作已拒绝",
		)
	}

	// 读取现有元数据，如果不存在则创建空白结构
	xmpData, err := h.xmpRepo.Read(absPath)
	if err != nil {
		return nil, err
	}
	if xmpData == nil {
		xmpData = metadata.NewXMPData(0, "", time.Time{}, "")
	}

	// 按需合并更新内容
	ratingVal := xmpData.Rating()
	if rating != nil {
		ratingVal = *rating
	}

	labelVal := xmpData.Label()
	if label != nil {
		labelVal = *label
	}

	newXMP := metadata.NewXMPData(ratingVal, xmpData.Action(), time.Now(), labelVal)
	err = h.xmpRepo.Write(absPath, newXMP)
	if err != nil {
		return nil, err
	}

	// 重新读取/重建 Image 对象以获取最新状态
	info, err = os.Stat(absPath)
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
		return nil, fmt.Errorf("failed to load image after update: %s", absPath)
	}

	dtoFactory := NewImageDTOFactory(h.urlSigner)
	return dtoFactory.New(img)
}
