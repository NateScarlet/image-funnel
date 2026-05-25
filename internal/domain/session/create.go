package session

import (
	"context"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
)

// Create 初始化一个新的会话
// 扫描目录、应用过滤器并创建会话，ID 由领域层自行生成
func (s *Service) Create(ctx context.Context, directoryID scalar.ID, filter *shared.ImageFilters, targetKeep int) (scalar.ID, error) {
	dir, err := s.directoryResolver.GetDirectory(ctx, directoryID)
	if err != nil {
		return scalar.ID{}, err
	}

	filterFunc := s.imageFilterBuilder.Build(util.UnwrapPointer(filter))
	var filteredImages []*image.Image
	for img, err := range s.imageScanner.Scan(ctx, dir.RelPath()) {
		if err != nil {
			return scalar.ID{}, err
		}
		if filterFunc(img) {
			filteredImages = append(filteredImages, img)
		}
	}

	id := scalar.NewID()
	sess := New(id, directoryID, filter, targetKeep, filteredImages, s.imageFilterBuilder)
	release, err := s.sessionRepo.Create(sess)
	if err != nil {
		return scalar.ID{}, err
	}
	defer release()

	s.sessionSaved.Publish(ctx, sess.ID())
	return id, nil
}
