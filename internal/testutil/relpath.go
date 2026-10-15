// Package testutil 提供跨包复用的测试辅助函数。
//
// 只放被多个包的 _test.go 文件共同依赖的东西；单个包自用的辅助留在该包的测试文件里。
package testutil

import (
	"path/filepath"
	"testing"

	"main/internal/domain/relpath"
)

// Rel 在一个虚拟根下构造根相对路径，供只关心路径字面形态、不触碰磁盘的测试使用。
// 围栏结果与真实根目录无关，因此根只是形式参数。
func Rel(t testing.TB, relPath string) relpath.Path {
	t.Helper()
	return RelIn(t, filepath.Join(t.TempDir(), "virtual-root"), relPath)
}

// RelIn 在指定根目录下构造根相对路径，供会据此访问磁盘的测试使用。
func RelIn(t testing.TB, rootDir, relPath string) relpath.Path {
	t.Helper()
	p, err := relpath.New(rootDir, relPath)
	if err != nil {
		t.Fatalf("构造根相对路径 %q 失败: %v", relPath, err)
	}
	return p
}