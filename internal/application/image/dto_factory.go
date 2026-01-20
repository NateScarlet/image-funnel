package image

import (
	"main/internal/domain/image"
	"main/internal/shared"
)

type ImageDTOFactory struct {
	urlSigner URLSigner
}

func NewImageDTOFactory(urlSigner URLSigner) *ImageDTOFactory {
	return &ImageDTOFactory{
		urlSigner: urlSigner,
	}
}

func (f *ImageDTOFactory) New(img *image.Image) (*shared.ImageDTO, error) {
	return &shared.ImageDTO{
		ID:            img.ID(),
		Filename:      img.Filename(),
		Size:          img.Size(),
		Path:          img.Path(),
		ModTime:       img.ModTime(),
		CurrentRating: img.Rating(),
		Width:         img.Width(),
		Height:        img.Height(),
		XMPExists:     img.XMPExists(),
	}, nil
}
