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

	// 先尝试解析为最新版本（主要场景，性能最优，仅进行一次解析）
	var state shared.DirectoryStateDTO
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	switch state.Version {
	case 1:
		return d.readStateV1(data)
	case 2:
		return d.readStateV2(data)
	case 3:
		return &state, nil
	default:
		return nil, fmt.Errorf("got unexpected state version %d", state.Version)
	}

}

func (d *DirectoryRepository) readStateV1(data []byte) (*shared.DirectoryStateDTO, error) {
	var stateV1 DirectoryStateDTOV1
	if err := json.Unmarshal(data, &stateV1); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state v1: %w", err)
	}
	if stateV1.Version != 1 {
		return nil, fmt.Errorf("expected version 1, got %d", stateV1.Version)
	}

	browseV3 := &shared.DirectoryStateBrowseDTO{}
	if stateV1.Browse != nil {
		browseV3.FilterBy = stateV1.Browse.FilterBy
		browseV3.FilterNoteBy = stateV1.Browse.FilterMemoBy // filterMemoBy -> filterNoteBy
	}

	var lastSessionV3 *shared.DirectoryStateLastSessionDTO
	if stateV1.LastSession != nil {
		lastSessionV3 = &shared.DirectoryStateLastSessionDTO{
			ID:         scalar.ToID(stateV1.LastSession.ID),
			TargetKeep: stateV1.LastSession.TargetKeep,
		}
		if stateV1.LastSession.Filter != nil {
			filterJSON, _ := json.Marshal(stateV1.LastSession.Filter)
			if err := json.Unmarshal(filterJSON, &lastSessionV3.Filter); err != nil {
				return nil, fmt.Errorf("failed to unmarshal filter from state v1: %w", err)
			}
		}
	}

	return &shared.DirectoryStateDTO{
		Version:     3,
		Browse:      browseV3,
		LastSession: lastSessionV3,
		UpdatedAt:   stateV1.UpdatedAt,
	}, nil
}

func (d *DirectoryRepository) readStateV2(data []byte) (*shared.DirectoryStateDTO, error) {
	var stateV2 DirectoryStateDTOV2
	if err := json.Unmarshal(data, &stateV2); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state v2: %w", err)
	}
	if stateV2.Version != 2 {
		return nil, fmt.Errorf("expected version 2, got %d", stateV2.Version)
	}

	// 提取老版本中的 actions 并整合到全新的前端管理 Default 顶级配置中
	var defaultV3 *shared.DirectoryStateDefaultDTO
	if stateV2.LastSession != nil {
		var actionsToUse *shared.WriteActions
		if stateV2.LastSession.CommitActions != nil {
			actionsToUse = stateV2.LastSession.CommitActions
		} else if stateV2.LastSession.CreateActions != nil {
			actionsToUse = stateV2.LastSession.CreateActions
		}

		if actionsToUse != nil {
			defaultV3 = &shared.DirectoryStateDefaultDTO{
				WriteActions: actionsToUse,
			}
		}
	}

	// 转换 lastSession
	var lastSessionV3 *shared.DirectoryStateLastSessionDTO
	if stateV2.LastSession != nil {
		var idStr string
		if stateV2.LastSession.ID != nil {
			idStr, _ = stateV2.LastSession.ID.(string)
		}
		lastSessionV3 = &shared.DirectoryStateLastSessionDTO{
			ID:         scalar.ToID(idStr),
			Filter:     stateV2.LastSession.Filter,
			TargetKeep: stateV2.LastSession.TargetKeep,
		}
	}

	return &shared.DirectoryStateDTO{
		Version:     3,
		Browse:      stateV2.Browse,
		LastSession: lastSessionV3,
		Default:     defaultV3,
		UpdatedAt:   stateV2.UpdatedAt,
	}, nil
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

	state.Version = 3
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(absPath, data, 0644)
}

var _ directory.Repository = (*DirectoryRepository)(nil)
