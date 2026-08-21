package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"main/internal/domain/directory"
	"main/internal/scalar"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestIFCopyContentHelper 当环境变量 IF_COPY_HELPER=1 时，作为复制增强脚本的假实现运行：
// 按环境变量配置输出 stdout/stderr 并以指定退出码退出（复用当前测试二进制，避免依赖外部运行时）
func TestIFCopyContentHelper(t *testing.T) {
	if os.Getenv("IF_COPY_HELPER") != "1" {
		return
	}

	if _, ok := os.LookupEnv("IF_COPY_HELPER_ECHO_ENV"); ok {
		// 契约模式：将注入的触发器名与图片上下文回显进信封 content
		payload, _ := json.Marshal(map[string]string{
			"trigger":    os.Getenv("IMAGE_FUNNEL_TRIGGER"),
			"imageIDs":   os.Getenv("IMAGE_FUNNEL_IMAGE_IDS"),
			"imagePaths": os.Getenv("IMAGE_FUNNEL_IMAGE_PATHS"),
			"custom":     os.Getenv("IF_COPY_CUSTOM_ENV"),
		})
		contentStr, _ := json.Marshal(string(payload)) // content 契约为字符串，需按字符串转义
		fmt.Printf(`{"content":%s,"description":"copied"}`, contentStr)
	} else if stdout := os.Getenv("IF_COPY_HELPER_STDOUT"); stdout != "" {
		fmt.Print(stdout)
	}
	if stderr := os.Getenv("IF_COPY_HELPER_STDERR"); stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}
	os.Exit(atoiDefault(os.Getenv("IF_COPY_HELPER_EXIT_CODE"), 0))
}

func atoiDefault(s string, fallback int) int {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return fallback
	}
	return v
}

// #region 测试基础设施

type copyTestEnv struct {
	tempDir  string
	hooksDir string
	runner   *Runner
	hookIDs  []scalar.ID
}

// newCopyTestRunner 搭建一个配置了 n 个 [copy] 假脚本钩子的测试 Runner
func newCopyTestRunner(t *testing.T, n int, envOverrides map[string]string) *copyTestEnv {
	t.Helper()
	tempDir := t.TempDir()
	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))

	helperCmd := fmt.Sprintf(`"%s" -test.run=^TestIFCopyContentHelper$`, os.Args[0])

	var envSB strings.Builder
	envSB.WriteString("[env]\n")
	envSB.WriteString("IF_COPY_HELPER = '1'\n")
	for k, v := range envOverrides {
		fmt.Fprintf(&envSB, "%s = '%s'\n", k, v)
	}
	envSection := envSB.String()

	env := &copyTestEnv{tempDir: tempDir, hooksDir: hooksDir}
	for i := 0; i < n; i++ {
		tomlContent := fmt.Sprintf(`
id = "copy-hook-%d"
name = "copy-hook-%d"
command = '''%s'''

[copy]

%s
`, i, i, helperCmd, envSection)
		path := filepath.Join(hooksDir, fmt.Sprintf("copy-%d.toml", i))
		require.NoError(t, os.WriteFile(path, []byte(tomlContent), 0644))
		env.hookIDs = append(env.hookIDs, scalar.ToID(fmt.Sprintf("hk:copy-hook-%d", i)))
	}

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".":   directory.FromRepository("."),
			"sub": directory.FromRepository("sub"),
		},
	}
	runner := NewRunner(tempDir, hooksDir, zap.NewNop(), ebus, fileChangedSub, "", &mockTokenSource{}, &mockImageRepository{}, nil, mockDirRepo, &mockNotificationSender{})
	t.Cleanup(runner.Close)
	env.runner = runner
	return env
}

// #endregion

func TestCopyContent_NoConfigReturnsNil(t *testing.T) {
	env := newCopyTestRunner(t, 0, nil)

	content, err := env.runner.CopyContent(context.Background(), scalar.ToID("img:1"), "test.png")
	require.NoError(t, err)
	assert.Nil(t, content, "未配置 [copy] 钩子时应返回空以降级为复制文件")
}

func TestCopyContent_MultipleConfigsError(t *testing.T) {
	env := newCopyTestRunner(t, 2, nil)

	content, err := env.runner.CopyContent(context.Background(), scalar.ToID("img:1"), "test.png")
	require.Error(t, err)
	assert.Nil(t, content)
	// 报错应列出所有冲突的 hook id，便于用户定位重复配置
	assert.Contains(t, err.Error(), "copy-hook-0")
	assert.Contains(t, err.Error(), "copy-hook-1")
}

func TestCopyContent_EnvelopeParsing(t *testing.T) {
	envelope := `{"content":"workflow-json","description":"copied!"}`
	env := newCopyTestRunner(t, 1, map[string]string{"IF_COPY_HELPER_STDOUT": envelope})

	content, err := env.runner.CopyContent(context.Background(), scalar.ToID("img:1"), "sub/test.png")
	require.NoError(t, err)
	require.NotNil(t, content)
	assert.Equal(t, "workflow-json", content.Content)
	assert.Equal(t, "copied!", content.Description)
}

func TestCopyContent_EmptyStdoutNotApplicable(t *testing.T) {
	env := newCopyTestRunner(t, 1, map[string]string{"IF_COPY_HELPER_STDOUT": "\n"})

	content, err := env.runner.CopyContent(context.Background(), scalar.ToID("img:1"), "test.png")
	require.NoError(t, err)
	assert.Nil(t, content, "空 stdout 应视为脚本声明不适用")
}

func TestCopyContent_NonZeroExitReturnsErrorWithStderr(t *testing.T) {
	env := newCopyTestRunner(t, 1, map[string]string{
		"IF_COPY_HELPER_STDERR":   "boom",
		"IF_COPY_HELPER_EXIT_CODE": "3",
	})

	content, err := env.runner.CopyContent(context.Background(), scalar.ToID("img:1"), "test.png")
	require.Error(t, err)
	assert.Nil(t, content)
	assert.Contains(t, err.Error(), "boom")
}

func TestCopyContent_InvalidJSONProtocolError(t *testing.T) {
	env := newCopyTestRunner(t, 1, map[string]string{"IF_COPY_HELPER_STDOUT": "not json at all"})

	content, err := env.runner.CopyContent(context.Background(), scalar.ToID("img:1"), "test.png")
	require.Error(t, err)
	assert.Nil(t, content)
	assert.Contains(t, err.Error(), "not json at all")
}

func TestCopyContent_EmptyContentProtocolError(t *testing.T) {
	envelope := `{"content":"","description":"empty"}`
	env := newCopyTestRunner(t, 1, map[string]string{"IF_COPY_HELPER_STDOUT": envelope})

	content, err := env.runner.CopyContent(context.Background(), scalar.ToID("img:1"), "test.png")
	require.Error(t, err)
	assert.Nil(t, content, "信封 content 为空属于协议违约")
}

func TestCopyContent_BaseEnvContract(t *testing.T) {
	// 契约模式：假脚本把注入的环境变量回显进 content
	env := newCopyTestRunner(t, 1, map[string]string{"IF_COPY_HELPER_ECHO_ENV": "1", "IF_COPY_CUSTOM_ENV": "custom-value"})

	imgID := scalar.ToID("img:test-1")
	content, err := env.runner.CopyContent(context.Background(), imgID, "sub/test.png")
	require.NoError(t, err)
	require.NotNil(t, content)

	var echo struct {
		Trigger    string `json:"trigger"`
		ImageIDs   string `json:"imageIDs"`
		ImagePaths string `json:"imagePaths"`
		Custom     string `json:"custom"`
	}
	require.NoError(t, json.Unmarshal([]byte(content.Content), &echo))
	assert.Equal(t, "image_copy", echo.Trigger)
	// 环境变量值为 JSON 字符串数组，需二次解析
	var imageIDs []string
	require.NoError(t, json.Unmarshal([]byte(echo.ImageIDs), &imageIDs))
	assert.Contains(t, imageIDs, "img:test-1")
	var imagePaths []string
	require.NoError(t, json.Unmarshal([]byte(echo.ImagePaths), &imagePaths))
	assert.Equal(t,
		[]string{filepath.Join(env.tempDir, "sub", "test.png")},
		imagePaths)
	assert.Equal(t, "custom-value", echo.Custom)
}
