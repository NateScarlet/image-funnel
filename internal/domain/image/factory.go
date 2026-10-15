package image

import (
	"context"
	"main/internal/domain/metadata"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"strings"
)

type Processor interface {
	Meta(ctx context.Context, absPath string) (*shared.ImageMeta, error)
}

type Factory struct {
	xmpRepo   metadata.Repository
	processor Processor
	rootDir   string
}

func NewFactory(xmpRepo metadata.Repository, processor Processor, rootDir string) *Factory {
	return &Factory{
		xmpRepo:   xmpRepo,
		processor: processor,
		rootDir:   rootDir,
	}
}

func (f *Factory) Create(ctx context.Context, relPath relpath.Path, directoryID scalar.ID) (*Image, error) {
	info, err := os.Stat(relPath.Abs(f.rootDir))
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, nil
	}

	if !f.IsSupportedImage(info.Name()) {
		return nil, nil
	}

	return f.CreateFromInfo(ctx, info, relPath, directoryID)
}

// CreateFromInfo creates an image from os.FileInfo, avoiding re-stat if caller has it.
func (f *Factory) CreateFromInfo(ctx context.Context, info os.FileInfo, relPath relpath.Path, directoryID scalar.ID) (*Image, error) {
	if info.IsDir() || !f.IsSupportedImage(info.Name()) {
		return nil, nil
	}

	absPath := relPath.Abs(f.rootDir)

	var xmpData *metadata.Data
	xmpData, err := f.xmpRepo.Read(absPath)
	if err != nil {
		return nil, err
	}

	width, height := 0, 0
	if f.processor != nil {
		meta, err := f.processor.Meta(ctx, absPath)
		if err == nil {
			width, height = meta.Width, meta.Height
		}
	}

	return FromRelPath(
		info.Name(),
		relPath,
		directoryID,
		info.Size(),
		info.ModTime(),
		xmpData,
		width,
		height,
	), nil
}

func (f *Factory) IsSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}
