package hook

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestImageMetadataTrigger_OnePathEndToEnd 示范只构造并验证一条触发路径：
// 配置来源（真实 toml）+ 事件筛选 + 防抖合批 + 单次执行（进程替身），全程不经 Runner。
func TestImageMetadataTrigger_OnePathEndToEnd(t *testing.T) {
	process := newBlockingProcessRunner()
	deps := newIsolatedDeps(t, map[string]string{
		"isolated.toml": hookTOML("isolated", "isolated", "uv run script.py --flag",
			"[on.post_update_image_metadata]\nrating = [4]\nlabel = [\"\"]\n"),
	}, process, nil)

	trigger := newImageMetadataTrigger(context.Background(), deps.config, deps.exec, deps.notes, 10*time.Millisecond, zap.NewNop())

	// 配置来源：真实逐次读盘，能解析出钩子
	hooks, err := deps.config.Load()
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Equal(t, "uv run script.py --flag", hooks[0].Command)

	imgAbsPath := filepath.Join(deps.rootDir, "a.png")

	// 事件筛选：rating 不匹配的事件被丢弃
	trigger.Handle(&shared.MetadataUpdatedEvent{
		ID: scalar.ToID("img:1:a.png"), Path: imgAbsPath, Rating: 3, Label: "",
	})
	// 事件筛选：label 不匹配的事件同样被丢弃
	trigger.Handle(&shared.MetadataUpdatedEvent{
		ID: scalar.ToID("img:1:a.png"), Path: imgAbsPath, Rating: 4, Label: "Red",
	})
	assert.Never(t, process.called, 200*time.Millisecond, 20*time.Millisecond,
		"不满足筛选条件的事件不应触发单次执行")

	// 事件筛选 + 防抖：命中筛选条件的事件经防抖窗口后进入单次执行
	trigger.Handle(&shared.MetadataUpdatedEvent{
		ID: scalar.ToID("img:1:a.png"), Path: imgAbsPath, Rating: 4, Label: "",
	})
	process.awaitCall(t)

	argv, env, dir := process.lastCall()
	assert.Equal(t, []string{"uv", "run", "script.py", "--flag"}, argv, "命令应按 shell 语法分词后原样交给进程原语")
	assert.Equal(t, deps.hooksDir, dir, "脚本工作目录应为 Hook 配置文件所在目录")
	assert.Equal(t, `["img:1:a.png"]`, envValue(env, "IMAGE_FUNNEL_IMAGE_IDS"))
	assert.Equal(t, mustJSON([]string{imgAbsPath}), envValue(env, "IMAGE_FUNNEL_IMAGE_PATHS"))
	assert.Equal(t, "4", envValue(env, "IMAGE_FUNNEL_IMAGE_RATING"))
	assert.Equal(t, "post_update_image_metadata", envValue(env, "IMAGE_FUNNEL_TRIGGER"))
	assert.NotEmpty(t, envValue(env, "IMAGE_FUNNEL_ACTION"), "单次执行应生成操作覆盖文件路径")
}
