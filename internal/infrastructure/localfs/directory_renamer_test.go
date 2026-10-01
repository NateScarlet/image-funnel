package localfs

import (
	"context"
	"main/internal/apperror"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryRenamer_Rename(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "parent", "old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "parent", "old", "keep.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	renamer := NewDirectoryRenamer(tmpDir)

	newRelPath, err := renamer.Rename(ctx, filepath.Join("parent", "old"), "new")
	if err != nil {
		t.Fatalf("Rename() unexpected error: %v", err)
	}
	if newRelPath != "parent/new" {
		t.Errorf("newRelPath = %q, want %q", newRelPath, "parent/new")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "parent", "old")); !os.IsNotExist(err) {
		t.Errorf("old directory still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "parent", "new", "keep.png")); err != nil {
		t.Errorf("content did not follow the rename: %v", err)
	}
}

func TestDirectoryRenamer_Rename_TargetExists(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	for _, name := range []string{"old", "taken"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	renamer := NewDirectoryRenamer(tmpDir)

	_, err := renamer.Rename(ctx, "old", "taken")
	if err == nil {
		t.Fatal("expected error when target name already exists")
	}
	if code := apperror.ErrCode(err); code != "DIRECTORY_ALREADY_EXISTS" {
		t.Errorf("error code = %q, want DIRECTORY_ALREADY_EXISTS (err: %v)", code, err)
	}
	// 冲突失败后原目录应保持原状
	if _, err := os.Stat(filepath.Join(tmpDir, "old")); err != nil {
		t.Errorf("source directory should be untouched: %v", err)
	}
}

func TestDirectoryRenamer_Rename_TargetIsFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "name.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	renamer := NewDirectoryRenamer(tmpDir)

	_, err := renamer.Rename(ctx, "old", "name.png")
	if err == nil {
		t.Fatal("expected error when target name is taken by a file")
	}
	if code := apperror.ErrCode(err); code != "DIRECTORY_ALREADY_EXISTS" {
		t.Errorf("error code = %q, want DIRECTORY_ALREADY_EXISTS (err: %v)", code, err)
	}
	// 同名的既有文件不能被目录覆盖
	if _, err := os.Stat(filepath.Join(tmpDir, "name.png")); err != nil {
		t.Errorf("existing file should be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "old")); err != nil {
		t.Errorf("source directory should be untouched: %v", err)
	}
}

func TestDirectoryRenamer_Rename_SourceMissing(t *testing.T) {
	ctx := context.Background()
	renamer := NewDirectoryRenamer(t.TempDir())

	if _, err := renamer.Rename(ctx, "missing", "new"); err == nil {
		t.Fatal("expected error when source directory does not exist")
	}
}

func TestDirectoryRenamer_Rename_EscapeRoot(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "old"), 0755); err != nil {
		t.Fatal(err)
	}

	renamer := NewDirectoryRenamer(tmpDir)

	if _, err := renamer.Rename(ctx, "old", filepath.Join("..", "escaped")); err == nil {
		t.Fatal("expected error when target escapes root directory")
	}
}
