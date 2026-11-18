package main

import (
	"os"
	"path/filepath"
	"testing"
)

// #region 训练样本归档根目录解析
func TestSampleArchiveRoot(t *testing.T) {
	t.Run("归档根目录位于 DataDir 下的 training-samples 子目录", func(t *testing.T) {
		dataDir := t.TempDir()
		got, err := sampleArchiveRoot(dataDir)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(dataDir, "training-samples")
		if got != want {
			t.Fatalf("sampleArchiveRoot(%q) = %q, want %q", dataDir, got, want)
		}
	})

	t.Run("相对 DataDir 解析为绝对路径", func(t *testing.T) {
		got, err := sampleArchiveRoot(filepath.Join("relative", "data-dir"))
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(got) {
			t.Fatalf("sampleArchiveRoot returned non-absolute path: %q", got)
		}
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(wd, "relative", "data-dir", "training-samples")
		if got != want {
			t.Fatalf("sampleArchiveRoot = %q, want %q", got, want)
		}
	})
}

// #endregion
