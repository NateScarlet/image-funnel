package directory

import (
	"testing"

	"main/internal/shared"
)

func TestFilterBuilder_Build(t *testing.T) {
	fb := NewFilterBuilder()

	// 准备测试目录数据
	dirs := []*Directory{
		FromRepository(""),
		FromRepository("A"),
		FromRepository("A/Subdir"),
		FromRepository("A/Subdir-Test"),
		FromRepository("A/subdir-test"),
		FromRepository("B/Other"),
	}

	t.Run("empty filters", func(t *testing.T) {
		filter := fb.Build(shared.DirectoryFilters{})
		for _, dir := range dirs {
			if !filter(dir) {
				t.Errorf("expected dir %s to match, but it did not", dir.RelPath())
			}
		}
	})

	t.Run("query matching subdir name case-insensitive", func(t *testing.T) {
		filter := fb.Build(shared.DirectoryFilters{
			Query: "Subdir",
		})

		expectedMatches := map[string]bool{
			"A/Subdir":      true,
			"A/Subdir-Test": true,
			"A/subdir-test": true,
		}

		for _, dir := range dirs {
			got := filter(dir)
			want := expectedMatches[dir.RelPath()]
			if got != want {
				t.Errorf("for path %q, got match=%v, want=%v", dir.RelPath(), got, want)
			}
		}
	})

	t.Run("query matching test case-insensitive", func(t *testing.T) {
		filter := fb.Build(shared.DirectoryFilters{
			Query: "test",
		})

		expectedMatches := map[string]bool{
			"A/Subdir-Test": true,
			"A/subdir-test": true,
		}

		for _, dir := range dirs {
			got := filter(dir)
			want := expectedMatches[dir.RelPath()]
			if got != want {
				t.Errorf("for path %q, got match=%v, want=%v", dir.RelPath(), got, want)
			}
		}
	})
}
