package directory

import (
	"main/internal/domain/directory"
)

type DirectoryDTOFactory struct{}

func NewDirectoryDTOFactory() *DirectoryDTOFactory {
	return &DirectoryDTOFactory{}
}

func (f *DirectoryDTOFactory) New(dirInfo *directory.DirectoryInfo, parentID string, isRoot bool) (*DirectoryDTO, error) {
	return &DirectoryDTO{
		ID:                 dirInfo.ID(),
		ParentID:           parentID,
		Path:               dirInfo.Path(),
		Root:               isRoot,
		ImageCount:         dirInfo.ImageCount(),
		SubdirectoryCount:  dirInfo.SubdirectoryCount(),
		LatestImageModTime: dirInfo.LatestImageModTime(),
		LatestImagePath:    dirInfo.LatestImagePath(),
		RatingCounts:       dirInfo.RatingCounts(),
	}, nil
}
