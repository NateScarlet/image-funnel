package hook

import (
	"os"
	"path/filepath"
	"testing"

	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteGuard_SuppressesOwnWriteEvent 防重入守卫的写侧登记与事件侧查询共享同一状态
func TestWriteGuard_SuppressesOwnWriteEvent(t *testing.T) {
	guard := newWriteGuard()
	noteAbs := filepath.Join(t.TempDir(), "note.md")

	assert.False(t, guard.ShouldIgnore(noteAbs, []byte("body")), "未登记过的路径不应被忽略")

	require.NoError(t, guard.Write(noteAbs, []byte("body"), 0644))
	assert.Equal(t, "body", readFile(t, noteAbs), "登记的写入应真正落盘")

	assert.True(t, guard.ShouldIgnore(noteAbs, []byte("body")), "自身写入且内容一致的事件应被抑制")
	assert.False(t, guard.ShouldIgnore(noteAbs, []byte("other")), "内容与自身写入不一致的事件不应被抑制")
}

// TestWriteGuard_WritesAtomically 守卫委托的写入是原子落位：
// 旧文件不会被就地截断，读者手中的旧文件始终保持旧全文
func TestWriteGuard_WritesAtomically(t *testing.T) {
	guard := newWriteGuard()
	noteAbs := filepath.Join(t.TempDir(), "note.md")

	require.NoError(t, os.WriteFile(noteAbs, []byte("old"), 0644))
	testutil.AssertAtomicReplace(t, noteAbs, "old", "new", func() error {
		return guard.Write(noteAbs, []byte("new"), 0644)
	})
}
