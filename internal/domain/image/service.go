package image

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/util"
	"path/filepath"
	"time"
)

// Service 图片领域服务
type Service struct {
	xmpRepo   metadata.Repository
	imageRepo Repository
	rootDir   string
}

// NewService 创建图片领域服务
func NewService(xmpRepo metadata.Repository, imageRepo Repository, rootDir string) *Service {
	return &Service{
		xmpRepo:   xmpRepo,
		imageRepo: imageRepo,
		rootDir:   rootDir,
	}
}

// GetImage 根据图片 ID 获取图片实体，由领域层内部解码 ID 并校验版本一致性
func (s *Service) GetImage(ctx context.Context, id scalar.ID) (*Image, error) {
	relPath, expectedModTime, err := decodeID(id)
	if err != nil {
		return nil, err
	}
	img, err := s.imageRepo.Get(ctx, relPath)
	if err != nil {
		return nil, err
	}
	if img.ModTime().UnixNano() != expectedModTime.UnixNano() {
		return nil, apperror.New(
			"VERSION_CONFLICT",
			"image file has been modified on disk",
			"图片在磁盘上已被修改，操作已拒绝",
		)
	}
	return img, nil
}

// UpdateImageMetadata 更新单个图片的元数据（评星和颜色标签），操作即时写入 XMP 伴随文件
func (s *Service) UpdateImageMetadata(
	ctx context.Context,
	id scalar.ID,
	rating *int,
	label *string,
) (err error) {
	img, err := s.GetImage(ctx, id)
	if err != nil {
		return err
	}

	relPath := img.RelPath()
	absPath := filepath.Join(s.rootDir, relPath)

	if err := util.EnsurePathInRoot(s.rootDir, relPath); err != nil {
		return err
	}

	// 读取现有元数据，如果不存在则创建空白结构
	xmpData, err := s.xmpRepo.Read(absPath)
	if err != nil {
		return err
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
	return s.xmpRepo.Write(absPath, newXMP)
}
