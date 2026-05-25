package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
)

// Create 初始化一个新的会话
// 扫描目录、应用过滤器并创建会话
func (s *Service) Create(ctx context.Context, id scalar.ID, directoryID scalar.ID, filter *shared.ImageFilters, targetKeep int) error {
	directory, err := directory.DecodeID(directoryID)
	if err != nil {
		return err
	}

	filterFunc := s.imageFilterBuilder.Build(util.UnwrapPointer(filter))
	var filteredImages []*image.Image
	for img, err := range s.imageScanner.Scan(ctx, directory) {
		if err != nil {
			return err
		}
		if filterFunc(img) {
			filteredImages = append(filteredImages, img)
		}
	}

	sess := New(id, directoryID, filter, targetKeep, filteredImages, s.imageFilterBuilder)
	release, err := s.sessionRepo.Create(sess)
	if err != nil {
		return err
	}
	defer release()

	s.sessionSaved.Publish(ctx, sess.ID())
	return nil
}
