package localfs

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"main/internal/domain/directory"
	"main/internal/scalar"
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

	// 先尝试解析为最新版本（主要场景，性能最优）
	var state shared.DirectoryStateDTO
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// 如果是旧版本，根据版本号进行迁移
	if state.Version < 2 {
		return d.migrateToV2(data), nil
	}

	return &state, nil
}

// migrateToV2 根据版本号将旧版本的 state 迁移到 v2
func (d *DirectoryRepository) migrateToV2(data []byte) *shared.DirectoryStateDTO {
	// 尝试解析为 v1 版本
	var stateV1 DirectoryStateDTOV1
	if err := json.Unmarshal(data, &stateV1); err != nil || stateV1.Version != 1 {
		// 解析失败或不是 v1，返回空的 v2 state
		return &shared.DirectoryStateDTO{Version: 2}
	}

	browseV2 := &shared.DirectoryStateBrowseDTO{}
	if stateV1.Browse != nil {
		// 直接复制，因为 v1 和 v2 的数据结构相同，只需重命名字段
		browseV2.FilterBy = stateV1.Browse.FilterBy
		browseV2.FilterNoteBy = stateV1.Browse.FilterMemoBy // filterMemoBy -> filterNoteBy
	}

	// 转换 LastSession
	var lastSession *shared.DirectoryStateLastSessionDTO
	if stateV1.LastSession != nil {
		lastSession = &shared.DirectoryStateLastSessionDTO{
			ID:         scalar.ToID(stateV1.LastSession.ID),
			TargetKeep: stateV1.LastSession.TargetKeep,
		}
		if stateV1.LastSession.Filter != nil {
			filterJSON, _ := json.Marshal(stateV1.LastSession.Filter)
			json.Unmarshal(filterJSON, &lastSession.Filter)
		}
	}

	return &shared.DirectoryStateDTO{
		Version:     2,
		Browse:      browseV2,
		LastSession: lastSession,
		UpdatedAt:   stateV1.UpdatedAt,
	}
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

	state.Version = 2
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(absPath, data, 0644)
}

var _ directory.Repository = (*DirectoryRepository)(nil)

