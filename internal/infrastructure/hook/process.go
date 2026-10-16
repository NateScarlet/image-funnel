package hook

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// processRunner 外部脚本的进程原语：包住「构造进程 + 运行 + 取消」，
// 同时覆盖普通单次执行与 JSON-RPC 常驻进程两条生命周期。
type processRunner interface {
	// Run 启动一次性进程并等待其结束，收集 stdout 与 stderr
	Run(ctx context.Context, argv []string, env []string, dir string) (processOutput, error)
	// SpawnResident 启动常驻进程，返回可按行读写 stdin/stdout 的句柄。
	// 进程生命周期与调用方的 context 绑定，context 取消时进程树被终止。
	SpawnResident(ctx context.Context, argv []string, env []string, dir string) (residentProcess, error)
}

// processOutput 一次性进程的输出
type processOutput struct {
	Stdout string
	Stderr string
}

// residentProcess 常驻进程句柄：JSON-RPC 补全服务以此与脚本进程长期交互
type residentProcess interface {
	// WriteLine 向 stdin 写入一行（自动补行尾换行）
	WriteLine(data []byte) error
	// ReadLine 从 stdout 读取一行。进程退出时可能同时返回残缺行与错误，
	// 调用方需先消费残缺行再处理错误。
	ReadLine() (string, error)
	// DrainStderr 持续消费 stderr 直到流结束，逐行回调
	DrainStderr(onLine func(string))
	// Wait 等待进程退出并返回其退出错误
	Wait() error
	// Kill 关闭 stdin 并终止进程树
	Kill()
}

// #region 基于 os/exec 的实现

// runOnceScript 单次 spawn 脚本并收集输出：命令已分词，直接交给进程原语运行；
// 取消与失败都把 stderr 并入错误。自动补全（单次模式）与复制增强共用这条管道，
// 各自只负责命令分词与参数准备。
func runOnceScript(ctx context.Context, process processRunner, logger *zap.Logger, hooksDir string, argv []string, env []string, scriptName string) (processOutput, error) {
	logger.Debug("will execute "+scriptName,
		zap.Strings("argv", argv),
	)

	out, err := process.Run(ctx, argv, env, hooksDir)
	if ctx.Err() != nil {
		logger.Debug(scriptName+" canceled by context", zap.Error(ctx.Err()))
		return out, ctx.Err()
	}
	if err != nil {
		logger.Warn(scriptName+" failed", zap.Error(err), zap.String("stderr", out.Stderr))
		return out, fmt.Errorf("%s failed: %w, stderr: %s", scriptName, err, strings.TrimSpace(out.Stderr))
	}

	// stderr 是调试通道，只记日志不进入返回值
	if out.Stderr != "" {
		logger.Debug(scriptName+" stderr", zap.String("stderr", out.Stderr))
	}
	return out, nil
}

// execProcessRunner 基于 os/exec 的进程原语实现
type execProcessRunner struct {
	log *zap.Logger
}

var _ processRunner = execProcessRunner{}

func (execProcessRunner) Run(ctx context.Context, argv []string, env []string, dir string) (processOutput, error) {
	cmd := newProcessCmd(ctx, argv)
	cmd.Dir = dir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return processOutput{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

func (r execProcessRunner) SpawnResident(ctx context.Context, argv []string, env []string, dir string) (residentProcess, error) {
	cmd := newProcessCmd(ctx, argv)
	cmd.Dir = dir
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
		return nil, fmt.Errorf("failed to start hook script: %w", err)
	}

	return &cmdResidentProcess{
		log:    r.log,
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		stderr: stderr,
	}, nil
}

// cmdResidentProcess 基于 exec.Cmd 的常驻进程句柄。
// 读取 stdout/stderr 与等待退出并发进行：cmd.Wait() 会在进程退出后关闭管道，
// 因此必须由独立的 goroutine 各自消费。
type cmdResidentProcess struct {
	log    *zap.Logger
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	stderr io.ReadCloser

	killOnce sync.Once
}

var _ residentProcess = (*cmdResidentProcess)(nil)

func (p *cmdResidentProcess) WriteLine(data []byte) error {
	buf := make([]byte, 0, len(data)+1)
	buf = append(buf, data...)
	buf = append(buf, '\n')
	_, err := p.stdin.Write(buf)
	return err
}

func (p *cmdResidentProcess) ReadLine() (string, error) {
	return p.reader.ReadString('\n')
}

func (p *cmdResidentProcess) DrainStderr(onLine func(string)) {
	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		onLine(scanner.Text())
	}
}

func (p *cmdResidentProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *cmdResidentProcess) Kill() {
	p.killOnce.Do(func() {
		// 两条中止手段都是尽力而为：stdin 已随进程退出关闭、或进程尚未启动时它们都
		// 必然报错，而这里能采取的中止已经全部尝试，没有可交给调用者的失败语义。
		if p.stdin != nil {
			if err := p.stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				p.log.Debug("failed to close resident process stdin", zap.Error(err))
			}
		}
		if p.cmd.Cancel != nil {
			if err := p.cmd.Cancel(); err != nil {
				p.log.Debug("failed to cancel resident process", zap.Error(err))
			}
		}
	})
}

// #endregion
