package directory

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/shared"
	"testing"

	"go.uber.org/zap"
)

// listingRepository 按固定顺序返回子目录，模拟文件系统遍历（目录名升序）
type listingRepository struct {
	mockRepository
	names []string
}

func (r *listingRepository) Find(ctx context.Context, relPath relpath.Path) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {
		for _, name := range r.names {
			dir, err := relPath.Join(testRootDir, name)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(directory.FromRepository(dir), nil) {
				return
			}
		}
	}
}

// noopAnalyzer 统计接口的空实现，避免测试注入可选依赖
type noopAnalyzer struct{}

func (noopAnalyzer) Analyze(ctx context.Context, relPath relpath.Path) (*directory.Stats, error) {
	return directory.NewStats(0, 0, nil, nil), nil
}

func newListingHandler(t *testing.T, names ...string) *Handler {
	t.Helper()
	logger := zap.NewNop()
	repo := &listingRepository{
		mockRepository: mockRepository{dirs: make(map[relpath.Path]*directory.Directory)},
		names:          names,
	}
	watcher := &mockWatcher{}
	pub := &mockFileChangedPub{ch: make(chan *directory.FileChangedEvent, 100)}
	dirSvc, cleanup := directory.NewService(watcher, pub, testRootDir, repo, &mockRenamer{}, logger)
	t.Cleanup(cleanup)

	return NewHandler(logger, noopAnalyzer{}, NewDTOFactory(nil), directory.NewFilterBuilder(), repo, dirSvc, pub)
}

func listDirectories(t *testing.T, h *Handler, parentID string, pageSize int, filter shared.DirectoryFilters, after *string) *shared.DirectoryConnectionDTO {
	t.Helper()
	conn, err := h.Directories(context.Background(), mustDir(t, parentID).ID(), filter, &pageSize, after, nil, nil)
	if err != nil {
		t.Fatalf("list directories: %v", err)
	}
	return conn
}

func listDirectoriesBackward(t *testing.T, h *Handler, parentID string, pageSize int, before string) *shared.DirectoryConnectionDTO {
	t.Helper()
	conn, err := h.Directories(context.Background(), mustDir(t, parentID).ID(), shared.DirectoryFilters{}, nil, nil, &pageSize, &before)
	if err != nil {
		t.Fatalf("list directories backward: %v", err)
	}
	return conn
}

func nodeRelPaths(conn *shared.DirectoryConnectionDTO) []string {
	var paths []string
	for _, edge := range conn.Edges {
		paths = append(paths, edge.Node.RelPath)
	}
	return paths
}

// 跨页游标必须在同一比较空间内：字符串比较结果等于目录在整个查询结果中的先后顺序
func TestDirectories_CursorsAreComparableAcrossPages(t *testing.T) {
	h := newListingHandler(t, "a", "b", "c", "d")

	first := listDirectories(t, h, "parent", 2, shared.DirectoryFilters{}, nil)
	if got := nodeRelPaths(first); len(got) != 2 || got[0] != "parent/a" || got[1] != "parent/b" {
		t.Fatalf("unexpected first page: %v", got)
	}
	if !first.PageInfo.HasNextPage {
		t.Fatal("first page should have next page")
	}

	after := first.PageInfo.EndCursor
	second := listDirectories(t, h, "parent", 2, shared.DirectoryFilters{}, &after)
	if got := nodeRelPaths(second); len(got) != 2 || got[0] != "parent/c" || got[1] != "parent/d" {
		t.Fatalf("unexpected second page: %v", got)
	}

	cursors := []string{
		first.Edges[0].Cursor,
		first.Edges[1].Cursor,
		second.Edges[0].Cursor,
		second.Edges[1].Cursor,
	}
	for i := 1; i < len(cursors); i++ {
		if !(cursors[i-1] < cursors[i]) {
			t.Fatalf("cursors are not ordered across pages: %v", cursors)
		}
	}
	for _, cursor := range cursors {
		if cursor == "" {
			t.Fatal("cursor should not be empty")
		}
	}
}

// 游标锚定目录名而不是下标：前置新增目录不会让后续页的位置整体平移
func TestDirectories_CursorStaysStableWhenDirectoryAddedBefore(t *testing.T) {
	before := newListingHandler(t, "a", "b", "c", "d")
	first := listDirectories(t, before, "parent", 2, shared.DirectoryFilters{}, nil)
	after := first.PageInfo.EndCursor

	// 在 "a" 之前插入 "0"，按下标分页时后续页会整体偏移一位
	afterInsert := newListingHandler(t, "0", "a", "b", "c", "d")
	second := listDirectories(t, afterInsert, "parent", 2, shared.DirectoryFilters{}, &after)
	if got := nodeRelPaths(second); len(got) != 2 || got[0] != "parent/c" || got[1] != "parent/d" {
		t.Fatalf("cursor should keep pointing at the same position, got %v", got)
	}
}

func TestDirectories_BackwardPaging(t *testing.T) {
	h := newListingHandler(t, "a", "b", "c", "d", "e")

	all := listDirectories(t, h, "parent", 10, shared.DirectoryFilters{}, nil)
	if len(all.Edges) != 5 {
		t.Fatalf("unexpected full list: %v", nodeRelPaths(all))
	}
	before := all.Edges[4].Cursor

	// 从 "e" 之前取一页：应得到紧邻游标之前的条目，且保持正序
	previous := listDirectoriesBackward(t, h, "parent", 2, before)
	if got := nodeRelPaths(previous); len(got) != 2 || got[0] != "parent/c" || got[1] != "parent/d" {
		t.Fatalf("unexpected backward page: %v", got)
	}
	if !previous.PageInfo.HasPreviousPage {
		t.Fatal("backward page should report remaining earlier items")
	}
	if !previous.PageInfo.HasNextPage {
		t.Fatal("backward page should report later items")
	}
}

func TestDirectories_FilteredCursorsShareTheSameSpace(t *testing.T) {
	h := newListingHandler(t, "a-1", "b-1", "b-2", "b-3")
	filter := shared.DirectoryFilters{Query: "b-"}

	first := listDirectories(t, h, "parent", 2, filter, nil)
	if got := nodeRelPaths(first); len(got) != 2 || got[0] != "parent/b-1" || got[1] != "parent/b-2" {
		t.Fatalf("unexpected filtered first page: %v", got)
	}
	after := first.PageInfo.EndCursor
	second := listDirectories(t, h, "parent", 2, filter, &after)
	if got := nodeRelPaths(second); len(got) != 1 || got[0] != "parent/b-3" {
		t.Fatalf("unexpected filtered second page: %v", got)
	}
	if !(first.Edges[1].Cursor < second.Edges[0].Cursor) {
		t.Fatalf("filtered cursors should stay ordered: %q < %q", first.Edges[1].Cursor, second.Edges[0].Cursor)
	}
}
