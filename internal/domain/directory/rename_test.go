package directory

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"testing"

	"go.uber.org/zap"
)

type mockRenamer struct {
	gotRelPath relpath.Path
	gotNewName string
	result     relpath.Path
	err        error
	calls      int
}

func (m *mockRenamer) Rename(ctx context.Context, relPath relpath.Path, newName string) (relpath.Path, error) {
	m.calls++
	m.gotRelPath = relPath
	m.gotNewName = newName
	if m.err != nil {
		return relpath.Path{}, m.err
	}
	if !m.result.IsRoot() {
		return m.result, nil
	}
	newRelPath, err := relPath.Dir().Join(testRootDir, newName)
	if err != nil {
		return relpath.Path{}, err
	}
	return newRelPath, nil
}

// dirAt 构造测试用的目录实体
func dirAt(t *testing.T, relPath string) *Directory {
	t.Helper()
	p, err := relpath.New(testRootDir, relPath)
	if err != nil {
		t.Fatalf("failed to build root-relative path %q: %v", relPath, err)
	}
	return FromRepository(p)
}

func newRenameTestService(t *testing.T, repo Repository, renamer Renamer) *Service {
	t.Helper()
	s, cleanup := NewService(&mockWatcher{}, &mockFileChangedPub{}, testRootDir, repo, renamer, zap.NewNop())
	t.Cleanup(cleanup)
	return s
}

func TestService_Rename(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepository{}
	renamer := &mockRenamer{}
	svc := newRenameTestService(t, repo, renamer)

	dir, err := svc.Rename(ctx, dirAt(t, "parent/old").ID(), "new")
	if err != nil {
		t.Fatalf("Rename() unexpected error: %v", err)
	}
	if renamer.gotRelPath.String() != "parent/old" {
		t.Errorf("renamer received relPath %q, want %q", renamer.gotRelPath, "parent/old")
	}
	if renamer.gotNewName != "new" {
		t.Errorf("renamer received newName %q, want %q", renamer.gotNewName, "new")
	}
	if dir.RelPath().String() != "parent/new" {
		t.Errorf("returned relPath %q, want %q", dir.RelPath().String(), "parent/new")
	}
	if dir.ParentID() != dirAt(t, "parent").ID() {
		t.Errorf("parent ID changed unexpectedly: %v", dir.ParentID())
	}
	if dir.ID() == dirAt(t, "parent/old").ID() {
		t.Error("expected directory ID to change after rename")
	}
}

func TestService_Rename_RootNotAllowed(t *testing.T) {
	ctx := context.Background()
	renamer := &mockRenamer{}
	svc := newRenameTestService(t, &mockRepository{}, renamer)

	_, err := svc.Rename(ctx, dirAt(t, "").ID(), "new")
	if err == nil {
		t.Fatal("expected error renaming root directory")
	}
	if code := apperror.ErrCode(err); code != "ROOT_DIRECTORY_NOT_RENAMEABLE" {
		t.Errorf("unexpected error code %q: %v", code, err)
	}
	if renamer.calls != 0 {
		t.Errorf("renamer should not be called, got %d calls", renamer.calls)
	}
}

func TestService_Rename_InvalidName(t *testing.T) {
	ctx := context.Background()
	invalidNames := map[string]string{
		"empty":             "",
		"blank":             "   ",
		"forward slash":     "a/b",
		"backslash":         `a\b`,
		"current dir":       ".",
		"parent dir":        "..",
		"absolute":          "C:/abs",
		"leading separator": "/abs",
	}
	for name, newName := range invalidNames {
		t.Run(name, func(t *testing.T) {
			renamer := &mockRenamer{}
			svc := newRenameTestService(t, &mockRepository{}, renamer)

			_, err := svc.Rename(ctx, dirAt(t, "parent/old").ID(), newName)
			if err == nil {
				t.Fatalf("expected error for name %q", newName)
			}
			if code := apperror.ErrCode(err); code != "DIRECTORY_NAME_INVALID" {
				t.Errorf("unexpected error code %q: %v", code, err)
			}
			if renamer.calls != 0 {
				t.Errorf("renamer should not be called, got %d calls", renamer.calls)
			}
		})
	}
}

func TestService_Rename_PropagateRenamerError(t *testing.T) {
	ctx := context.Background()
	renamer := &mockRenamer{err: apperror.New("DIRECTORY_ALREADY_EXISTS", "exists", "已存在")}
	svc := newRenameTestService(t, &mockRepository{}, renamer)

	if _, err := svc.Rename(ctx, dirAt(t, "parent/old").ID(), "new"); err == nil {
		t.Fatal("expected error from renamer")
	}
}

func TestService_Rename_InvalidDirectoryID(t *testing.T) {
	ctx := context.Background()
	renamer := &mockRenamer{}
	svc := newRenameTestService(t, &mockRepository{}, renamer)

	_, err := svc.Rename(ctx, scalar.ToID("img:whatever"), "new")
	if err == nil {
		t.Fatal("expected error for non-directory ID")
	}
	if renamer.calls != 0 {
		t.Errorf("renamer should not be called, got %d calls", renamer.calls)
	}
}
