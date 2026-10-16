package hook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"main/internal/domain/directory"
	"main/internal/domain/relpath"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// blockingProcessRunner 进程原语替身：记录收到的 argv/env/工作目录，并在调用方取消时
// 才返回（并按被终止的进程返回错误）。这样取消行为可在进程替身上断言，
// 无需 spawn 真实进程、也无需观察操作系统的进程表。
type blockingProcessRunner struct {
	// Output 被取消后返回给调用方的结果
	Output processOutput
	// Err 被取消后返回给调用方的错误；nil 表示按 context 的取消错误返回
	Err error

	mu       sync.Mutex
	argv     []string
	env      []string
	dir      string
	observed chan struct{} // 被取消时关闭
}

func newBlockingProcessRunner() *blockingProcessRunner {
	return &blockingProcessRunner{observed: make(chan struct{})}
}

func (p *blockingProcessRunner) Run(ctx context.Context, argv []string, env []string, dir string) (processOutput, error) {
	p.mu.Lock()
	p.argv, p.env, p.dir = argv, env, dir
	p.mu.Unlock()

	<-ctx.Done()
	close(p.observed)
	if p.Err != nil {
		return p.Output, p.Err
	}
	// 被取消的进程以取消错误收场，与真实进程被终止后的行为一致
	return p.Output, ctx.Err()
}

func (p *blockingProcessRunner) SpawnResident(ctx context.Context, argv []string, env []string, dir string) (residentProcess, error) {
	return nil, errors.New("blockingProcessRunner 只实现一次性执行")
}

func (p *blockingProcessRunner) lastCall() (argv []string, env []string, dir string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.argv, p.env, p.dir
}

// called 报告是否已有一次执行发生
func (p *blockingProcessRunner) called() bool {
	_, env, _ := p.lastCall()
	return len(env) > 0
}

// awaitCall 等待第一次执行发生
func (p *blockingProcessRunner) awaitCall(t *testing.T) {
	t.Helper()
	require.Eventually(t, p.called, 5*time.Second, 10*time.Millisecond, "任务应被交给进程原语")
}

// canceled 等待替身观察到调用方取消
func (p *blockingProcessRunner) canceled(t *testing.T) {
	t.Helper()
	select {
	case <-p.observed:
	case <-time.After(5 * time.Second):
		t.Fatal("进程替身未在预期时间内观察到取消")
	}
}

// isolatedTriggerDeps 一条触发路径所需的协作者，不经过 Runner 构造
type isolatedTriggerDeps struct {
	rootDir  string
	hooksDir string
	config   configSource
	exec     *hookExecutor
	notes    *noteContext
	notifier *mockNotificationSender
}

// newIsolatedDeps 装配单条触发路径的协作者：配置来源与进程原语都从 seam 注入，
// 测试因此无需构造完整 Runner。
func newIsolatedDeps(t *testing.T, hooks map[string]string, process processRunner, notifSender *mockNotificationSender) *isolatedTriggerDeps {
	t.Helper()

	rootDir := t.TempDir()
	hooksDir := filepath.Join(rootDir, ".image-funnel", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	for name, content := range hooks {
		require.NoError(t, os.WriteFile(filepath.Join(hooksDir, name), []byte(content), 0644))
	}

	if notifSender == nil {
		notifSender = &mockNotificationSender{}
	}
	dirRepo := &mockDirectoryRepository{
		dirs: map[relpath.Path]*directory.Directory{relpath.Path{}: directory.FromRepository(relpath.Path{})},
	}

	logger := zap.NewNop()
	deps := &isolatedTriggerDeps{
		rootDir:  rootDir,
		hooksDir: hooksDir,
		config:   newTOMLConfigSource(hooksDir, logger),
		notifier: notifSender,
	}
	deps.notes = newNoteContext(rootDir, &mockImageRepository{}, dirRepo)
	deps.exec = newHookExecutor(context.Background(), process, &envBuilder{
		rootDir:     rootDir,
		dataDir:     rootDir,
		graphqlURL:  "http://127.0.0.1:0/graphql",
		tokenSource: &mockTokenSource{},
		dirRepo:     dirRepo,
	}, newHookNotifier(notifSender, logger), logger, rootDir, hooksDir)
	t.Cleanup(deps.exec.Close)
	return deps
}
