package inmem

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"main/internal/domain/directory"
	"main/internal/shared"
	"main/internal/util"
	"os"
	"path/filepath"
	"strings"
)

func NewDirectoryRepository(rootDir string) *DirectoryRepository {
	return &DirectoryRepository{
		rootDir: rootDir,
	}
}

type DirectoryRepository struct {
	rootDir string
}

// Get implements [directory.Repository].
func (d *DirectoryRepository) Get(ctx context.Context, relPath string) (*directory.Directory, error) {
	err := util.EnsurePathInRoot(d.rootDir, relPath)
	if err != nil {
		return nil, err
	}
	return directory.FromRepository(relPath), nil
}

func (d *DirectoryRepository) Find(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {
		if relPath != "" {
			if err := util.EnsurePathInRoot(d.rootDir, relPath); err != nil {
				yield(nil, err)
				return
			}
		}

		absPath := filepath.Join(d.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		for _, entry := range entries {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			subRelPath := filepath.Join(relPath, entry.Name())
			dirInfo, err := d.Get(ctx, subRelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(dirInfo, nil) {
				break
			}
		}
	}
}

// ReadState implements [directory.Repository].
func (d *DirectoryRepository) ReadState(ctx context.Context, relPath string) (*shared.DirectoryStateDTO, error) {
	err := util.EnsurePathInRoot(d.rootDir, relPath)
	if err != nil {
		return nil, err
	}
	absPath := filepath.Join(d.rootDir, relPath, ".io.github.natescarlet.image-funnel.state.json")
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state shared.DirectoryStateDTO
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	return &state, nil
}

// WriteState implements [directory.Repository].
func (d *DirectoryRepository) WriteState(ctx context.Context, relPath string, state *shared.DirectoryStateDTO) error {
	err := util.EnsurePathInRoot(d.rootDir, relPath)
	if err != nil {
		return err
	}
	absPath := filepath.Join(d.rootDir, relPath, ".io.github.natescarlet.image-funnel.state.json")

	if state == nil {
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	state.Version = 1
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(absPath, data, 0644)
}

var _ directory.Repository = (*DirectoryRepository)(nil)

