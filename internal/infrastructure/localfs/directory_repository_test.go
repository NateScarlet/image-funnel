package localfs

import (
	"context"
	"encoding/json"
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
	state, err := repo.ReadState(ctx, "")
	if err != nil {
		t.Fatalf("ReadState empty failed: %v", err)
	}
	if state != nil {
		t.Fatalf("expected state to be nil, got: %v", state)
	}

	// 2. 写入状态
	originalState := &shared.DirectoryStateDTO{
		Browse: &shared.DirectoryStateBrowseDTO{
			FilterBy: &shared.ImageFilters{
				Rating: []int{3, 4},
				Label:  []string{"red"},
				Query:  "test",
			},
		},
	}
	err = repo.WriteState(ctx, "", originalState)
	if err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	// 3. 验证是否生成了文件
	filePath := filepath.Join(tmpDir, ".io.github.natescarlet.image-funnel.state.json")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("state file was not created: %v", err)
	}

	// 4. 读取状态并比对
	loadedState, err := repo.ReadState(ctx, "")
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loadedState == nil {
		t.Fatal("expected state to be loaded, got nil")
	}
	if loadedState.Version != 3 {
		t.Errorf("expected Version to be 3, got %d", loadedState.Version)
	}
	if loadedState.Browse == nil || loadedState.Browse.FilterBy == nil {
		t.Fatal("browse filterBy is missing")
	}
	if loadedState.Browse.FilterBy.Query != "test" {
		t.Errorf("expected Query to be 'test', got %q", loadedState.Browse.FilterBy.Query)
	}

	// 5. 写入 nil 应删除文件
	err = repo.WriteState(ctx, "", nil)
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
	loadedState, err := repo.ReadState(ctx, "")
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loadedState == nil {
		t.Fatal("expected state to be loaded, got nil")
	}

	// 验证版本已升级到 3
	if loadedState.Version != 3 {
		t.Errorf("expected Version to be 3, got %d", loadedState.Version)
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
	loadedState, err := repo.ReadState(ctx, "")
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

	loadedState, err := repo.ReadState(ctx, "")
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if loadedState == nil {
		t.Fatal("expected state to be loaded, got nil")
	}
	if loadedState.Version != 3 {
		t.Errorf("expected Version to be 3, got %d", loadedState.Version)
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
