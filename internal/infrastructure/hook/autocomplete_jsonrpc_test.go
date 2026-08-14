package hook

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"main/internal/domain/directory"
	domimage "main/internal/domain/image"
	"main/internal/scalar"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// #region 假 JSON-RPC 补全服务（复用当前测试二进制作为子进程，避免依赖外部运行时）

// TestIFJSONRPCReaderHelper 当环境变量 IF_AUTOCOMPLETE_HELPER=1 时，作为常驻 JSON-RPC
// 补全服务的假实现运行：stdin 读请求，stdout 写响应，stdin 关闭即退出。
func TestIFJSONRPCReaderHelper(t *testing.T) {
	if os.Getenv("IF_AUTOCOMPLETE_HELPER") != "1" {
		return
	}
	runAutocompleteHelper()
}

// helperParams 假服务解析的自动补全请求参数
type helperParams struct {
	Query            string   `json:"query"`
	Cwords           []string `json:"cwords"`
	CwordIdx         int      `json:"cwordIdx"`
	PrevWord         string   `json:"prevWord"`
	LinePrefix       string   `json:"linePrefix"`
	ImageIDs         []string `json:"imageIDs"`
	ImagePaths       []string `json:"imagePaths"`
	NotePath         string   `json:"notePath"`
	RootDir          string   `json:"rootDir"`
	DirectoryRelPath string   `json:"directoryRelPath"`
}

func runAutocompleteHelper() {
	pid := os.Getpid()
	if f := os.Getenv("IF_HELPER_PID_FILE"); f != "" {
		_ = os.WriteFile(f, []byte(strconv.Itoa(pid)), 0644)
	}
	logFile := os.Getenv("IF_HELPER_LOG_FILE")
	logf := func(format string, args ...any) {
		if logFile == "" {
			return
		}
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = fmt.Fprintf(f, format+"\n", args...)
	}

	// 单次执行模式：输出一行 JSONL 建议后退出（os.Exit 避免测试框架追加的 PASS 污染 stdout）
	if os.Getenv("IF_HELPER_ONESHOT") == "1" {
		fmt.Printf(`{"text":"%d","displayText":"helper","type":"test"}`+"\n", pid)
		os.Exit(0)
	}

	sleepMS, _ := strconv.Atoi(os.Getenv("IF_HELPER_SLEEP_MS"))
	exitAfter, _ := strconv.Atoi(os.Getenv("IF_HELPER_EXIT_AFTER"))
	uninterruptible := os.Getenv("IF_HELPER_UNINTERRUPTIBLE") == "1"

	var mu sync.Mutex
	canceled := map[string]bool{}
	isCanceled := func(id string) bool {
		mu.Lock()
		defer mu.Unlock()
		return canceled[id]
	}

	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	var writeMu sync.Mutex

	// writeResp 写一条 JSON-RPC 响应，description 回显请求参数以支持契约断言
	writeResp := func(reqID json.RawMessage, params []byte, suggestions []map[string]string) {
		resp, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(reqID),
			"result":  suggestions,
		})
		writeMu.Lock()
		_, _ = writer.Write(append(resp, '\n'))
		_ = writer.Flush()
		writeMu.Unlock()
	}

	// handle 处理一次 autocomplete 请求，返回 false 表示进程应退出
	handle := func(reqID json.RawMessage, paramsRaw []byte) bool {
		var p helperParams
		_ = json.Unmarshal(paramsRaw, &p)

		if strings.Contains(p.Query, "ignore") {
			logf("ignored:%s", string(reqID))
			return true
		}

		if strings.HasPrefix(p.Query, "slow") {
			// 慢请求：独立 goroutine 分片睡眠，主循环继续读 stdin 以接收 $/cancelRequest
			go func() {
				if !uninterruptible {
					deadline := time.Now().Add(time.Duration(sleepMS) * time.Millisecond)
					for time.Now().Before(deadline) {
						if isCanceled(string(reqID)) {
							logf("interrupted:%s", string(reqID))
							writeResp(reqID, paramsRaw, nil)
							return
						}
						time.Sleep(50 * time.Millisecond)
					}
				} else {
					time.Sleep(time.Duration(sleepMS) * time.Millisecond)
				}
				// 未被打断：返回标记为过期的建议，便于验证应用侧丢弃过期响应
				writeResp(reqID, paramsRaw, []map[string]string{{
					"text":        "stale:" + string(reqID),
					"displayText": "helper",
					"description": string(paramsRaw),
					"type":        "test",
				}})
			}()
			return true
		}

		// 快请求：同步响应
		writeResp(reqID, paramsRaw, []map[string]string{{
			"text":        strconv.Itoa(pid),
			"displayText": "helper",
			"description": string(paramsRaw),
			"type":        "test",
		}})

		if exitAfter > 0 {
			exitAfter--
			if exitAfter <= 0 {
				return false
			}
		}
		return true
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		switch req.Method {
		case "$/cancelRequest":
			var params struct {
				ID json.RawMessage `json:"id"`
			}
			_ = json.Unmarshal(req.Params, &params)
			idStr := string(params.ID)
			logf("cancel:%s", idStr)
			mu.Lock()
			canceled[idStr] = true
			mu.Unlock()
		case "autocomplete":
			logf("req:%s", string(req.ID))
			if !handle(req.ID, req.Params) {
				return
			}
		}
	}
}

// #endregion

// #region 测试基础设施

type autocompleteTestEnv struct {
	tempDir  string
	hooksDir string
	runner   *Runner
	hookID   scalar.ID
}

// newAutocompleteTestRunner 搭建一个配置了假补全服务的测试 Runner
func newAutocompleteTestRunner(t *testing.T, jsonrpc bool, envOverrides map[string]string, imgRepo *mockImageRepository) *autocompleteTestEnv {
	t.Helper()
	tempDir := t.TempDir()
	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))

	// autocomplete 命令：spawn 当前测试二进制，仅运行假服务辅助测试
	helperCmd := fmt.Sprintf(`"%s" -test.run=^TestIFJSONRPCReaderHelper$`, os.Args[0])

	protocolLine := ""
	if jsonrpc {
		protocolLine = "protocol = \"json-rpc\"\n"
	}

	var envSB strings.Builder
	envSB.WriteString("[env]\n")
	envSB.WriteString("IF_AUTOCOMPLETE_HELPER = '1'\n")
	for k, v := range envOverrides {
		fmt.Fprintf(&envSB, "%s = '%s'\n", k, v)
	}

	tomlContent := fmt.Sprintf(`
id = "if-autocomplete-test"
name = "if-autocomplete-test"
command = "echo unused"

[directive]
name = "if"

[directive.autocomplete]
command = '''%s'''
%s%s
`, helperCmd, protocolLine, envSB.String())
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "if-autocomplete-test.toml"), []byte(tomlContent), 0644))

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	logger := zap.NewNop()
	mockDirRepo := &mockDirectoryRepository{
		dirs: map[string]*directory.Directory{".": directory.FromRepository(".")},
	}
	if imgRepo == nil {
		imgRepo = &mockImageRepository{}
	}
	runner := NewRunner(tempDir, hooksDir, logger, ebus, fileChangedSub, "", &mockTokenSource{}, imgRepo, nil, mockDirRepo, &mockNotificationSender{})
	t.Cleanup(runner.Close)
	return &autocompleteTestEnv{tempDir: tempDir, hooksDir: hooksDir, runner: runner, hookID: scalar.ToID("hk:if-autocomplete-test")}
}

// processAlive 判断 pid 对应的进程是否仍在运行
func processAlive(pidStr string) bool {
	if filepath.Separator == '\\' {
		out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %s", pidStr)).Output()
		return err == nil && strings.Contains(string(out), pidStr)
	}
	return exec.Command("ps", "-p", pidStr).Run() == nil
}

// poolSize 返回进程池中存活的常驻进程数量
func (env *autocompleteTestEnv) poolSize() int {
	env.runner.jsonrpcPool.mu.Lock()
	defer env.runner.jsonrpcPool.mu.Unlock()
	return len(env.runner.jsonrpcPool.procs)
}

// #endregion

// #region 配置层测试

func TestAutocompleteConfig_ProtocolParsing(t *testing.T) {
	tempDir := t.TempDir()
	hooksDir := filepath.Join(tempDir, ".image-funnel", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))

	// 与 example_hooks 中的实际写法一致：protocol 行带内联注释
	tomlContent := `
id = "proto-test"
name = "proto-test"
command = "echo unused"

[directive]
name = "proto"

[directive.autocomplete]
command = "uv run runner.py comfyui.autocomplete serve"
protocol = "json-rpc" # 常驻复用：进程启动后复用，避免每次输入触发脚本冷启动
`
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "proto.toml"), []byte(tomlContent), 0644))

	ebus := &mockMetadataUpdatedSub{}
	fileChangedSub := &mockFileChangedSub{}
	runner := NewRunner(tempDir, hooksDir, zap.NewNop(), ebus, fileChangedSub, "", &mockTokenSource{}, &mockImageRepository{}, nil, nil, &mockNotificationSender{})
	defer runner.Close()

	configs, err := runner.loadHooks()
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.NotNil(t, configs[0].Directive)
	require.NotNil(t, configs[0].Directive.Autocomplete)
	assert.Equal(t, "json-rpc", configs[0].Directive.Autocomplete.Protocol)
	assert.Equal(t, "uv run runner.py comfyui.autocomplete serve", configs[0].Directive.Autocomplete.Command)
}

// #endregion

// #region 常驻进程复用与生命周期

func TestAutocomplete_JSONRPC_ResidentReuse(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "helper.pid")
	env := newAutocompleteTestRunner(t, true, map[string]string{"IF_HELPER_PID_FILE": pidFile}, nil)

	// 同一 command 的多次补全请求应复用同一常驻进程（返回相同 pid）
	pids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		suggestions, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add --region po", fmt.Sprintf("q%d", i))
		require.NoError(t, err)
		require.Len(t, suggestions, 1)
		pids = append(pids, suggestions[0].Text)
	}
	assert.Equal(t, pids[0], pids[1], "连续请求应复用同一常驻进程")
	assert.Equal(t, pids[1], pids[2], "连续请求应复用同一常驻进程")

	// 进程只被 spawn 一次：pid 文件内容应与返回的 pid 一致
	pidBytes, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	assert.Equal(t, pids[0], strings.TrimSpace(string(pidBytes)))
	assert.Equal(t, 1, env.poolSize(), "进程池中应只有一个常驻进程")
}

func TestAutocomplete_JSONRPC_CrashRestart(t *testing.T) {
	env := newAutocompleteTestRunner(t, true, map[string]string{"IF_HELPER_EXIT_AFTER": "1"}, nil)

	sug1, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "a")
	require.NoError(t, err)
	require.Len(t, sug1, 1)
	pid1 := sug1[0].Text

	// 假服务响应 1 次后退出（模拟崩溃），等待其失效并从池中移除
	assert.Eventually(t, func() bool { return env.poolSize() == 0 }, 5*time.Second, 20*time.Millisecond)

	// 下一次请求应自动重启新进程
	sug2, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "b")
	require.NoError(t, err)
	require.Len(t, sug2, 1)
	assert.NotEqual(t, pid1, sug2[0].Text, "进程崩溃后应自动重启为新进程")
}

func TestAutocomplete_JSONRPC_ProcessExitsOnStdinClose(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "helper.pid")
	env := newAutocompleteTestRunner(t, true, map[string]string{"IF_HELPER_PID_FILE": pidFile}, nil)

	suggestions, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "a")
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	pidStr := suggestions[0].Text

	// 随应用退出：常驻进程的 stdin 关闭后进程应退出
	env.runner.Close()

	assert.Eventually(t, func() bool { return !processAlive(pidStr) }, 10*time.Second, 50*time.Millisecond)
}

// #endregion

// #region JSON-RPC 契约

func TestAutocomplete_JSONRPC_RequestContract(t *testing.T) {
	env := newAutocompleteTestRunner(t, true, nil, nil)

	suggestions, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add --region po", "po")
	require.NoError(t, err)
	require.Len(t, suggestions, 1)

	// 假服务回显请求参数，验证请求契约与 id 回显
	var echo helperParams
	require.NoError(t, json.Unmarshal([]byte(suggestions[0].Description), &echo))
	assert.Equal(t, "po", echo.Query)
	assert.Equal(t, "/add --region po", echo.LinePrefix)
	assert.Equal(t, []string{"/add", "--region"}, echo.Cwords)
	assert.Equal(t, 2, echo.CwordIdx)
	assert.Equal(t, "--region", echo.PrevWord)
	// 目录上下文（rootDir / directoryRelPath）随请求参数逐请求传递
	assert.Equal(t, env.tempDir, echo.RootDir)
	assert.Equal(t, "", echo.DirectoryRelPath)
}

func TestAutocomplete_JSONRPC_ImageContextInParams(t *testing.T) {
	imgID := scalar.ToID("img:1")
	img := domimage.New(imgID, "test.png", "test.png", scalar.ToID("dir:1"), 100, time.Now(), nil, 0, 0)
	env := newAutocompleteTestRunner(t, true, nil, &mockImageRepository{images: []*domimage.Image{img}})

	suggestions, err := env.runner.Autocomplete(context.Background(), env.hookID, "test.png.md", "/add", "d")
	require.NoError(t, err)
	require.Len(t, suggestions, 1)

	var echo helperParams
	require.NoError(t, json.Unmarshal([]byte(suggestions[0].Description), &echo))
	assert.Equal(t, []string{imgID.String()}, echo.ImageIDs)
	assert.Equal(t, []string{filepath.Join(env.tempDir, "test.png")}, echo.ImagePaths)
	assert.Equal(t, filepath.Join(env.tempDir, "test.png.md"), echo.NotePath)
	// 笔记位于根目录，目录上下文随请求传递
	assert.Equal(t, env.tempDir, echo.RootDir)
	assert.Equal(t, ".", echo.DirectoryRelPath)
}

// #endregion

// #region 取消、过期响应与超时

func TestAutocomplete_JSONRPC_Cancel(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "helper.log")
	env := newAutocompleteTestRunner(t, true, map[string]string{
		"IF_HELPER_SLEEP_MS": "5000",
		"IF_HELPER_LOG_FILE": logFile,
	}, nil)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	type result struct {
		suggestions []string
		err         error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := env.runner.Autocomplete(ctx1, env.hookID, "", "/add slow", "slow query")
		var texts []string
		for _, sug := range s {
			texts = append(texts, sug.Text)
		}
		ch <- result{texts, err}
	}()

	// 等待假服务收到慢请求（开始处理）
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(logFile)
		return err == nil && strings.Contains(string(data), "req:")
	}, 10*time.Second, 20*time.Millisecond)

	// 取消应中断请求处理：立即返回 context.Canceled
	start := time.Now()
	cancel1()
	res := <-ch
	assert.ErrorIs(t, res.err, context.Canceled)
	assert.Less(t, time.Since(start), 3*time.Second, "取消后应立即返回")

	// 应用应发送 $/cancelRequest 通知，假服务应中断对应任务
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(logFile)
		return err == nil && strings.Contains(string(data), "cancel:")
	}, 3*time.Second, 20*time.Millisecond)
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(logFile)
		return err == nil && strings.Contains(string(data), "interrupted:")
	}, 3*time.Second, 20*time.Millisecond)
}

func TestAutocomplete_JSONRPC_StaleResponseDropped(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "helper.log")
	env := newAutocompleteTestRunner(t, true, map[string]string{
		"IF_HELPER_SLEEP_MS":        "800",
		"IF_HELPER_UNINTERRUPTIBLE": "1",
		"IF_HELPER_LOG_FILE":        logFile,
	}, nil)

	// 请求 1：慢请求（不可中断），独立 goroutine 执行
	ctx1, cancel1 := context.WithCancel(context.Background())
	type result struct {
		texts []string
		err   error
	}
	ch1 := make(chan result, 1)
	go func() {
		s, err := env.runner.Autocomplete(ctx1, env.hookID, "", "/add slow", "slow one")
		var texts []string
		for _, sug := range s {
			texts = append(texts, sug.Text)
		}
		ch1 <- result{texts, err}
	}()

	// 等待假服务收到慢请求
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(logFile)
		return err == nil && strings.Contains(string(data), "req:")
	}, 10*time.Second, 20*time.Millisecond)

	// 取消慢请求：立即返回 context.Canceled
	cancel1()
	res1 := <-ch1
	assert.ErrorIs(t, res1.err, context.Canceled)

	// 请求 2/3：快请求应返回真实 pid（不是慢请求晚到的 stale 响应）
	sug2, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "fast two")
	require.NoError(t, err)
	require.Len(t, sug2, 1)
	assert.NotContains(t, sug2[0].Text, "stale", "过期响应不应泄漏到后续请求")

	sug3, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "fast three")
	require.NoError(t, err)
	require.Len(t, sug3, 1)
	assert.Equal(t, sug2[0].Text, sug3[0].Text, "晚到的过期响应不应杀死或污染常驻进程")
}

func TestAutocomplete_JSONRPC_TimeoutRestartsProcess(t *testing.T) {
	env := newAutocompleteTestRunner(t, true, nil, nil)
	env.runner.autocompleteTimeout = 300 * time.Millisecond

	// 请求被假服务忽略（不响应）→ 应用超时丢弃该请求，返回空建议而非错误
	suggestions, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add ignore", "ignore me")
	assert.NoError(t, err)
	assert.Nil(t, suggestions)

	// 超时后应杀死不响应的进程并移出进程池
	assert.Eventually(t, func() bool { return env.poolSize() == 0 }, 5*time.Second, 20*time.Millisecond)

	// 恢复默认超时：新 spawn 的进程在慢速环境（如 CI 模拟架构）下启动可能超过 300ms，
	// 避免新进程的首次请求因启动过慢被误判为超时
	env.runner.autocompleteTimeout = autocompleteDefaultTimeout

	// 下一次请求应重新 spawn 新进程正常工作
	sug2, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "b")
	require.NoError(t, err)
	require.Len(t, sug2, 1)
	assert.NotEqual(t, "", sug2[0].Text)
}

func TestAutocomplete_JSONRPC_ConcurrentRequests(t *testing.T) {
	env := newAutocompleteTestRunner(t, true, nil, nil)

	// 并发请求：各请求都应拿到自己的响应并复用同一常驻进程，且不会被较新请求误取消
	const n = 6
	var wg sync.WaitGroup
	results := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			suggestions, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", fmt.Sprintf("c%d", i))
			errs[i] = err
			if len(suggestions) > 0 {
				results[i] = suggestions[0].Text
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		assert.NoError(t, errs[i], "并发请求 %d 不应失败", i)
		assert.NotEmpty(t, results[i], "并发请求 %d 应返回建议", i)
		assert.Equal(t, results[0], results[i], "并发请求应复用同一常驻进程")
	}
}

// #endregion

// #region 单次模式向后兼容

func TestAutocomplete_SingleShot_BackwardCompat(t *testing.T) {
	// 未设置 protocol：保持现有单次执行行为，每次请求 spawn 新进程（pid 不同）
	env := newAutocompleteTestRunner(t, false, map[string]string{
		"IF_HELPER_PID_FILE": filepath.Join(t.TempDir(), "helper.pid"),
		"IF_HELPER_ONESHOT":  "1",
	}, nil)

	s1, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "a")
	require.NoError(t, err)
	require.Len(t, s1, 1)
	s2, err := env.runner.Autocomplete(context.Background(), env.hookID, "", "/add", "b")
	require.NoError(t, err)
	require.Len(t, s2, 1)
	assert.NotEqual(t, s1[0].Text, s2[0].Text, "单次模式每次请求应产生新进程")
	assert.Equal(t, 0, env.poolSize(), "单次模式不应占用常驻进程池")
}

// #endregion
