package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	appimage "main/internal/application/image"
	"main/internal/shared"

	"go.uber.org/zap"
)

// 编码器标识（进入变体 key）
const encoderSVTAV1 = "svt-av1"

// CommandRunner 命令执行端口：隔离真实进程调用，测试可注入假实现。
// 实现只负责以给定参数执行 ffmpeg，产物由调用方从输出文件读取
type CommandRunner interface {
	Run(ctx context.Context, args []string) error
}

// procRunner 真实进程执行器
type procRunner struct{}

func (procRunner) Run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg error: %w: stderr: %q", err, stderr.String())
	}
	return nil
}

// Encoder 基于 ffmpeg/SVT-AV1 的 AVIF 编码器：只处理 AVIF 转码（Meta 由混合
// 处理链的 magick 分支承担）。仅在启动探测成功后由 main 装配
//
// AVIF muxer 不支持不可寻址输出（无法写管道），因此编码到临时文件再读回，
// 临时文件位于注入的临时目录，编码结束后立即删除
type Encoder struct {
	runner CommandRunner
	// tempDir 临时文件目录；生产为系统临时目录，测试注入受控目录
	tempDir string
}

func NewEncoder(runner CommandRunner, tempDir string) *Encoder {
	return &Encoder{runner: runner, tempDir: tempDir}
}

// ID 返回编码器标识（参与变体缓存 key）
func (e *Encoder) ID() string { return appimage.EncoderSVTAV1 }

// crfFromQuality 把 magick quality（1-100）线性映射到 SVT-AV1 CRF（clamp 18-55）。
// 校准基准：q95→29、q80→39（与 magick 同档体积/画质相当）
func crfFromQuality(quality int) int {
	crf := 95 - quality*7/10
	if crf < 18 {
		crf = 18
	}
	if crf > 55 {
		crf = 55
	}
	return crf
}

// buildArgs 组装 ffmpeg 转码参数。复刻 magick 管线语义：
// - "-y" 覆盖已存在的临时输出文件（CreateTemp 预先创建了空文件）
// - "-nostdin" 防止 ffmpeg 在无输入终端环境下挂起等待交互
// - "-ignore_loop 1" 等价 magick 的 -coalesce（动图按帧序合成，忽略循环元数据）
// - "scale='min(iw,N)':-2" 等价 magick 的 "Nx>"（仅当源宽超过 N 时缩小，且保证偶数高度）
// - 输出目标为临时文件路径（AVIF muxer 需要可寻址输出）
func buildArgs(srcPath, outPath string, spec appimage.Spec, crf int) []string {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-nostdin",
		"-i", srcPath,
		"-ignore_loop", "1",
	}
	if spec.Width() > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale='min(iw,%d)':-2", spec.Width()))
	}
	args = append(args,
		"-c:v", "libsvtav1",
		"-still-picture", "1",
		"-crf", fmt.Sprintf("%d", crf),
		"-f", "avif",
		outPath,
	)
	return args
}

// Process 执行一次 AVIF 转码，结果写入 w
func (e *Encoder) Process(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
	outFile, err := os.CreateTemp(e.tempDir, "image-funnel-avif-*.avif")
	if err != nil {
		return fmt.Errorf("create temp file for avif transcode: %w", err)
	}
	outPath := outFile.Name()
	defer os.Remove(outPath)
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := e.runner.Run(ctx, buildArgs(srcPath, outPath, spec, crfFromQuality(spec.Quality()))); err != nil {
		return err
	}
	result, err := os.ReadFile(outPath)
	if err != nil {
		return fmt.Errorf("read avif transcode result: %w", err)
	}
	_, err = w.Write(result)
	return err
}

// Meta 满足 appimage.Processor 接口：混合链从不把元数据查询路由到编码器，
// 此实现仅为接口完整性——显式报错暴露任何误用
func (e *Encoder) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	return nil, fmt.Errorf("ffmpeg encoder does not implement meta: misuse of avif encoder for metadata")
}

// DetectWithRunner 用给定执行器探测 ffmpeg AVIF 编码能力：
// 实际编码一张测试图验证 muxer 与编码器链路完整可用
// （测试图用 64x64——SVT-AV1 要求最小 4x4；输出走临时文件——AVIF muxer
// 不支持不可寻址输出）
func DetectWithRunner(runner CommandRunner, tempDir string) *Encoder {
	outFile, err := os.CreateTemp(tempDir, "image-funnel-avif-probe-*.avif")
	if err != nil {
		return nil
	}
	probeOut := outFile.Name()
	outFile.Close()
	defer os.Remove(probeOut)

	probe := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=black:s=64x64:d=0.1",
		"-c:v", "libsvtav1",
		"-still-picture", "1",
		"-f", "avif",
		probeOut,
	}
	if err := runner.Run(context.Background(), probe); err != nil {
		return nil
	}
	return NewEncoder(runner, tempDir)
}

// Detect 探测系统 ffmpeg 可用性。成功返回 Encoder（main 用它做 AVIF 分支），
// 失败记录 warn 日志并返回 nil（调用方回退 ImageMagick）
func Detect(logger *zap.Logger) *Encoder {
	encoder := DetectWithRunner(procRunner{}, os.TempDir())
	if encoder == nil {
		logger.Warn("ffmpeg avif encoder not available, fallback to imagemagick for avif transcoding")
		return nil
	}
	logger.Info("ffmpeg avif encoder detected")
	return encoder
}
