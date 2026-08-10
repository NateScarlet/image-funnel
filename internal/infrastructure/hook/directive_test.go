package hook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestApplyDirectiveAction_Keep_NoTrailingNewline 指令行位于笔记最后一行且无尾换行时，
// alert 块必须与指令行之间以换行分隔，否则 `>[!stdout]...` 会被粘到指令行末尾，
// 下一次扫描时被误识别为指令参数并在 shell 中被解释为重定向
func TestApplyDirectiveAction_Keep_NoTrailingNewline(t *testing.T) {
	ts := time.Date(2026, 7, 31, 13, 21, 15, 0, time.UTC)
	result := applyDirectiveAction("KEEP", "/adjust prompt leaning_in x:x-0.6:0.2", "stdout line", "", ts)
	assert.Equal(t, "/adjust prompt leaning_in x:x-0.6:0.2\n>[!stdout]2026-07-31T13:21:15\n> stdout line\n", result)
}

// TestApplyDirectiveAction_Keep_WithTrailingNewline 指令行已含尾换行时，不应重复插入换行产生空行
func TestApplyDirectiveAction_Keep_WithTrailingNewline(t *testing.T) {
	result := applyDirectiveAction("KEEP", "/fork a\n", "ok", "", time.Time{})
	assert.Equal(t, "/fork a\n>[!stdout]0001-01-01T00:00:00\n> ok\n", result)
}

// TestApplyDirectiveAction_Keep_CRLF 保持 CRLF 换行风格且不重复插入换行
func TestApplyDirectiveAction_Keep_CRLF(t *testing.T) {
	result := applyDirectiveAction("KEEP", "/fork a\r\n", "ok", "", time.Time{})
	assert.Equal(t, "/fork a\r\n>[!stdout]0001-01-01T00:00:00\r\n> ok\r\n", result)
}

// TestApplyDirectiveAction_CommentOut_NoTrailingNewline COMMENT_OUT 分支同样需要保证 alert 块独占一行
func TestApplyDirectiveAction_CommentOut_NoTrailingNewline(t *testing.T) {
	result := applyDirectiveAction("COMMENT_OUT", "/fork a", "ok", "", time.Time{})
	assert.Equal(t, "%% /fork a %%\n>[!stdout]0001-01-01T00:00:00\n> ok\n", result)
}

// TestApplyDirectiveAction_Keep_NoOutput 无输出时 alert 块为空，保持原指令行不变
func TestApplyDirectiveAction_Keep_NoOutput(t *testing.T) {
	result := applyDirectiveAction("KEEP", "/fork a", "", "", time.Time{})
	assert.Equal(t, "/fork a", result)
}

// TestApplyDirectiveAction_Remove 移除指令行
func TestApplyDirectiveAction_Remove(t *testing.T) {
	result := applyDirectiveAction("REMOVE", "/fork a", "stdout", "stderr", time.Time{})
	assert.Equal(t, "", result)
}
