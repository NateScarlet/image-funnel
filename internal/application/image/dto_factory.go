package image

import (
	"path/filepath"

	"main/internal/domain/image"
	"main/internal/shared"
)

type ImageDTOFactory struct {
	urlSigner URLSigner
	rootDir   string
}

func NewImageDTOFactory(urlSigner URLSigner, rootDir string) *ImageDTOFactory {
	return &ImageDTOFactory{
		urlSigner: urlSigner,
		rootDir:   rootDir,
	}
}

func (f *ImageDTOFactory) New(img *image.Image) (*shared.ImageDTO, error) {
	relPath, err := filepath.Rel(f.rootDir, img.AbsPath())
	if err != nil {
		relPath = img.Filename()
	}

	return &shared.ImageDTO{
		ID:            img.ID(),
		Filename:      img.Filename(),
		Size:          img.Size(),
		AbsPath:       img.AbsPath(),
		RelPath:       relPath,
		ModTime:       img.ModTime(),
		CurrentRating: img.Rating(),
		Width:         img.Width(),
		Height:        img.Height(),
		XMPExists:     img.XMPExists(),
		Label:         img.Label(),
	}, nil
}
