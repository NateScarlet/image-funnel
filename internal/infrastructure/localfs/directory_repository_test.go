package localfs

import (
	"context"
	"encoding/json"
	"main/internal/domain/relpath"
	"main/internal/shared"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"main/internal/scalar"
)

func TestDirectoryRepository_State(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	repo := NewDirectoryRepository(tmpDir)

	// 1. 读取不存在的状态，应返回 nil, nil
	state, err := repo.ReadState(ctx, relpath.Path{})
	if err != nil {
		t.Fatalf("ReadState empty failed: %v", err)
	}
	if state != nil {
		t.Fatalf("expected state to be nil, got: %v", state)
	}

	// 2. 写入状态
	originalState := &shared.DirectoryStateDTO{
		Version: shared.CurrentDirectoryStateVersion,
		Browse: &shared.DirectoryStateBrowseDTO{
			FilterBy: &shared.ImageFilters{
				Rating: []int{3, 4},
				Label:  []string{"red"},
				Query:  "test",
			},
		},
	}
	err = repo.WriteState(ctx, relpath.Path{}, originalState)
	if err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	// 3. 验证是否生成了文件
	filePath := filepath.Join(tmpDir, ".io.github.natescarlet.image-funnel.state.json")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("state file was not created: %v", err)
	}

	// 4. 读取状态并比对
	loadedState, err := repo.ReadState(ctx, relpath.Path{})
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loadedState == nil {
		t.Fatal("expected state to be loaded, got nil")
	}
	if loadedState.Version != shared.CurrentDirectoryStateVersion {
		t.Errorf("expected Version to be %d, got %d", shared.CurrentDirectoryStateVersion, loadedState.Version)
	}
	if loadedState.Browse == nil || loadedState.Browse.FilterBy == nil {
		t.Fatal("browse filterBy is missing")
	}
	if loadedState.Browse.FilterBy.Query != "test" {
		t.Errorf("expected Query to be 'test', got %q", loadedState.Browse.FilterBy.Query)
	}

	// 5. 写入 nil 应删除文件
	err = repo.WriteState(ctx, relpath.Path{}, nil)
	if err != nil {
		t.Fatalf("WriteState nil failed: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected state file to be deleted, stat err: %v", err)
	}
}

func TestDirectoryRepository_MigrateStateFromV1(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo := NewDirectoryRepository(tmpDir)

	// 模拟 v1 版本的 state 文件（使用 filterMemoBy）
	v1Content := map[string]any{
		"version": 1,
		"browse": map[string]any{
			"filterBy": map[string]any{
				"rating": []int{3, 4},
				"label":  []string{"red"},
				"query":  "test from v1",
			},
			"filterMemoBy": map[string]any{
				"hidden": false,
			},
		},
		"updatedAt": time.Now().Format(time.RFC3339),
	}
	v1JSON, _ := json.Marshal(v1Content)
	v1FilePath := filepath.Join(tmpDir, ".io.github.natescarlet.image-funnel.state.json")
	err := os.WriteFile(v1FilePath, v1JSON, 0644)
	if err != nil {
		t.Fatalf("failed to write v1 state file: %v", err)
	}

	// 读取应自动迁移到 v2
	loadedState, err := repo.ReadState(ctx, relpath.Path{})
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loadedState == nil {
		t.Fatal("expected state to be loaded, got nil")
	}

	// 验证版本已升级到当前版本
	if loadedState.Version != shared.CurrentDirectoryStateVersion {
		t.Errorf("expected Version to be %d, got %d", shared.CurrentDirectoryStateVersion, loadedState.Version)
	}

	// 验证 filterBy 已正确迁移
	if loadedState.Browse == nil || loadedState.Browse.FilterBy == nil {
		t.Fatal("browse filterBy is missing after migration")
	}
	if loadedState.Browse.FilterBy.Query != "test from v1" {
		t.Errorf("expected Query to be 'test from v1', got %q", loadedState.Browse.FilterBy.Query)
	}

	// 验证 filterMemoBy 已正确迁移到 filterNoteBy
	if loadedState.Browse.FilterNoteBy == nil {
		t.Fatal("browse filterNoteBy is missing after migration")
	}
	if loadedState.Browse.FilterNoteBy.Hidden == nil || *loadedState.Browse.FilterNoteBy.Hidden {
		t.Error("expected filterNoteBy.hidden to be false")
	}
}

func TestDirectoryRepository_MigrateStateFromV1WithLastSession(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo := NewDirectoryRepository(tmpDir)

	// 模拟带 LastSession 的 v1 state 文件
	v1Content := map[string]any{
		"version": 1,
		"browse": map[string]any{
			"filterMemoBy": map[string]any{
				"hidden": true,
			},
		},
		"lastSession": map[string]any{
			"id":         "sess:abc123",
			"filter":     map[string]any{"rating": []int{0}},
			"targetKeep": 5,
		},
		"updatedAt": time.Now().Format(time.RFC3339),
	}
	v1JSON, _ := json.Marshal(v1Content)
	v1FilePath := filepath.Join(tmpDir, ".io.github.natescarlet.image-funnel.state.json")
	err := os.WriteFile(v1FilePath, v1JSON, 0644)
	if err != nil {
		t.Fatalf("failed to write v1 state file: %v", err)
	}

	// 读取应自动迁移到 v2
	loadedState, err := repo.ReadState(ctx, relpath.Path{})
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loadedState == nil {
		t.Fatal("expected state to be loaded, got nil")
	}

	// 验证 LastSession 已正确迁移
	if loadedState.LastSession == nil {
		t.Fatal("lastSession is missing after migration")
	}
	if loadedState.LastSession.ID != scalar.ToID("sess:abc123") {
		t.Errorf("expected LastSession.ID to be 'sess:abc123', got %v", loadedState.LastSession.ID)
	}
	if loadedState.LastSession.TargetKeep != 5 {
		t.Errorf("expected LastSession.TargetKeep to be 5, got %d", loadedState.LastSession.TargetKeep)
	}
}

func TestDirectoryRepository_MigrateStateFromV1WithEmptyObjectID(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo := NewDirectoryRepository(tmpDir)

	// 模拟旧版本生成的 v1 state 文件：lastSession.id 为 {}（当时 scalar.ID 未实现 JSON 序列化，写入时空对象）
	v1Content := map[string]any{
		"version": 1,
		"lastSession": map[string]any{
			"id":         map[string]any{},
			"filter":     map[string]any{"rating": []int{0, 3, 4}},
			"targetKeep": 4,
		},
		"updatedAt": time.Now().Format(time.RFC3339),
	}
	v1JSON, _ := json.Marshal(v1Content)
	v1FilePath := filepath.Join(tmpDir, ".io.github.natescarlet.image-funnel.state.json")
	if err := os.WriteFile(v1FilePath, v1JSON, 0644); err != nil {
		t.Fatalf("failed to write v1 state file: %v", err)
	}

	loadedState, err := repo.ReadState(ctx, relpath.Path{})
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loadedState == nil {
		t.Fatal("expected state to be loaded, got nil")
	}
	if loadedState.Version != shared.CurrentDirectoryStateVersion {
		t.Errorf("expected Version to be %d, got %d", shared.CurrentDirectoryStateVersion, loadedState.Version)
	}

	// id 丢失的脏数据应降级为空 ID，其余字段保留
	if loadedState.LastSession == nil {
		t.Fatal("lastSession is missing after migration")
	}
	if !loadedState.LastSession.ID.IsZero() {
		t.Errorf("expected LastSession.ID to be empty, got %v", loadedState.LastSession.ID)
	}
	if loadedState.LastSession.TargetKeep != 4 {
		t.Errorf("expected LastSession.TargetKeep to be 4, got %d", loadedState.LastSession.TargetKeep)
	}
	if !slices.Equal(loadedState.LastSession.Filter.Rating, []int{0, 3, 4}) {
		t.Errorf("expected filter rating to be [0 3 4], got %v", loadedState.LastSession.Filter.Rating)
	}
}

func TestDirectoryRepository_MigrateStateFromV2(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo := NewDirectoryRepository(tmpDir)

	// v2 的 commitActions 应迁移为顶层的 default.writeActions
	v2Content := map[string]any{
		"version": 2,
		"browse": map[string]any{
			"filterBy": map[string]any{"query": "from v2"},
		},
		"lastSession": map[string]any{
			"id":            "sess:v2",
			"filter":        map[string]any{"rating": []int{4}},
			"targetKeep":    6,
			"commitActions": map[string]any{"keepRating": 3, "shelveRating": 2, "rejectRating": 1},
		},
		"updatedAt": time.Now().Format(time.RFC3339),
	}
	v2JSON, _ := json.Marshal(v2Content)
	if err := os.WriteFile(filepath.Join(tmpDir, stateFileName), v2JSON, 0644); err != nil {
		t.Fatalf("failed to write v2 state file: %v", err)
	}

	loaded, err := repo.ReadState(ctx, relpath.Path{})
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loaded.Version != shared.CurrentDirectoryStateVersion {
		t.Errorf("迁移后版本 = %d, want %d", loaded.Version, shared.CurrentDirectoryStateVersion)
	}
	if loaded.Browse == nil || loaded.Browse.FilterBy == nil || loaded.Browse.FilterBy.Query != "from v2" {
		t.Errorf("v2 的 browse.filterBy 未正确迁移: %+v", loaded.Browse)
	}
	if loaded.LastSession == nil || loaded.LastSession.ID != scalar.ToID("sess:v2") || loaded.LastSession.TargetKeep != 6 {
		t.Errorf("v2 的 lastSession 未正确迁移: %+v", loaded.LastSession)
	}
	if loaded.Default == nil || loaded.Default.WriteActions == nil ||
		loaded.Default.WriteActions.KeepRating == nil || *loaded.Default.WriteActions.KeepRating != 3 {
		t.Errorf("v2 的 commitActions 未迁移为 default.writeActions: %+v", loaded.Default)
	}
}

// 仓储不得强制覆写调用者写入的版本；版本由写入方统一引用当前版本常量决定
func TestDirectoryRepository_WriteStateDoesNotOverrideVersion(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	repo := NewDirectoryRepository(tmpDir)

	// 使用与当前版本不同的值，确保强制覆写语句一旦回归就会被捕获
	const callerVersion = 99
	state := &shared.DirectoryStateDTO{Version: callerVersion}
	if err := repo.WriteState(ctx, relpath.Path{}, state); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, stateFileName))
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var raw struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal state file: %v", err)
	}
	if raw.Version != callerVersion {
		t.Errorf("落盘版本 = %d, want %d（仓储不应强制覆写）", raw.Version, callerVersion)
	}
}
