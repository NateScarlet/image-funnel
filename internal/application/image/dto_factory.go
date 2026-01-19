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
	url, _ := f.urlSigner.GenerateSignedURL(img.Path())
	return &shared.ImageDTO{
		ID:            img.ID(),
		Filename:      img.Filename(),
		Size:          img.Size(),
		URL:           url,
		ModTime:       img.ModTime(),
		CurrentRating: img.Rating(),
		XMPExists:     img.XMPExists(),
	}, nil
}
