package hook

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"main/internal/domain/directory"
	"main/internal/scalar"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
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

// TestSplitArgs_SingleQuoted 单引号包裹的参数作为一个整体
func TestSplitArgs_SingleQuoted(t *testing.T) {
	assert.Equal(t, []string{"hello world"}, splitArgs(`'hello world'`))
}

// TestSplitArgs_CommandTokenizer 用于将 command 字段分词为 argv（shell 作为程序被调用）
func TestSplitArgs_CommandTokenizer(t *testing.T) {
	assert.Equal(t,
		[]string{"cmd", "/c", `echo %IMAGE_FUNNEL_DIRECTORY_ID%^|%IMAGE_FUNNEL_DIRECTORY_REL_PATH% > flag`},
		splitArgs(`cmd /c "echo %IMAGE_FUNNEL_DIRECTORY_ID%^|%IMAGE_FUNNEL_DIRECTORY_REL_PATH% > flag"`))
	assert.Equal(t,
		[]string{"sh", "-c", `echo "$IMAGE_FUNNEL_DIRECTORY_ID|$IMAGE_FUNNEL_DIRECTORY_REL_PATH" > flag`},
		splitArgs(`sh -c 'echo "$IMAGE_FUNNEL_DIRECTORY_ID|$IMAGE_FUNNEL_DIRECTORY_REL_PATH" > flag'`))
	assert.Equal(t,
		[]string{"uv", "run", "runner.py", "comfyui", "add"},
		splitArgs("uv run runner.py comfyui add"))
}

// TestRunner_ExecuteNoteDirectives_LiteralArgChars 验证指令参数中的 shell 元字符（如 >）
// 必须作为字面参数传递给钩子脚本，而不是被 shell 解释为重定向等语法
func TestRunner_ExecuteNoteDirectives_LiteralArgChars(t *testing.T) {
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("python not found, skipping")
	}
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-literal-arg-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "args_flag.txt")

	// 钩子脚本：把收到的所有参数用 "|" 连接后写入 flag 文件，argv[0] 是脚本路径本身
	script := fmt.Sprintf("import sys\nwith open(r'%s', 'w') as f:\n    f.write('|'.join(sys.argv[1:]))\n", flagFile)
	err = os.WriteFile(filepath.Join(hooksDir, "echo_args.py"), []byte(script), 0644)
	assert.NoError(t, err)

	hookToml := `
id = "literal-arg-test"
name = "literal-arg-test"
command = "python echo_args.py"

[directive]
name = "literal-arg"
on_success_action = "REMOVE"
on_fail_action = "KEEP"

[on.post_update_note]
`
	err = os.WriteFile(filepath.Join(hooksDir, "literal-arg.toml"), []byte(hookToml), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, &mockImageRepository{}, nil, mockDirRepo, &mockNotificationSender{})
	defer runner.Close()

	// 复现用户上报的畸形指令：alert 块被粘到指令行末尾后，参数中出现 ">" 特殊字符
	noteRelPath := "test_note.md"
	noteAbsPath := filepath.Join(tempDir, noteRelPath)
	literalArg := "x:x-0.6:0.2>[!stdout]2026-07-31T13:21:15"
	initialContent := fmt.Sprintf("/literal-arg %s\n", literalArg)
	err = os.WriteFile(noteAbsPath, []byte(initialContent), 0644)
	assert.NoError(t, err)

	ok, err := runner.executeNoteDirectives(context.Background(), directory.FromRepository("."), noteRelPath, initialContent, "post_update_note", scalar.ID{})
	assert.NoError(t, err)
	assert.True(t, ok)

	// 脚本应收到完整字面参数（含 ">"），而不是被 shell 解释为重定向
	contentBytes, err := os.ReadFile(flagFile)
	assert.NoError(t, err)
	assert.Equal(t, literalArg, strings.TrimSpace(string(contentBytes)))

	// 指令执行成功后 on_success_action=REMOVE 将指令行移除，笔记内容为空时文件被删除
	_, err = os.Stat(noteAbsPath)
	assert.True(t, os.IsNotExist(err), "指令成功后笔记应为空并被删除")
}
