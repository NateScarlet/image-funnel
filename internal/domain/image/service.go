package image

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/util"
	"os"
	"path/filepath"
	"time"
)

// Service 图片领域服务
type Service struct {
	xmpRepo metadata.Repository
	rootDir string
}

// NewService 创建图片领域服务
func NewService(xmpRepo metadata.Repository, rootDir string) *Service {
	return &Service{
		xmpRepo: xmpRepo,
		rootDir: rootDir,
	}
}

// UpdateImageMetadata 更新单个图片的元数据（评星和颜色标签），操作即时写入 XMP 伴随文件
func (s *Service) UpdateImageMetadata(
	ctx context.Context,
	id scalar.ID,
	rating *int,
	label *string,
) (err error) {
	// 解析出图片的绝对路径与编码时的修改时间
	absPath, expectedModTime, err := DecodeID(id)
	if err != nil {
		return err
	}

	// 确保路径在 rootDir 下，防止修改非当前应用管理的文件
	relPath, err := filepath.Rel(s.rootDir, absPath)
	if err != nil {
		return err
	}
	if err := util.EnsurePathInRoot(s.rootDir, relPath); err != nil {
		return err
	}

	// 校验修改时间以防止对过时版本的图片进行修改
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if info.ModTime().UnixNano() != expectedModTime.UnixNano() {
		return apperror.New(
			"VERSION_CONFLICT",
			"image file has been modified on disk",
			"图片在磁盘上已被修改，操作已拒绝",
		)
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
