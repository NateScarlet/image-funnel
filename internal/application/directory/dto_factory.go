package directory

import (
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/shared"
)

type DirectoryDTOFactory struct{}

func NewDirectoryDTOFactory() *DirectoryDTOFactory {
	return &DirectoryDTOFactory{}
}

func (f *DirectoryDTOFactory) New(dirInfo *directory.DirectoryInfo, parentID scalar.ID, isRoot bool) (*shared.DirectoryDTO, error) {
	var latestImageDTO *shared.ImageDTO
	if latestImage := dirInfo.LatestImage(); latestImage != nil {
		latestImageDTO = &shared.ImageDTO{
			ID:            latestImage.ID(),
			Filename:      latestImage.Filename(),
			Size:          latestImage.Size(),
			Path:          latestImage.Path(),
			ModTime:       latestImage.ModTime(),
			CurrentRating: latestImage.Rating(),
			Width:         latestImage.Width(),
			Height:        latestImage.Height(),
			XMPExists:     latestImage.XMPExists(),
		}
	}

	return &shared.DirectoryDTO{
		ID:                dirInfo.ID(),
		ParentID:          parentID,
		Path:              dirInfo.Path(),
		Root:              isRoot,
		ImageCount:        dirInfo.ImageCount(),
		SubdirectoryCount: dirInfo.SubdirectoryCount(),
		LatestImage:       latestImageDTO,
		RatingCounts:      dirInfo.RatingCounts(),
	}, nil
}
