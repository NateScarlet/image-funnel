package directory

import (
	appimage "main/internal/application/image"
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/shared"
)

type DirectoryDTOFactory struct {
	imageDTOFactory *appimage.ImageDTOFactory
}

func NewDirectoryDTOFactory(imageDTOFactory *appimage.ImageDTOFactory) *DirectoryDTOFactory {
	return &DirectoryDTOFactory{
		imageDTOFactory: imageDTOFactory,
	}
}

func (f *DirectoryDTOFactory) New(dirInfo *directory.Directory, parentID scalar.ID, isRoot bool) *shared.DirectoryDTO {
	return &shared.DirectoryDTO{
		ID:       dirInfo.ID(),
		ParentID: parentID,
		Path:     dirInfo.Path(),
		Root:     isRoot,
	}
}

func (f *DirectoryDTOFactory) NewDirectoryStatsDTO(stats *directory.DirectoryStats) (*shared.DirectoryStatsDTO, error) {
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
