package directory

import (
	appimage "main/internal/application/image"
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/shared"
)

type DTOFactory struct {
	imageDTOFactory *appimage.DTOFactory
}

func NewDTOFactory(imageDTOFactory *appimage.DTOFactory) *DTOFactory {
	return &DTOFactory{
		imageDTOFactory: imageDTOFactory,
	}
}

func (f *DTOFactory) New(dirInfo *directory.Directory, parentID scalar.ID, isRoot bool) *shared.DirectoryDTO {
	return &shared.DirectoryDTO{
		ID:       dirInfo.ID(),
		ParentID: parentID,
		RelPath:  dirInfo.RelPath(),
		Root:     isRoot,
	}
}

func (f *DTOFactory) NewDirectoryStatsDTO(stats *directory.Stats) (*shared.DirectoryStatsDTO, error) {
	if stats == nil {
		return nil, nil
	}

	var latestImageDTO *shared.ImageDTO
	if latestImage := stats.LatestImage(); latestImage != nil {
		var err error
		latestImageDTO, err = f.imageDTOFactory.New(latestImage)
		if err != nil {
			return nil, err
		}
	}

	return &shared.DirectoryStatsDTO{
		ImageCount:        stats.ImageCount(),
		SubdirectoryCount: stats.SubdirectoryCount(),
		LatestImage:       latestImageDTO,
		RatingCounts:      stats.RatingCounts(),
	}, nil
}
