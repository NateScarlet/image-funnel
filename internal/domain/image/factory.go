package image

import (
	"context"
	"fmt"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
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

func (f *Factory) Create(ctx context.Context, relPath string, directoryID scalar.ID) (*Image, error) {
	if err := util.EnsurePathInRoot(f.rootDir, relPath); err != nil {
		return nil, err
	}
	absPath := filepath.Join(f.rootDir, relPath)
	info, err := os.Stat(absPath)
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
func (f *Factory) CreateFromInfo(ctx context.Context, info os.FileInfo, relPath string, directoryID scalar.ID) (*Image, error) {
	// 校验以确保路径不是绝对路径，防御性拒绝绝对路径的入参
	if filepath.IsAbs(relPath) {
		return nil, fmt.Errorf("absolute path not allowed: %s", relPath)
	}

	if info.IsDir() || !f.IsSupportedImage(info.Name()) {
		return nil, nil
	}

	absPath := filepath.Join(f.rootDir, relPath)

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
