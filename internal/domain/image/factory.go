package image

import (
	"context"
	"main/internal/domain/metadata"
	"main/internal/shared"
	"main/internal/util"
	"os"
	"path/filepath"
	"strings"
)

type Processor interface {
	Meta(ctx context.Context, path string) (*shared.ImageMeta, error)
}

type Factory struct {
	xmpRepo   metadata.Repository
	processor Processor
}

func NewFactory(xmpRepo metadata.Repository, processor Processor) *Factory {
	return &Factory{
		xmpRepo:   xmpRepo,
		processor: processor,
	}
}

func (f *Factory) Create(ctx context.Context, relPath string, rootDir string) (*Image, error) {
	if err := util.EnsurePathInRoot(rootDir, relPath); err != nil {
		return nil, err
	}
	absPath := filepath.Join(rootDir, relPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		// Not a file, return nil/error?
		// Scanner filtered directories, here we assume caller might pass one?
		// Or we return error "is directory".
		// But ScanFile returns nil for unsupported.
		return nil, nil
	}

	if !f.isSupportedImage(info.Name()) {
		return nil, nil
	}

	return f.CreateFromInfo(ctx, info, absPath)
}

// CreateFromInfo creates an image from os.FileInfo, avoiding re-stat if caller has it.
// absPath is required.
func (f *Factory) CreateFromInfo(ctx context.Context, info os.FileInfo, absPath string) (*Image, error) {
	if info.IsDir() || !f.isSupportedImage(info.Name()) {
		return nil, nil
	}

	var xmpData *metadata.XMPData
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

	return NewImageFromPath(
		info.Name(),
		absPath,
		info.Size(),
		info.ModTime(),
		xmpData,
		width,
		height,
	), nil
}

func (f *Factory) isSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}
