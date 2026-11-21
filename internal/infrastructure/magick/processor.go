package magick

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	appimage "main/internal/application/image"
	"main/internal/shared"
	"main/internal/shared/transcode"

	"golang.org/x/sync/semaphore"
)

// Processor 纯转码执行器：不负责缓存（缓存由应用层 TranscodeCoordinator 编排），
// 只对单张源图执行一次 ImageMagick 转码并把结果流式交给调用方
type Processor struct {
	sem *semaphore.Weighted
}

func NewProcessor(concurrency int64) *Processor {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Processor{
		sem: semaphore.NewWeighted(concurrency),
	}
}

// webpQuality WebP 输出质量。取 92 的依据（实测 2304px 源图）：
// q92 相比 q75 画质高 3.3dB 而编码仅慢约 15ms；q95 附近会滑向无损编码
// （q100 时 1024w 从 501ms 暴涨到 2496ms、体积 129KB→1298KB），故留出余量
const webpQuality = 92

// resizeArgument 生成 -resize 参数，目标宽度由唯一规则 transcode.EffectiveWidth 给出，
// 编码器侧不另立收缩规则。
//
// 本 seam 不掌握源宽时传入 0：规则原样返回 width，由 magick 的 '>' 在运行时完成收缩。
// '>' 与 ffmpeg 的 min(iw,N) 同为运行时表达，语义等价性由共享测试锁定。
func resizeArgument(width, sourceWidth int) string {
	return fmt.Sprintf("%dx>", transcode.EffectiveWidth(width, sourceWidth))
}

// Process 执行一次转码，结果流式写入 w。源文件状态（修改时间/大小）在执行时读取，
// 由调用方用于缓存键计算
func (p *Processor) Process(ctx context.Context, srcPath string, spec transcode.Spec, w io.Writer) error {
	if err := p.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer p.sem.Release(1)

	args := []string{srcPath, "-coalesce"}
	if spec.Width() > 0 {
		// spec 宽度已由 handler 经 EffectiveWidth 收敛；参数仍由同一规则生成，
		// 编码器侧没有第二份收缩规则
		args = append(args, "-resize", resizeArgument(spec.Width(), 0))
	}
	args = append(args, "-quality", fmt.Sprintf("%d", webpQuality))
	args = append(args, spec.Format().String()+":-")

	cmd := exec.CommandContext(ctx, "magick", args...)
	cmd.Stdout = w
	var b = new(bytes.Buffer)
	cmd.Stderr = b

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		errStr := b.String()
		// 识别文件尚未写完时的意外截止错误
		if strings.Contains(errStr, "unexpected end-of-file") || strings.Contains(errStr, "unexpected end of file") {
			return fmt.Errorf("%w: ImageMagick error: %s", io.ErrUnexpectedEOF, errStr)
		}
		return fmt.Errorf("ImageMagick error: %w, args: %v: stderr: %q", err, args, errStr)
	}
	return nil
}

func (p *Processor) Meta(ctx context.Context, absPath string) (*shared.ImageMeta, error) {
	if err := p.sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer p.sem.Release(1)

	cmd := exec.CommandContext(ctx, "magick", "identify", "-ping", "-format", "%w %h", absPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		errStr := stderr.String()
		// 识别并转换为领域层标准错误
		if strings.Contains(errStr, "unexpected end-of-file") || strings.Contains(errStr, "unexpected end of file") {
			return nil, fmt.Errorf("%w: failed to get image metadata: %s", io.ErrUnexpectedEOF, errStr)
		}
		return nil, fmt.Errorf("failed to get image metadata: %w, stderr: %s", err, errStr)
	}

	var width, height int
	_, err = fmt.Sscanf(string(output), "%d %d", &width, &height)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image dimensions: %w", err)
	}

	return &shared.ImageMeta{
		Width:  width,
		Height: height,
	}, nil
}

var _ appimage.Processor = (*Processor)(nil)
