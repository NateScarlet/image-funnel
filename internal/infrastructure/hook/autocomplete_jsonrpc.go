package hook

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"main/internal/domain/hook"

	"go.uber.org/zap"
)

// #region JSON-RPC 常驻补全协议常量

// JSON-RPC 2.0 方法名与版本
const (
	jsonRPCVersion            = "2.0"
	jsonRPCMethodAutocomplete = "autocomplete"
	jsonRPCNotificationCancel = "$/cancelRequest"
)

// autocompleteDefaultTimeout 单次补全请求的超时兜底时长（超时未响应则丢弃该请求并重启进程）
const autocompleteDefaultTimeout = 10 * time.Second

// autocompleteDefaultIdleTimeout 常驻补全进程的空闲回收时长
const autocompleteDefaultIdleTimeout = 5 * time.Minute

// errJSONRPCProcessGone 请求期间进程失效（崩溃/退出）的哨兵错误，触发重启后重试一次
var errJSONRPCProcessGone = errors.New("autocomplete jsonrpc process exited")

// errJSONRPCTimeout 请求级超时的哨兵错误
var errJSONRPCTimeout = errors.New("autocomplete jsonrpc request timed out")

// #endregion

// autocompleteJSONRPCParams 自动补全请求参数：沿用现有自动补全上下文
type autocompleteJSONRPCParams struct {
	Cwords           []string `json:"cwords"`
	CwordIdx         int      `json:"cwordIdx"`
	PrevWord         string   `json:"prevWord"`
	LinePrefix       string   `json:"linePrefix"`
	Query            string   `json:"query"`
	ImageIDs         []string `json:"imageIDs,omitempty"`
	ImagePaths       []string `json:"imagePaths,omitempty"`
	NotePath         string   `json:"notePath,omitempty"`
	RootDir          string   `json:"rootDir"`
	DirectoryRelPath string   `json:"directoryRelPath"`
}

// jsonrpcResponse 单个请求的响应结果
type jsonrpcResponse struct {
	suggestions []*hook.AutocompleteSuggestion
	err         error
}

// deliver 非阻塞投递响应，避免进程死亡与响应到达竞争时阻塞
func deliver(ch chan jsonrpcResponse, resp jsonrpcResponse) {
	select {
	case ch <- resp:
	default:
	}
}

// jsonrpcProcess 一个常驻的 JSON-RPC 自动补全脚本进程
type jsonrpcProcess struct {
	logger  *zap.Logger
	pool    *jsonrpcPool
	command string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	stderr io.ReadCloser

	mu       sync.Mutex
	dead     bool
	nextID   int64
	waiters  map[int64]chan jsonrpcResponse
	lastUsed time.Time

	sendMu    sync.Mutex
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// autocompleteJSONRPC 通过常驻 JSON-RPC 进程获取自动补全建议：
// 首次请求 spawn 进程，后续请求复用；进程崩溃/不响应时自动重启。
func (r *Runner) autocompleteJSONRPC(ctx context.Context, targetHook *hookConfig, linePrefix, query string, imageIDs, imagePaths []string, noteAbsPath string) ([]*hook.AutocompleteSuggestion, error) {
	cfg := targetHook.Directive.Autocomplete
	env, err := r.buildAutocompleteEnv(ctx, targetHook, linePrefix, query, imageIDs, imagePaths, noteAbsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build autocomplete env: %w", err)
	}

	cwords, cwordIdx, prevWord := parseLineContext(linePrefix, query)
	// 目录上下文：与 buildBaseEnv 的推导一致（单次模式注入 IMAGE_FUNNEL_DIRECTORY_REL_PATH 的来源）
	dirRel := ""
	if noteAbsPath != "" {
		dirRel = r.dirRelFromAbsPath(noteAbsPath)
	} else if len(imagePaths) > 0 {
		dirRel = r.dirRelFromAbsPath(imagePaths[0])
	}
	params := autocompleteJSONRPCParams{
		Cwords:           cwords,
		CwordIdx:         cwordIdx,
		PrevWord:         prevWord,
		LinePrefix:       linePrefix,
		Query:            query,
		ImageIDs:         imageIDs,
		ImagePaths:       imagePaths,
		NotePath:         noteAbsPath,
		RootDir:          r.rootDir,
		DirectoryRelPath: dirRel,
	}

	proc, err := r.jsonrpcPool.Acquire(cfg.Command, env)
	if err != nil {
		return nil, fmt.Errorf("failed to start autocomplete jsonrpc process: %w", err)
	}

	suggestions, err := proc.request(ctx, r.autocompleteTimeout, jsonRPCMethodAutocomplete, params)
	if err == nil {
		return suggestions, nil
	}
	if errors.Is(err, errJSONRPCTimeout) {
		// 请求超时兜底：丢弃该请求并重启进程，避免补全静默失效
		proc.kill()
		r.logger.Warn("autocomplete jsonrpc request timed out, restarting process",
			zap.String("hook_id", targetHook.ID),
			zap.Duration("timeout", r.autocompleteTimeout),
		)
		return nil, nil
	}
	if errors.Is(err, errJSONRPCProcessGone) {
		// 进程在请求期间崩溃/退出：重启进程后重试一次
		proc, err = r.jsonrpcPool.Acquire(cfg.Command, env)
		if err != nil {
			return nil, fmt.Errorf("failed to restart autocomplete jsonrpc process: %w", err)
		}
		return proc.request(ctx, r.autocompleteTimeout, jsonRPCMethodAutocomplete, params)
	}
	return nil, err
}

// spawnJSONRPCProcess 启动一个常驻的 JSON-RPC 自动补全脚本进程。
// 进程生命周期随 Runner（应用退出时统一回收），请求取消通过 $/cancelRequest 通知而非杀进程。
func (r *Runner) spawnJSONRPCProcess(command string, env []string) (*jsonrpcProcess, error) {
	argv, err := parseCommandArgs(command)
	if err != nil {
		return nil, fmt.Errorf("invalid autocomplete command: %w", err)
	}
	cmd := newHookCmd(r.ctx, argv)
	cmd.Dir = r.hooksDir
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start autocomplete script: %w", err)
	}

	return &jsonrpcProcess{
		logger:  r.logger,
		command: command,
		cmd:     cmd,
		stdin:   stdin,
		reader:  bufio.NewReader(stdout),
		stderr:  stderr,
		waiters: make(map[int64]chan jsonrpcResponse),
	}, nil
}

// start 启动进程的读取 goroutine，须在 pool 将进程登记后再调用
func (p *jsonrpcProcess) start() {
	p.wg.Add(3)
	go func() {
		defer p.wg.Done()
		p.readLoop()
	}()
	go func() {
		defer p.wg.Done()
		p.drainStderr()
	}()
	go func() {
		defer p.wg.Done()
		p.waitProcess()
	}()
}

// alive 报告进程是否仍可处理请求
func (p *jsonrpcProcess) alive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.dead
}

// allocRequestID 分配单调递增的请求 id
func (p *jsonrpcProcess) allocRequestID() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	return p.nextID
}

// request 向常驻进程发送一次 JSON-RPC 请求并等待响应。
// 三层兜底：发新请求前主动取消进行中旧请求；响应按 id 匹配，过期响应被丢弃；
// 请求级超时未响应则返回 errJSONRPCTimeout；进程崩溃则返回 errJSONRPCProcessGone。
func (p *jsonrpcProcess) request(ctx context.Context, timeout time.Duration, method string, params any) ([]*hook.AutocompleteSuggestion, error) {
	reqID := p.allocRequestID()
	reqBody, err := json.Marshal(map[string]any{
		"jsonrpc": jsonRPCVersion,
		"id":      reqID,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.dead {
		p.mu.Unlock()
		return nil, errJSONRPCProcessGone
	}
	respCh := make(chan jsonrpcResponse, 1)
	p.waiters[reqID] = respCh
	p.lastUsed = time.Now()
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.waiters, reqID)
		p.mu.Unlock()
	}()

	// 发新请求前先取消进行中的旧请求，再写入当前请求（串行保证顺序与 id 回显可靠）
	p.sendMu.Lock()
	p.cancelInflightLocked(reqID)
	writeErr := p.writeLine(reqBody)
	p.sendMu.Unlock()
	if writeErr != nil {
		p.markDead(writeErr)
		return nil, errJSONRPCProcessGone
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-respCh:
		return resp.suggestions, resp.err
	case <-ctx.Done():
		// 主动取消：通知脚本中断对应 id 的处理（尽力而为，结果正确性由丢弃过期响应与超时兜底）
		p.sendCancel(reqID)
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errJSONRPCTimeout
	}
}

// cancelInflightLocked 向所有进行中的更旧请求发送 $/cancelRequest 通知（排除当前请求）。
// 以请求 id 大小而非 sendMu 抢占顺序判断新旧，保证并发下只有较新请求能取消较旧请求。
func (p *jsonrpcProcess) cancelInflightLocked(excludeID int64) {
	p.mu.Lock()
	ids := make([]int64, 0, len(p.waiters))
	for id := range p.waiters {
		if id < excludeID {
			ids = append(ids, id)
		}
	}
	p.mu.Unlock()
	for _, id := range ids {
		p.writeCancel(id)
	}
}

// sendCancel 向进程发送单个请求的取消通知
func (p *jsonrpcProcess) sendCancel(id int64) {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	p.writeCancel(id)
}

func (p *jsonrpcProcess) writeCancel(id int64) {
	notif, err := json.Marshal(map[string]any{
		"jsonrpc": jsonRPCVersion,
		"method":  jsonRPCNotificationCancel,
		"params":  map[string]any{"id": id},
	})
	if err != nil {
		return
	}
	_ = p.writeLine(notif)
}

// writeLine 将一段 JSON 以行尾换行写入进程 stdin
func (p *jsonrpcProcess) writeLine(data []byte) error {
	p.mu.Lock()
	dead := p.dead
	p.mu.Unlock()
	if dead {
		return errJSONRPCProcessGone
	}
	buf := make([]byte, 0, len(data)+1)
	buf = append(buf, data...)
	buf = append(buf, '\n')
	_, err := p.stdin.Write(buf)
	return err
}

// readLoop 持续读取进程 stdout，按请求 id 派发响应
func (p *jsonrpcProcess) readLoop() {
	for {
		line, err := p.reader.ReadString('\n')
		if line != "" {
			p.handleLine(line)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				p.logger.Debug("autocomplete jsonrpc stdout read failed",
					zap.String("command", p.command),
					zap.Error(err),
				)
			}
			p.markDead(fmt.Errorf("stdout closed: %w", err))
			return
		}
	}
}

// handleLine 解析一行 JSON-RPC 响应并投递给对应的等待者；过期响应直接丢弃
func (p *jsonrpcProcess) handleLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	var msg struct {
		ID     *int64          `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		// 非 JSON 行（脚本误把日志写入 stdout，或测试进程的框架输出）记录并忽略
		p.logger.Warn("ignoring non-JSON line from autocomplete script stdout",
			zap.String("command", p.command),
			zap.String("line", line),
		)
		return
	}
	if msg.ID == nil {
		// 服务端主动通知（非响应），忽略
		return
	}

	p.mu.Lock()
	ch, ok := p.waiters[*msg.ID]
	delete(p.waiters, *msg.ID)
	p.mu.Unlock()
	if !ok {
		// 过期响应（id 已超时/已取消），丢弃
		return
	}

	var resp jsonrpcResponse
	if msg.Error != nil {
		resp.err = fmt.Errorf("autocomplete script error (code %d): %s", msg.Error.Code, msg.Error.Message)
	} else {
		suggestions, parseErr := parseJSONRPCResult(msg.Result)
		resp = jsonrpcResponse{suggestions: suggestions, err: parseErr}
	}
	deliver(ch, resp)
}

// parseJSONRPCResult 解析 JSON-RPC result 中的建议列表
func parseJSONRPCResult(raw json.RawMessage) ([]*hook.AutocompleteSuggestion, error) {
	var raws []autocompleteSuggestionRaw
	if err := json.Unmarshal(raw, &raws); err != nil {
		return nil, fmt.Errorf("failed to parse autocomplete jsonrpc result: %w", err)
	}
	suggestions := make([]*hook.AutocompleteSuggestion, 0, len(raws))
	for _, r := range raws {
		suggestions = append(suggestions, convertSuggestion(r))
	}
	return suggestions, nil
}

// drainStderr 持续消费进程 stderr 并记录日志（业务日志通道，不污染协议 stdout）
func (p *jsonrpcProcess) drainStderr() {
	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		p.logger.Debug("autocomplete jsonrpc script stderr",
			zap.String("command", p.command),
			zap.String("stderr", scanner.Text()),
		)
	}
}

// waitProcess 等待进程退出并标记失效（进程崩溃/正常退出的最终信号）
func (p *jsonrpcProcess) waitProcess() {
	err := p.cmd.Wait()
	p.markDead(err)
}

// markDead 标记进程失效：从池中移除自身，并向所有等待中的请求投递失败
func (p *jsonrpcProcess) markDead(err error) {
	p.mu.Lock()
	if p.dead {
		p.mu.Unlock()
		return
	}
	p.dead = true
	waiters := p.waiters
	p.waiters = map[int64]chan jsonrpcResponse{}
	p.mu.Unlock()

	// 从进程池中移除自身，后续请求会重新 spawn
	if p.pool != nil {
		p.pool.mu.Lock()
		if p.pool.procs[p.command] == p {
			delete(p.pool.procs, p.command)
		}
		p.pool.mu.Unlock()
	}

	respErr := fmt.Errorf("%w: %v", errJSONRPCProcessGone, err)
	for _, ch := range waiters {
		deliver(ch, jsonrpcResponse{err: respErr})
	}
}

// kill 关闭 stdin 并终止进程树（用于超时重启 / 空闲回收 / 应用退出）
func (p *jsonrpcProcess) kill() {
	p.closeOnce.Do(func() {
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		if p.cmd.Cancel != nil {
			_ = p.cmd.Cancel()
		}
		p.markDead(errors.New("process killed"))
	})
}

// jsonrpcPool 按 command 维度维护常驻自动补全进程池
type jsonrpcPool struct {
	logger      *zap.Logger
	idleTimeout time.Duration
	spawn       func(command string, env []string) (*jsonrpcProcess, error)

	mu        sync.Mutex
	procs     map[string]*jsonrpcProcess
	closed    chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func newJSONRPCPool(logger *zap.Logger, idleTimeout time.Duration, spawn func(string, []string) (*jsonrpcProcess, error)) *jsonrpcPool {
	pl := &jsonrpcPool{
		logger:      logger,
		idleTimeout: idleTimeout,
		spawn:       spawn,
		procs:       make(map[string]*jsonrpcProcess),
		closed:      make(chan struct{}),
	}
	pl.wg.Add(1)
	go pl.reaperLoop()
	return pl
}

// Acquire 返回 command 对应的常驻进程；不存在或已失效则启动新进程
func (pl *jsonrpcPool) Acquire(command string, env []string) (*jsonrpcProcess, error) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if p, ok := pl.procs[command]; ok && p.alive() {
		return p, nil
	}
	p, err := pl.spawn(command, env)
	if err != nil {
		return nil, err
	}
	// 先登记再启动读取 goroutine，保证进程立即崩溃时也能从池中清理
	p.pool = pl
	pl.procs[command] = p
	p.start()
	return p, nil
}

// reaperLoop 周期性回收超过空闲时长且无进行中请求的进程
func (pl *jsonrpcPool) reaperLoop() {
	defer pl.wg.Done()
	ticker := time.NewTicker(pl.idleTimeout / 4)
	defer ticker.Stop()
	for {
		select {
		case <-pl.closed:
			return
		case now := <-ticker.C:
			pl.reap(now)
		}
	}
}

func (pl *jsonrpcPool) reap(now time.Time) {
	pl.mu.Lock()
	var toKill []*jsonrpcProcess
	for _, p := range pl.procs {
		p.mu.Lock()
		dead := p.dead
		idle := now.Sub(p.lastUsed)
		inFlight := len(p.waiters) > 0
		p.mu.Unlock()
		if !dead && !inFlight && idle > pl.idleTimeout {
			toKill = append(toKill, p)
		}
	}
	pl.mu.Unlock()
	for _, p := range toKill {
		pl.logger.Debug("reaping idle autocomplete jsonrpc process",
			zap.String("command", p.command),
			zap.Duration("idle_time", pl.idleTimeout),
		)
		p.kill()
	}
}

// Close 随应用退出：终止所有常驻进程并等待回收 goroutine 结束
func (pl *jsonrpcPool) Close() {
	pl.closeOnce.Do(func() {
		close(pl.closed)
		pl.mu.Lock()
		procs := pl.procs
		pl.procs = map[string]*jsonrpcProcess{}
		pl.mu.Unlock()
		for _, p := range procs {
			p.kill()
		}
		pl.wg.Wait()
	})
}
