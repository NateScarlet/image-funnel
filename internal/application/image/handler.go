package image

import (
	"context"
	"fmt"
	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"strings"
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

// ComfyUIWorkflow 通过图片 ID 获取 ComfyUI 工作流
func (h *Handler) ComfyUIWorkflow(
	ctx context.Context,
	id scalar.ID,
) (*string, error) {
	h.logger.Debug("will get ComfyUI workflow", zap.Stringer("id", id))
	startTime := time.Now()

	defer func() {
		if err := recover(); err != nil {
			h.logger.Error("did get ComfyUI workflow (panic)",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Any("err", err),
			)
		}
	}()

	// 解析出图片的绝对路径和期望的修改时间
	absPath, expectedModTime, err := image.DecodeID(id)
	if err != nil {
		return nil, err
	}

	// 检查文件是否存在
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			h.logger.Debug("file not found",
				zap.Stringer("id", id),
				zap.String("path", absPath),
			)
			return nil, apperror.NewErrDocumentNotFound(id)
		}
		return nil, err
	}

	// 验证文件修改时间是否匹配 ID 中的时间戳
	// 如果时间不匹配，说明文件已被修改，返回版本冲突错误
	actualModTime := info.ModTime()
	if actualModTime.UnixNano() != expectedModTime.UnixNano() {
		h.logger.Debug("file modification time does not match",
			zap.Stringer("id", id),
			zap.String("path", absPath),
			zap.Time("expected_mod_time", expectedModTime),
			zap.Time("actual_mod_time", actualModTime),
		)
		return nil, apperror.New(
			"VERSION_CONFLICT",
			"image file has been modified on disk",
			"图片在磁盘上已被修改",
		)
	}

	// 验证文件类型，只处理 PNG 文件
	ext := strings.ToLower(filepath.Ext(info.Name()))
	if ext != ".png" {
		return nil, nil
	}

	// 提取工作流
	workflow, err := ExtractComfyUIWorkflow(absPath)
	if err != nil {
		h.logger.Error("did get ComfyUI workflow (error)",
			zap.Stringer("id", id),
			zap.String("path", absPath),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return nil, err
	}

	h.logger.Debug("did get ComfyUI workflow",
		zap.Stringer("id", id),
		zap.String("path", absPath),
		zap.Time("expected_mod_time", expectedModTime),
		zap.Time("actual_mod_time", actualModTime),
		zap.Duration("duration", time.Since(startTime)),
		zap.Bool("has_workflow", workflow != nil),
	)

	return workflow, nil
}
