package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"main/internal/domain/device"
	"main/internal/domain/directory"
	domimage "main/internal/domain/image"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// mockToken 模拟 device.Token
type mockToken struct{}

func (m *mockToken) String() string     { return "mock-token" }
func (m *mockToken) UserID() scalar.ID  { return scalar.ToID("usr:mock") }
func (m *mockToken) Expire() time.Time  { return time.Now().Add(time.Hour) }
func (m *mockToken) IssueAt() time.Time { return time.Now() }
func (m *mockToken) JTI() string        { return "mock-jti" }

// mockTokenSource 模拟 device.TokenSource
type mockTokenSource struct{}

func (m *mockTokenSource) NewAccessToken(ctx context.Context, deviceID scalar.ID) (device.Token, error) {
	return &mockToken{}, nil
}
func (m *mockTokenSource) NewRefreshToken(ctx context.Context, deviceID scalar.ID) (device.Token, error) {
	return &mockToken{}, nil
}
func (m *mockTokenSource) VerifyAccessToken(ctx context.Context, rawToken string) (device.Token, error) {
	return &mockToken{}, nil
}
func (m *mockTokenSource) VerifyRefreshToken(ctx context.Context, rawToken string) (device.Token, error) {
	return &mockToken{}, nil
}

type mockMetadataUpdatedSub struct {
	subscribers []chan *shared.MetadataUpdatedEvent
}

func (m *mockMetadataUpdatedSub) Subscribe(ctx context.Context) iter.Seq2[*shared.MetadataUpdatedEvent, error] {
	ch := make(chan *shared.MetadataUpdatedEvent, 10)
	m.subscribers = append(m.subscribers, ch)
	return func(yield func(*shared.MetadataUpdatedEvent, error) bool) {
		for {
			select {
			case ev := <-ch:
				if !yield(ev, nil) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

func (m *mockMetadataUpdatedSub) Publish(ctx context.Context, ev *shared.MetadataUpdatedEvent, opts ...pubsub.PublishOption) error {
	for _, ch := range m.subscribers {
		ch <- ev
	}
	return nil
}

type mockFileChangedSub struct{}

func (m *mockFileChangedSub) Subscribe(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error] {
	return func(yield func(*shared.FileChangedEvent, error) bool) {}
}

func (m *mockFileChangedSub) Publish(ctx context.Context, ev *shared.FileChangedEvent, opts ...pubsub.PublishOption) error {
	return nil
}

type mockImageRepository struct {
	images []*domimage.Image
}

func (m *mockImageRepository) Get(ctx context.Context, relPath string) (*domimage.Image, error) {
	for _, img := range m.images {
		if img.RelPath() == relPath {
			return img, nil
		}
	}
	return nil, os.ErrNotExist
}

func (m *mockImageRepository) Find(ctx context.Context, relPath string) iter.Seq2[*domimage.Image, error] {
	return func(yield func(*domimage.Image, error) bool) {
		for _, img := range m.images {
			if !yield(img, nil) {
				return
			}
		}
	}
}

// mockDirectoryRepository 模拟目录仓库
type mockDirectoryRepository struct {
	dirs map[string]*directory.Directory
}

func (m *mockDirectoryRepository) Get(ctx context.Context, relPath string) (*directory.Directory, error) {
	if m.dirs == nil {
		return nil, os.ErrNotExist
	}
	d, ok := m.dirs[relPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	return d, nil
}

func (m *mockDirectoryRepository) Find(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {}
}

func (m *mockDirectoryRepository) ReadState(ctx context.Context, relPath string) (*shared.DirectoryStateDTO, error) {
	return nil, os.ErrNotExist
}

func (m *mockDirectoryRepository) WriteState(ctx context.Context, relPath string, state *shared.DirectoryStateDTO) error {
	return nil
}

// mockNotificationSender 模拟 NotificationSender
type mockNotificationSender struct {
	mu            sync.Mutex
	notifications []sentNotification
	sendErr       error
}

type sentNotification struct {
	Tag     string
	Channel string
	Title   string
	Opts    *shared.SendNotificationOptions
}

func (m *mockNotificationSender) SendNotification(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	opts ...shared.SendNotificationOption,
) (*shared.SendNotificationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	options := shared.NewSendNotificationOptions(opts...)
	m.notifications = append(m.notifications, sentNotification{
		Tag:     tag,
		Channel: channel,
		Title:   title,
		Opts:    options,
	})
	return shared.NewSendNotificationResult(scalar.ToID("notify:"+tag), true), nil
}

func (m *mockNotificationSender) Notifications() []sentNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]sentNotification, len(m.notifications))
	copy(copied, m.notifications)
	return copied
}

func TestRunner_TOML_Parsing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	tomlContent := `
id = "comfyui-test"
name = "ComfyUI 测试"
description = "ComfyUI 测试说明"
command = "python test.py"

[on.post_update_image_metadata]
rating = [4, 5]
label = [""]

[on.image_dispatch]
`
	tomlPath := filepath.Join(hooksDir, "comfyui.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, nil, &mockNotificationSender{})
	defer runner.Close()

	hooks, err := runner.List(context.Background())
	assert.NoError(t, err)
	assert.Len(t, hooks, 1)

	configs, err := runner.loadHooks()
	assert.NoError(t, err)
	assert.Len(t, configs, 1)

	h := hooks[0]
	assert.Equal(t, "hk:comfyui-test", h.ID().String())
	assert.Equal(t, "ComfyUI 测试", h.Name())
	assert.True(t, h.CanDispatchByImage())
}

func TestRunner_FilteringAndDebounce(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-exec-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "flag_dir")
	cmdStr := "echo 1 > " + flagFile

	tomlContent := `
id = "exec-test"
name = "执行测试"
command = '` + cmdStr + `'

[on.post_update_image_metadata]
rating = [4]
label = [""]
`
	tomlPath := filepath.Join(hooksDir, "exec.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewExample()

	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, &mockNotificationSender{})
	runner.debouncer.duration = 10 * time.Millisecond // 10ms 防抖
	defer runner.Close()

	// 1. 自动触发：测试发送不符合过滤条件的事件（3星）
	ebus.Publish(context.Background(), &shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(tempDir, "a.png"),
		Rating:    3,
		Label:     "",
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	})

	time.Sleep(50 * time.Millisecond) // 等待防抖
	_, err = os.Stat(flagFile)
	assert.True(t, os.IsNotExist(err), "3星评级不应触发钩子运行")

	// 2. 自动触发：测试发送符合过滤条件的事件（4星）且标签为 ""
	ebus.Publish(context.Background(), &shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(tempDir, "a.png"),
		Rating:    4,
		Label:     "",
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	})

	assert.True(t, waitFlagFile(flagFile, 15*time.Second), "4星评级应触发钩子成功运行")
	time.Sleep(50 * time.Millisecond) // 等待进程执行和日志打印完全收尾

	// 清理标志文件
	err = os.RemoveAll(flagFile)
	assert.NoError(t, err)

	// 3. 自动触发：测试发送符合 4星但标签为 Red（不满足 label = [""] 的过滤）
	ebus.Publish(context.Background(), &shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(tempDir, "a.png"),
		Rating:    4,
		Label:     "Red",
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	})

	time.Sleep(50 * time.Millisecond)
	_, err = os.Stat(flagFile)
	assert.True(t, os.IsNotExist(err), "由于只匹配空标签，标签为 Red 的图片应该不匹配")
}

func waitFlagFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestRunner_Env_Injection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-env-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	tomlContent := `
id = "env-test"
name = "环境变量测试"
command = "python test_env.py"

[env]
TEST_VAR_ONE = "val1"
TEST_VAR_TWO = "val2"
`
	tomlPath := filepath.Join(hooksDir, "env.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, nil, &mockNotificationSender{})
	defer runner.Close()

	configs, err := runner.loadHooks()
	assert.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, "val1", configs[0].Env["TEST_VAR_ONE"])
	assert.Equal(t, "val2", configs[0].Env["TEST_VAR_TWO"])
}

func TestRunner_Trigger_SyncAndError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-trigger-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	// 1. 测试成功的外部钩子命令
	successToml := `
id = "success-test"
name = "成功测试"
command = "echo hello"

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "success.toml"), []byte(successToml), 0644)
	assert.NoError(t, err)

	// 2. 测试失败的外部钩子命令（会返回错误，并且 stderr 中有报错）
	failToml := `
id = "fail-test"
name = "失败测试"
command = "echo test_error_out >&2 && exit 42"

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "fail.toml"), []byte(failToml), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()

	// 构建 mock 目录仓库，使得路径能推导出目录信息
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, &mockNotificationSender{})
	defer runner.Close()

	// 使用绝对路径，使得 dirRelFromAbsPath 能正确推导目录信息
	absPath := filepath.Join(tempDir, "a.png")

	// 触发成功钩子，应该同步等待并返回 nil
	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:success-test"), "image_dispatch")
	assert.NoError(t, err)

	// 触发失败钩子，应该同步等待并返回错误，且错误信息中包含退出码和 stderr 输出
	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:fail-test"), "image_dispatch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test_error_out")
	assert.Contains(t, err.Error(), "exit status 42")
}

func TestRunner_DirectoryEnvVars_Injection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-direnv-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "dir_env_flag")

	// 使用 shell 命令输出目录环境变量到文件，使用 | 作为分隔符避免与目录 ID 中的 : 冲突
	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `echo "$IMAGE_FUNNEL_DIRECTORY_ID|$IMAGE_FUNNEL_DIRECTORY_REL_PATH" > ` + flagFile
	} else {
		cmdStr = `echo %IMAGE_FUNNEL_DIRECTORY_ID%^|%IMAGE_FUNNEL_DIRECTORY_REL_PATH% > ` + flagFile
	}

	tomlContent := `
id = "direnv-test"
name = "目录环境变量测试"
command = '` + cmdStr + `'

[on.post_update_image_metadata]
rating = [4]
label = [""]
`
	err = os.WriteFile(filepath.Join(hooksDir, "direnv.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewExample()

	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}
	expectedDir := mockDirRepo.dirs["."]

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, &mockNotificationSender{})
	runner.debouncer.duration = 10 * time.Millisecond
	defer runner.Close()

	// 等待监听器就绪
	time.Sleep(10 * time.Millisecond)

	// 发布匹配过滤条件的事件，触发 post_update_image_metadata 钩子
	ebus.Publish(context.Background(), &shared.MetadataUpdatedEvent{
		ID:        scalar.ToID("img:1:a.png"),
		Path:      filepath.Join(tempDir, "a.png"),
		Rating:    4,
		Label:     "",
		Action:    "keep",
		OldRating: 0,
		OldLabel:  "",
		OldAction: "",
	})

	assert.True(t, waitFlagFile(flagFile, 500*time.Millisecond), "应触发钩子并写入 flag 文件")

	// 读取 flag 文件内容验证环境变量
	contentBytes, err := os.ReadFile(flagFile)
	assert.NoError(t, err)
	result := strings.TrimSpace(string(contentBytes))

	// 使用 | 作为分隔符避免与 ID 中的 : 冲突
	parts := strings.SplitN(result, "|", 2)
	assert.Len(t, parts, 2, "输出应为 DIRECTORY_ID|DIRECTORY_REL_PATH 格式")
	assert.Equal(t, expectedDir.ID().String(), parts[0], "DIRECTORY_ID 应与目录仓库返回的 ID 一致")
}

func TestRunner_Trigger_DirectoryEnvVars(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-trigger-direnv-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "trigger_dir_env_flag")

	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `echo "$IMAGE_FUNNEL_DIRECTORY_ID|$IMAGE_FUNNEL_DIRECTORY_REL_PATH" > ` + flagFile
	} else {
		cmdStr = `echo %IMAGE_FUNNEL_DIRECTORY_ID%^|%IMAGE_FUNNEL_DIRECTORY_REL_PATH% > ` + flagFile
	}

	tomlContent := `
id = "trigger-direnv-test"
name = "触发目录环境变量测试"
command = '` + cmdStr + `'

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "trigger-direnv.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewExample()

	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			"subdir": directory.FromRepository("subdir"),
		},
	}
	expectedDir := mockDirRepo.dirs["subdir"]

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, &mockNotificationSender{})
	defer runner.Close()

	// 创建一个位于子目录中的文件，确保目录信息可推导
	subDir := filepath.Join(tempDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	assert.NoError(t, err)
	absPath := filepath.Join(subDir, "test.png")

	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:trigger-direnv-test"), "image_dispatch")
	assert.NoError(t, err)

	assert.True(t, waitFlagFile(flagFile, 500*time.Millisecond), "Trigger 应成功执行钩子并写入 flag 文件")

	contentBytes, err := os.ReadFile(flagFile)
	assert.NoError(t, err)
	result := strings.TrimSpace(string(contentBytes))

	parts := strings.SplitN(result, "|", 2)
	assert.Len(t, parts, 2, "输出应为 DIRECTORY_ID|DIRECTORY_REL_PATH 格式")
	assert.Equal(t, expectedDir.ID().String(), parts[0], "DIRECTORY_ID 应与目录仓库返回的 ID 一致")
	assert.Equal(t, "subdir", parts[1], "DIRECTORY_REL_PATH 应为 subdir")
}

func TestRunner_Trigger_DirectoryResolveFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-dir-fail-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "should_not_exist")
	tomlContent := `
id = "dir-fail-test"
name = "目录解析失败测试"
command = 'echo "should not run" > ` + flagFile + `'

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "dir-fail.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()

	// 目录仓库为空，任何路径都无法解析
	mockDirRepo := &mockDirectoryRepository{dirs: map[string]*directory.Directory{}}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, &mockNotificationSender{})
	defer runner.Close()

	absPath := filepath.Join(tempDir, "unknown_subdir", "a.png")
	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:dir-fail-test"), "image_dispatch")
	assert.Error(t, err, "目录解析失败时应返回错误")
	assert.Contains(t, err.Error(), "failed to resolve directory")

	// 验证钩子没有被执行
	_, err = os.Stat(flagFile)
	assert.True(t, os.IsNotExist(err), "目录解析失败时不应执行钩子")
}

func TestRunner_NoDirective_PostUpdateNote(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "no-dir-update-note-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "update_flag")
	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `echo "$IMAGE_FUNNEL_NOTE_PATHS" > ` + flagFile
	} else {
		cmdStr = `echo %IMAGE_FUNNEL_NOTE_PATHS% > ` + flagFile
	}

	tomlContent := `
id = "no-dir-update"
name = "无指令更新测试"
command = '` + cmdStr + `'

[on.post_update_note]
`
	err = os.WriteFile(filepath.Join(hooksDir, "no-dir-update.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()

	imgID := scalar.ToID("img:1")
	img := domimage.New(imgID, "test.png", "test.png", scalar.ToID("dir:1"), 100, time.Now(), nil, 0, 0)
	imgRepo := &mockImageRepository{images: []*domimage.Image{img}}
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, imgRepo, nil, mockDirRepo, &mockNotificationSender{})
	defer runner.Close()

	noteRelPath := "test.png.md"
	noteAbsPath := filepath.Join(tempDir, noteRelPath)
	err = os.WriteFile(noteAbsPath, []byte("Just a normal note with no slash commands"), 0644)
	assert.NoError(t, err)

	runner.handleFileChanged(&shared.FileChangedEvent{
		DirectoryID: scalar.ToID("dir:1"),
		RelPath:     noteRelPath,
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	})

	assert.True(t, waitFlagFile(flagFile, 1500*time.Millisecond), "应该无指令直接触发并创建 flag 文件")
	contentBytes, err := os.ReadFile(flagFile)
	assert.NoError(t, err)

	result := strings.TrimSpace(string(contentBytes))
	var mPaths []string
	err = json.Unmarshal([]byte(result), &mPaths)
	assert.NoError(t, err)
	assert.Len(t, mPaths, 1)
	assert.Equal(t, noteAbsPath, mPaths[0], "环境变量 IMAGE_FUNNEL_NOTE_PATHS 应该注入正确")
}

func TestRunner_NoDirective_PostCommitSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "no-dir-commit-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagPure := filepath.Join(tempDir, "pure_flag")
	flagScan := filepath.Join(tempDir, "scan_flag")

	var cmdPure, cmdScan string
	if filepath.Separator == '/' {
		cmdPure = `echo "pure" > ` + flagPure
		cmdScan = `echo "$IMAGE_FUNNEL_NOTE_PATHS" > ` + flagScan
	} else {
		cmdPure = `echo pure > ` + flagPure
		cmdScan = `echo %IMAGE_FUNNEL_NOTE_PATHS% > ` + flagScan
	}

	tomlPure := `
id = "pure-commit"
name = "纯会话提交测试"
command = '` + cmdPure + `'

[on.post_commit_session]
`
	err = os.WriteFile(filepath.Join(hooksDir, "pure.toml"), []byte(tomlPure), 0644)
	assert.NoError(t, err)

	tomlScan := `
id = "scan-commit"
name = "笔记扫描提交测试"
command = '` + cmdScan + `'

[on.post_commit_session.note_scan]
`
	err = os.WriteFile(filepath.Join(hooksDir, "scan.toml"), []byte(tomlScan), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()

	imgID := scalar.ToID("img:1")
	img := domimage.New(imgID, "test.png", "test.png", scalar.ToID("dir:1"), 100, time.Now(), nil, 0, 0)
	imgRepo := &mockImageRepository{images: []*domimage.Image{img}}
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, imgRepo, nil, mockDirRepo, &mockNotificationSender{})
	defer runner.Close()

	// 等待...

	err = os.WriteFile(filepath.Join(tempDir, "test.png.md"), []byte("Hello world note"), 0644)
	assert.NoError(t, err)

	err = runner.OnCommitSession(context.Background(), ".")
	assert.NoError(t, err)

	assert.True(t, waitFlagFile(flagPure, 1500*time.Millisecond), "纯提交钩子应成功执行")
	assert.True(t, waitFlagFile(flagScan, 1500*time.Millisecond), "无指令扫描提交钩子应成功执行")

	contentBytes, err := os.ReadFile(flagScan)
	assert.NoError(t, err)
	result := strings.TrimSpace(string(contentBytes))
	var mPaths []string
	err = json.Unmarshal([]byte(result), &mPaths)
	assert.NoError(t, err)
	assert.Len(t, mPaths, 1)
	assert.Equal(t, filepath.Join(tempDir, "test.png.md"), mPaths[0], "无指令笔记扫描钩子的 NotePaths 应正确携带")
}

// TestRunner_PostCommitSession_DirectoryID 验证 post_commit_session 纯提交钩子
// 在没有图片路径和笔记路径时，IMAGE_FUNNEL_DIRECTORY_ID 仍能正确注入
func TestRunner_PostCommitSession_DirectoryID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "post-commit-dir-id-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	flagFile := filepath.Join(tempDir, "dir_id_flag")

	var cmdStr string
	if filepath.Separator == '/' {
		cmdStr = `echo "$IMAGE_FUNNEL_DIRECTORY_ID" > ` + flagFile
	} else {
		cmdStr = `echo %IMAGE_FUNNEL_DIRECTORY_ID% > ` + flagFile
	}

	tomlContent := `
id = "post-commit-dir-id"
name = "提交时目录ID注入测试"
command = '` + cmdStr + `'

[on.post_commit_session]
`
	err = os.WriteFile(filepath.Join(hooksDir, "dirid.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()

	expectedDir := directory.FromRepository("subdir")
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			"subdir": expectedDir,
		},
	}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, &mockNotificationSender{})
	defer runner.Close()

	err = runner.OnCommitSession(context.Background(), "subdir")
	assert.NoError(t, err)

	assert.True(t, waitFlagFile(flagFile, 1500*time.Millisecond), "纯提交钩子应成功执行并写入 flag 文件")

	contentBytes, err := os.ReadFile(flagFile)
	assert.NoError(t, err)
	result := strings.TrimSpace(string(contentBytes))

	// 验证回退机制：即使没有图片路径或笔记路径，DIRECTORY_ID 也能正确注入
	assert.Equal(t, expectedDir.ID().String(), result,
		"post_commit_session 纯提交钩子应正确注入 IMAGE_FUNNEL_DIRECTORY_ID（回退到调用者传入的目录信息）")
}

func TestSplitArgs_Simple(t *testing.T) {
	assert.Equal(t, []string{"arg1", "arg2", "arg3"}, splitArgs("arg1 arg2 arg3"))
}

func TestSplitArgs_SingleArg(t *testing.T) {
	assert.Equal(t, []string{"hello"}, splitArgs("hello"))
}

func TestSplitArgs_Empty(t *testing.T) {
	assert.Empty(t, splitArgs(""))
}

func TestSplitArgs_OnlyWhitespace(t *testing.T) {
	assert.Empty(t, splitArgs("   \t  "))
}

func TestSplitArgs_QuotedSimple(t *testing.T) {
	assert.Equal(t, []string{"hello world"}, splitArgs(`"hello world"`))
}

func TestSplitArgs_QuotedMixed(t *testing.T) {
	assert.Equal(t, []string{"arg1", "hello world", "arg3"}, splitArgs(`arg1 "hello world" arg3`))
}

func TestSplitArgs_MultipleQuoted(t *testing.T) {
	assert.Equal(t, []string{"first", "second"}, splitArgs(`"first" "second"`))
}

func TestSplitArgs_QuotedAtStart(t *testing.T) {
	assert.Equal(t, []string{"hello world", "arg2"}, splitArgs(`"hello world" arg2`))
}

func TestSplitArgs_QuotedAtEnd(t *testing.T) {
	assert.Equal(t, []string{"arg1", "hello world"}, splitArgs(`arg1 "hello world"`))
}

func TestSplitArgs_NestedQuotes(t *testing.T) {
	// 引号内的内容原样保留，不支持转义
	assert.Equal(t, []string{`it's`}, splitArgs(`"it's"`))
}

func TestSplitArgs_TabSeparated(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, splitArgs("a\tb\tc"))
}

func TestSplitArgs_QuotedWithTabInside(t *testing.T) {
	// 引号内的制表符原样保留
	assert.Equal(t, []string{"a\tb"}, splitArgs("\"a\tb\""))
}

func TestRunner_ExecuteNoteDirectives_PartialFailures(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-note-partial-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	// 1. 成功钩子定义，指令为 "fork"
	forkToml := `
id = "fork-test"
name = "fork-test"
command = "echo fork success"

[directive]
name = "fork"
on_success_action = "REMOVE"
on_fail_action = "KEEP"

[on.post_update_note]
`
	err = os.WriteFile(filepath.Join(hooksDir, "fork.toml"), []byte(forkToml), 0644)
	assert.NoError(t, err)

	// 2. 失败钩子定义，指令为 "comfyui"
	comfyuiToml := `
id = "comfyui-test"
name = "comfyui-test"
command = "echo fail_output >&2 && exit 1"

[directive]
name = "comfyui"
on_success_action = "REMOVE"
on_fail_action = "KEEP"

[on.post_update_note]
`
	err = os.WriteFile(filepath.Join(hooksDir, "comfyui.toml"), []byte(comfyuiToml), 0644)
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

	// 准备笔记文件
	noteRelPath := "test_note.md"
	noteAbsPath := filepath.Join(tempDir, noteRelPath)
	initialContent := `Some text.
/fork a
/comfyui b
Other text.`
	err = os.WriteFile(noteAbsPath, []byte(initialContent), 0644)
	assert.NoError(t, err)

	// 运行指令处理
	ok, err := runner.executeNoteDirectives(context.Background(), directory.FromRepository("."), noteRelPath, initialContent, "post_update_note", scalar.ID{})
	assert.Error(t, err)
	assert.True(t, ok)

	// 检查同步清理后的内容：
	// fork 成功应被 REMOVE 变成空行（正则替换后可能留空，或者去掉）
	// comfyui 失败应被 KEEP 保留
	contentBytes, err := os.ReadFile(noteAbsPath)
	assert.NoError(t, err)
	content := string(contentBytes)

	// 期望 fork 行被移除，comfyui 行被保留，并且 run-id 标签已擦除
	assert.NotContains(t, content, "/fork")
	assert.Contains(t, content, "/comfyui b")
	assert.NotContains(t, content, "hook-run-id")

	// 3. 测试迟到清理的分支：
	// 写回一个带未清理指令和 run-id 的文本，模拟迟到写入
	runID := "run_test_12345"
	lateContent := fmt.Sprintf("---\nhook-run-id: %s\n---\nSome text.\n/fork a\n/comfyui b\nOther text.", runID)
	err = os.WriteFile(noteAbsPath, []byte(lateContent), 0644)
	assert.NoError(t, err)

	// 在 runner.activeTasks 中预设这个 runID
	runner.muTasks.Lock()
	runner.activeTasks[runID] = &activeTask{
		phase: phaseAfter3,
		paths: map[string]struct{}{noteAbsPath: {}},
		failedDirectives: map[string]bool{
			"fork":    false, // 成功
			"comfyui": true,  // 失败
		},
	}
	runner.muTasks.Unlock()

	// 获取 hookMap
	hooks, err := runner.loadHooks()
	assert.NoError(t, err)
	hookMap := make(map[string]hookConfig)
	for _, h := range hooks {
		if h.Directive != nil && h.Directive.Name != "" {
			hookMap[h.Directive.Name] = h
		}
	}

	// 触发 late cleanup
	err = runner.postProcessNoteDirectives(context.Background(), noteAbsPath, runID, "post_update_note", hookMap, map[string]bool{
		"fork":    false,
		"comfyui": true,
	})
	assert.NoError(t, err)

	// 再次检查迟到清理后的文件内容
	contentBytes, err = os.ReadFile(noteAbsPath)
	assert.NoError(t, err)
	content = string(contentBytes)

	assert.NotContains(t, content, "/fork")
	assert.Contains(t, content, "/comfyui b")
	assert.NotContains(t, content, "hook-run-id")
}

func TestRunner_loadHooks_StrictParsing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-strict-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	// 写一个包含未知 Key（拼写错误）的 TOML 配置文件
	invalidToml := `
id = "strict-test"
name = "strict-test"
command = "echo success"
unknown_key_here = "oops"

[directive]
name = "strict"
`
	err = os.WriteFile(filepath.Join(hooksDir, "invalid.toml"), []byte(invalidToml), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, &mockImageRepository{}, nil, nil, &mockNotificationSender{})
	defer runner.Close()

	configs, err := runner.loadHooks()
	assert.NoError(t, err)

	// configs 不应该包含 strict-test，因为解析由于未知字段而报错被跳过
	for _, c := range configs {
		assert.NotEqual(t, "strict-test", c.ID)
	}
}

func TestRunner_Autocomplete_Cancel(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-auto-cancel-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	pidFile := filepath.Join(tempDir, "pid.txt")

	// 根据操作系统，指定一个能将 PID 写入文件并延迟退出的命令
	var cmdStr string
	if filepath.Separator == '\\' {
		// Windows: 使用 powershell 写入当前 PID 并休眠 5 秒
		cmdStr = fmt.Sprintf(`powershell -Command "$PID | Out-File -FilePath '%s' -Encoding utf8; Start-Sleep -Seconds 5"`, strings.ReplaceAll(pidFile, "\\", "\\\\"))
	} else {
		// Unix: 写入当前 PID 到文件并休眠 5 秒
		cmdStr = fmt.Sprintf(`sh -c "echo $$ > '%s' && sleep 5"`, pidFile)
	}

	tomlContent := fmt.Sprintf(`
id = "auto-cancel-test"
name = "自动补全取消测试"
command = "echo fallback"

[directive]
name = "test"

[directive.autocomplete]
command = '''%s'''
`, cmdStr)

	tomlPath := filepath.Join(hooksDir, "autocomplete_cancel.toml")
	err = os.WriteFile(tomlPath, []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, &mockImageRepository{}, nil, nil, &mockNotificationSender{})
	defer runner.Close()

	// 启动一个会被取消的 context
	ctx, cancel := context.WithCancel(context.Background())

	// 模拟在运行一小段时间后 cancel 掉（等子进程起来并把 PID 写入文件）
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	hookID := scalar.ToID("hk:auto-cancel-test")
	start := time.Now()
	suggestions, err := runner.Autocomplete(ctx, hookID, "", "", "")
	duration := time.Since(start)

	// 验证：
	// 1. 应该返回 context.Canceled 错误
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, suggestions)

	// 2. 应当在取消后立即返回（因为异步 Cancel），整个耗时大约在 300ms + 几毫秒内，低性能环境下也应在 15 秒内完成
	assert.Less(t, duration, 15*time.Second, "cancellation should stop execution within graceful period")

	// 3. 验证进程是否已经被真的关闭了
	pidBytes, err := os.ReadFile(pidFile)
	assert.NoError(t, err)
	pidStr := strings.TrimSpace(string(pidBytes))
	pidStr = strings.TrimPrefix(pidStr, "\ufeff") // 剔除可能存在的 UTF-8 BOM 头
	var pid int
	_, err = fmt.Sscanf(pidStr, "%d", &pid)
	assert.NoError(t, err)
	assert.Greater(t, pid, 0)

	// 等待 1.2 秒（因为优雅期为 1.0 秒，强杀是异步触发的，所以 1.2 秒后子进程树必须全部死亡）
	time.Sleep(1200 * time.Millisecond)

	// 检测这个 PID 是否已在系统上消失
	var exists bool
	if filepath.Separator == '\\' {
		// Windows: 使用 tasklist /FI "PID eq <pid>" 检测
		chkCmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
		output, err := chkCmd.Output()
		if err == nil && strings.Contains(string(output), pidStr) {
			exists = true
		}
	} else {
		// Unix: 使用 ps -p <pid> 检测
		chkCmd := exec.Command("ps", "-p", pidStr)
		if err := chkCmd.Run(); err == nil {
			exists = true
		}
	}

	assert.False(t, exists, "spawned process should be terminated after cancel")
}

func TestRunner_NotificationOnSuccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-notif-success-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	tomlContent := `
id = "notif-success-test"
name = "通知成功测试"
command = "echo hello_success"

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "success.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}
	notifSender := &mockNotificationSender{}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, notifSender)
	defer runner.Close()

	absPath := filepath.Join(tempDir, "a.png")
	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:notif-success-test"), "image_dispatch")
	assert.NoError(t, err)

	notifs := notifSender.Notifications()
	assert.Len(t, notifs, 1)
	assert.Equal(t, "hooks", notifs[0].Channel)
	assert.Equal(t, "钩子 [通知成功测试] 执行成功", notifs[0].Title)
	assert.Equal(t, shared.NotificationPriorityLow, notifs[0].Opts.Priority())
	assert.Equal(t, "hello_success", notifs[0].Opts.Body())
}

func TestRunner_NotificationOnFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-hook-notif-fail-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	err = os.MkdirAll(hooksDir, 0755)
	assert.NoError(t, err)

	tomlContent := `
id = "notif-fail-test"
name = "通知失败测试"
command = "echo stderr_err_msg >&2 && exit 1"

[on.image_dispatch]
`
	err = os.WriteFile(filepath.Join(hooksDir, "fail.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{
			".": directory.FromRepository("."),
		},
	}
	notifSender := &mockNotificationSender{}

	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, nil, nil, mockDirRepo, notifSender)
	defer runner.Close()

	absPath := filepath.Join(tempDir, "a.png")
	err = runner.Trigger(context.Background(), []string{"img:1"}, []string{absPath}, scalar.ToID("hk:notif-fail-test"), "image_dispatch")
	assert.Error(t, err)

	notifs := notifSender.Notifications()
	assert.Len(t, notifs, 1)
	assert.Equal(t, "hooks", notifs[0].Channel)
	assert.Equal(t, "钩子 [通知失败测试] 执行失败", notifs[0].Title)
	assert.Equal(t, shared.NotificationPriorityHigh, notifs[0].Opts.Priority())
	assert.Contains(t, notifs[0].Opts.Body(), "stderr_err_msg")
}
