package image

import (
	"path/filepath"

	"main/internal/domain/hook"
	"main/internal/domain/image"
	"main/internal/shared"
)

type DTOFactory struct {
	urlSigner URLSigner
	rootDir   string
}

func NewDTOFactory(urlSigner URLSigner, rootDir string) *DTOFactory {
	return &DTOFactory{
		urlSigner: urlSigner,
		rootDir:   rootDir,
	}
}

func (f *DTOFactory) New(img *image.Image) (*shared.ImageDTO, error) {
	absPath := filepath.Join(f.rootDir, img.RelPath())

	return &shared.ImageDTO{
		ID:            img.ID(),
		Filename:      img.Filename(),
		Size:          img.Size(),
		AbsPath:       absPath,
		RelPath:       img.RelPath(),
		ModTime:       img.ModTime(),
		CurrentRating: img.Rating(),
		Width:         img.Width(),
		Height:        img.Height(),
		XMPExists:     img.XMPExists(),
		Label:         img.Label(),
	}, nil
}

// NewCopyContent 将复制增强内容转换为 DTO，脚本未提供通知文案时保持 nil
func (f *DTOFactory) NewCopyContent(content *hook.CopyContent) *shared.CopyContentDTO {
	var description *string
	if content.Description != "" {
		description = &content.Description
	}
	return &shared.CopyContentDTO{
		Content:     content.Content,
		Description: description,
	}
}
