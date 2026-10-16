package testutil

import (
	"os"
	"testing"
)

// AssertAtomicReplace 断言一次覆盖写是原子落位，而不是对原文件就地截断覆盖。
//
// 探测方式：写入前为目标文件建立第二个硬链接作为旧内容的观察点。
// 原子落位换用新文件后，旧硬链接仍指向旧文件、内容保持旧全文；
// 就地截断覆盖则会让观察点与新内容同源，暴露「写入过程中读者可能看到半截内容」。
func AssertAtomicReplace(t testing.TB, absPath, oldContent, newContent string, write func() error) {
	t.Helper()

	observedPath := absPath + ".old"
	if err := os.Link(absPath, observedPath); err != nil {
		t.Fatalf("建立旧内容观察点失败: %v", err)
	}
	defer os.Remove(observedPath)

	if err := write(); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	assertFileContent(t, absPath, newContent, "写入后目标路径应为新全文")
	assertFileContent(t, observedPath, oldContent, "旧文件被就地覆盖，写入不是原子落位")
}

// assertFileContent 断言文件内容与预期完全一致，失败时只报告长度以避免大内容刷屏
func assertFileContent(t testing.TB, path string, want string, message string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: 读取 %s 失败: %v", message, path, err)
	}
	if string(got) != want {
		t.Fatalf("%s: %s 内容不符合预期（got %d 字节, want %d 字节）", message, path, len(got), len(want))
	}
}
