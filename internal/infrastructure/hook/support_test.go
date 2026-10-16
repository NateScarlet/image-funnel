package hook

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"main/internal/domain/device"
	"main/internal/domain/directory"
	domimage "main/internal/domain/image"
	"main/internal/domain/relpath"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// #region 依赖替身

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

// mockMetadataUpdatedSub 把发布的事件投递给每个订阅者
type mockMetadataUpdatedSub struct {
	mu          sync.Mutex
	subscribers []chan *shared.MetadataUpdatedEvent
}

func (m *mockMetadataUpdatedSub) Subscribe(ctx context.Context) iter.Seq2[*shared.MetadataUpdatedEvent, error] {
	ch := make(chan *shared.MetadataUpdatedEvent, 10)
	m.mu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.mu.Unlock()
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
	m.mu.Lock()
	subs := make([]chan *shared.MetadataUpdatedEvent, len(m.subscribers))
	copy(subs, m.subscribers)
	m.mu.Unlock()
	for _, ch := range subs {
		ch <- ev
	}
	return nil
}

// mockFileChangedSub 空实现：文件变更事件的测试直接调用笔记触发路径，不经事件总线
type mockFileChangedSub struct{}

func (m *mockFileChangedSub) Subscribe(ctx context.Context) iter.Seq2[*directory.FileChangedEvent, error] {
	return func(yield func(*directory.FileChangedEvent, error) bool) {}
}

func (m *mockFileChangedSub) Publish(ctx context.Context, ev *directory.FileChangedEvent, opts ...pubsub.PublishOption) error {
	return nil
}

// mockImageRepository 模拟图片仓库
type mockImageRepository struct {
	images []*domimage.Image
}

func (m *mockImageRepository) Get(ctx context.Context, relPath relpath.Path) (*domimage.Image, error) {
	for _, img := range m.images {
		if img.RelPath() == relPath {
			return img, nil
		}
	}
	return nil, os.ErrNotExist
}

func (m *mockImageRepository) Find(ctx context.Context, relPath relpath.Path) iter.Seq2[*domimage.Image, error] {
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
	dirs map[relpath.Path]*directory.Directory
}

func (m *mockDirectoryRepository) Get(ctx context.Context, relPath relpath.Path) (*directory.Directory, error) {
	d, ok := m.dirs[relPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	return d, nil
}

func (m *mockDirectoryRepository) Find(ctx context.Context, relPath relpath.Path) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {}
}

func (m *mockDirectoryRepository) ReadState(ctx context.Context, relPath relpath.Path) (*shared.DirectoryStateDTO, error) {
	return nil, os.ErrNotExist
}

func (m *mockDirectoryRepository) WriteState(ctx context.Context, relPath relpath.Path, state *shared.DirectoryStateDTO) error {
	return nil
}

// sentNotification 一次通知的入参快照
type sentNotification struct {
	Tag     string
	Channel string
	Title   string
	Opts    *shared.SendNotificationOptions
}

// mockNotificationSender 记录所有发出的通知
type mockNotificationSender struct {
	mu            sync.Mutex
	notifications []sentNotification
	sendErr       error
}

func (m *mockNotificationSender) SendNotification(
	ctx context.Context,
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
	tag := options.Tag()
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

// #endregion

// #region Runner 测试环境

// testHooksOptions 测试环境的可调项，全部留空即为「根目录 + 空仓库 + 真实进程原语」的最小组合
type testHooksOptions struct {
	// Hooks 写入 hooksDir 的配置文件，键为文件名、值为 TOML 原文
	Hooks map[string]string
	// Dirs 目录仓库收录的目录，缺省只有根目录
	Dirs map[relpath.Path]*directory.Directory
	// Images 图片仓库收录的图片的根相对路径
	ImageRelPaths []string
	// GraphQLURL 注入脚本的本机 GraphQL 地址
	GraphQLURL string
	// Notifier 通知替身，缺省新建一个
	Notifier *mockNotificationSender
	// Seam 按需覆盖 seam 实现（防抖时长、补全超时、进程原语替身等）
	Seam func(*seams)
}

// testHooks 一套测试用 Runner 及其外部可观测面
type testHooks struct {
	t        *testing.T
	rootDir  string
	hooksDir string
	config   configSource
	dirRepo  *mockDirectoryRepository
	notifier *mockNotificationSender
	seams    seams
	runner   *Runner
}

// newTestHooks 构造测试用 Runner：依赖与 seam 都按具名字段给定，
// 测试因此无需按位置摆放构造参数，也无需直接改写 Runner 的私有字段。
func newTestHooks(t *testing.T, opts testHooksOptions) *testHooks {
	t.Helper()

	rootDir := t.TempDir()
	hooksDir := filepath.Join(rootDir, ".image-funnel", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	for name, content := range opts.Hooks {
		require.NoError(t, os.WriteFile(filepath.Join(hooksDir, name), []byte(content), 0644))
	}

	dirs := opts.Dirs
	if dirs == nil {
		dirs = map[relpath.Path]*directory.Directory{
			relpath.Path{}: directory.FromRepository(relpath.Path{}),
		}
	}
	dirRepo := &mockDirectoryRepository{dirs: dirs}

	notifier := opts.Notifier
	if notifier == nil {
		notifier = &mockNotificationSender{}
	}

	graphqlURL := opts.GraphQLURL
	if graphqlURL == "" {
		graphqlURL = "http://127.0.0.1:0/graphql"
	}

	var images []*domimage.Image
	for _, relPath := range opts.ImageRelPaths {
		images = append(images, domimage.New(
			scalar.ToID("img:"+relPath), relPath, testutil.RelIn(t, rootDir, relPath),
			scalar.ToID("dir:1"), 100, time.Now(), nil, 0, 0,
		))
	}

	deps := Deps{
		RootDir:            rootDir,
		HooksDir:           hooksDir,
		DataDir:            rootDir,
		GraphQLURL:         graphqlURL,
		Logger:             zap.NewNop(),
		MetadataUpdatedSub: &mockMetadataUpdatedSub{},
		FileChangedSub:     &mockFileChangedSub{},
		TokenSource:        &mockTokenSource{},
		ImageRepo:          &mockImageRepository{images: images},
		DirRepo:            dirRepo,
		NotificationSender: notifier,
	}

	sm := defaultSeams(deps)
	if opts.Seam != nil {
		opts.Seam(&sm)
	}

	runner := newRunner(deps, sm)
	t.Cleanup(runner.Close)

	return &testHooks{
		t:        t,
		rootDir:  rootDir,
		hooksDir: hooksDir,
		config:   sm.Config,
		dirRepo:  dirRepo,
		notifier: notifier,
		seams:    sm,
		runner:   runner,
	}
}

// writeHook 追加一个配置文件，可用于验证「改 toml 即时生效」
func (h *testHooks) writeHook(filename string, content string) {
	h.t.Helper()
	require.NoError(h.t, os.WriteFile(filepath.Join(h.hooksDir, filename), []byte(content), 0644))
}

// waitFlagFile 轮询等待文件出现
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

// waitFlagContent 轮询等待文件出现且内容非空。
// shell 的重定向会先创建文件再刷出内容，只等文件出现会读到空内容。
func waitFlagContent(t *testing.T, path string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return string(data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待文件内容超时: %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// hookTOML 生成一条钩子配置。command 用 TOML 字面量字符串包裹，避免 Windows 路径中的
// 反斜杠被当作转义序列；extra 为追加的触发器/指令小节。
func hookTOML(id, name, command, extra string) string {
	if extra != "" {
		extra = "\n" + extra
	}
	return fmt.Sprintf("id = %q\nname = %q\ncommand = '''%s'''%s\n", id, name, command, extra)
}

// noteHookTOML 生成一条声明了笔记指令的钩子配置
func noteHookTOML(id, command, directiveName, onSuccess, onFail, extra string) string {
	return hookTOML(id, id, command, fmt.Sprintf("[directive]\nname = %q\non_success_action = %q\non_fail_action = %q\n%s",
		directiveName, onSuccess, onFail, extra))
}

// shellRedirect 生成一条「由 shell 执行、把 body 的输出写入 flag 文件」的钩子命令。
// 钩子命令不经 shell 解析，需要重定向或退出码时必须显式调用 shell。
func shellRedirect(body string, flagFile string) string {
	if filepath.Separator == '/' {
		return `sh -c '` + body + ` > ` + flagFile + `'`
	}
	return `cmd /c "` + body + ` > ` + flagFile + `"`
}

// shellPrint 生成一条「由 shell 执行并打印 body」的钩子命令
func shellPrint(body string) string {
	if filepath.Separator == '/' {
		return `sh -c '` + body + `'`
	}
	return `cmd /c "` + body + `"`
}

// shellKeepThenPrint 生成一条钩子命令：先声明 KEEP（本次未产生结果），再打印一行输出。
// 用于覆盖「脚本覆盖 KEEP 但仍有输出」的通知分支
func shellKeepThenPrint(printed string) string {
	if filepath.Separator == '/' {
		return `sh -c 'echo KEEP > "$IMAGE_FUNNEL_ACTION" && echo ` + printed + `'`
	}
	return `cmd /c "echo KEEP > %IMAGE_FUNNEL_ACTION% && echo ` + printed + `"`
}

func assertNoFile(t *testing.T, path string, msg string) {
	t.Helper()
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), msg)
}

// #endregion
