package inmem

import (
	"context"
	"main/internal/shared"
	"os"
	"path/filepath"
	"testing"
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
	if loadedState.Version != 1 {
		t.Errorf("expected Version to be 1, got %d", loadedState.Version)
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
