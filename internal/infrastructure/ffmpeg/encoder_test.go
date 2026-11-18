package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"main/internal/shared/transcode"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestBuildArgs_Scaled(t *testing.T) {
	spec, err := transcode.NewSpec(2048, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	args := buildArgs("src/img.png", "out/result.avif", spec)
	joined := strings.Join(args, " ")

	// 动图语义（-coalesce 等价）：忽略帧延迟合成所有帧
	assert.Contains(t, joined, "-ignore_loop 1")
	// 仅缩小语义（1024x> 等价）：min(iw,N) 保证不放大
	assert.Contains(t, joined, "scale='min(iw,2048)':-2")
	// SVT-AV1 静帧
	assert.Contains(t, joined, "-c:v libsvtav1")
	assert.Contains(t, joined, "-still-picture 1")
	// AVIF muxer 强制
	assert.Contains(t, joined, "-f avif")
	assert.Equal(t, "out/result.avif", args[len(args)-1], "output path must be last arg (seekable file, AVIF muxer cannot write pipe)")
}

func TestBuildArgs_FullResolution(t *testing.T) {
	// 无宽度参数 = 全分辨率输出，不加 scale 滤镜
	spec, err := transcode.NewSpec(0, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	args := buildArgs("src/img.png", "out/result.avif", spec)
	joined := strings.Join(args, " ")

	assert.NotContains(t, joined, "scale=")
	assert.Contains(t, joined, "-f avif")
}

func TestBuildArgs_PreservesFullColorRange(t *testing.T) {
	// magick 输出标记 color_range=pc（全范围）；ffmpeg 默认 tv（受限范围）会让
	// 黑位与对比度被压缩。实测此标记影响 PSNR 约 0.6dB，必须显式设为 pc
	spec, err := transcode.NewSpec(1024, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	args := buildArgs("src/img.png", "out/result.avif", spec)
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "-color_range pc", "must mark full range to match magick output")
	assert.Contains(t, joined, "-pix_fmt yuv420p", "pixel format must be explicit")
}

func TestCRFForQuality_IsHighQuality(t *testing.T) {
	// CRF 定档依据真实产物实测（4 个 ComfyUI 样本 × 4 个档位，共 16 组，与 ImageMagick
	// 同档质量对比）：16/16 组画质不低于 ImageMagick、14/16 组高出 0.6dB 以上、最低仍高
	// 0.14dB，且 16/16 组编码更快（全尺寸最多省约 1100ms）。
	// 延迟预算分析：局域网传输仅占单张总延迟的 0.2%-3.2%，体积换不来有意义的延迟收益，
	// 因此不存在"用画质换延迟"的必要，应取高画质侧
	assert.Equal(t, 10, svtav1CRF, "crf must stay on the high-quality side calibrated against real products")
}

func TestProcess_WritesOutput(t *testing.T) {
	payload := []byte("fake-avif-data")
	runner := &fakeRunner{payload: payload}
	tempDir := t.TempDir()
	p := NewEncoder(runner, tempDir)

	spec, err := transcode.NewSpec(1024, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, p.Process(context.Background(), "src/a.png", spec, &out))
	assert.Equal(t, payload, out.Bytes())
	assert.Equal(t, 1, runner.calls)

	// 临时文件用后即删
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "temp files must be removed after transcode")
}

func TestProcess_ErrorIncludesStderr(t *testing.T) {
	runner := &fakeRunner{err: errors.New("exit status 1"), stderr: "encoder failed"}
	p := NewEncoder(runner, t.TempDir())

	spec, err := transcode.NewSpec(0, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	var out bytes.Buffer
	err = p.Process(context.Background(), "src/a.png", spec, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encoder failed")
}

func TestDetect_SpawnsValidationEncode(t *testing.T) {
	runner := &fakeRunner{payload: []byte("ok")}
	encoder, err := DetectWithRunner(runner, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, encoder)
	assert.Equal(t, encoderSVTAV1, encoder.ID())
	assert.Equal(t, 1, runner.calls, "detection must run one validation encode")
}

func TestDetect_Unavailable(t *testing.T) {
	runner := &fakeRunner{err: errors.New("exit status 1"), stderr: "avif muxer missing"}
	encoder, err := DetectWithRunner(runner, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "avif muxer missing", "probe error must carry ffmpeg stderr")
	assert.Nil(t, encoder, "unavailable ffmpeg must return nil encoder")
}

func TestDetectWithRunner_ProbeEnforcesOverwriteAndNoStdin(t *testing.T) {
	// 真实 ffmpeg 在输出文件已存在且缺少 -y 时会提示 Overwrite? 并在 stdin EOF 后
	// 失败退出；缺少 -nostdin 时则可能在无终端环境阻塞等待输入。探测命令必须像真实
	// 转码一样带上这两个参数，否则 CreateTemp 预建的空输出文件会让探测误判为不可用。
	runner := &interactiveGuardRunner{}
	tempDir := t.TempDir()

	encoder, err := DetectWithRunner(runner, tempDir)
	require.NoError(t, err, "probe must overwrite preexisting output and stay non-interactive")
	require.NotNil(t, encoder)
	assert.Equal(t, 1, runner.calls, "detection must run one validation encode")

	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "probe temp file must be removed after detection")
}

func TestDetectWithRunner_CreateTempFailureReturnsDistinctError(t *testing.T) {
	// 传入不存在的目录，稳定触发探测临时文件创建失败
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	encoder, err := DetectWithRunner(&fakeRunner{payload: []byte("ok")}, missingDir)
	require.Error(t, err)
	assert.Nil(t, encoder)
	assert.Contains(t, err.Error(), "create avif probe temp file", "temp-dir failure must be distinguishable from ffmpeg failure")
}

func TestDetect_WarnCarriesProbeError(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	runner := &fakeRunner{err: errors.New("exit status 1"), stderr: "avif muxer missing"}

	encoder := detectWith(zap.New(core), runner, t.TempDir())
	assert.Nil(t, encoder)

	logs := observed.FilterMessage("ffmpeg avif encoder not available, fallback to imagemagick for avif transcoding").All()
	require.Len(t, logs, 1)
	assert.Contains(t, fmt.Sprint(logs[0].ContextMap()["error"]), "avif muxer missing", "warn must carry ffmpeg stderr for diagnosis")
}

func TestDetect_WarnDistinguishesTempDirFailure(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")

	encoder := detectWith(zap.New(core), &fakeRunner{}, missingDir)
	assert.Nil(t, encoder)

	logs := observed.FilterMessage("ffmpeg avif encoder not available, fallback to imagemagick for avif transcoding").All()
	require.Len(t, logs, 1)
	assert.Contains(t, fmt.Sprint(logs[0].ContextMap()["error"]), "create avif probe temp file", "warn must distinguish temp-dir failure from ffmpeg failure")
}

// #region 真实编码器集成测试（本机存在 ffmpeg 时运行）

func realFFmpegAvailable(t *testing.T) bool {
	t.Helper()
	encoder, err := DetectWithRunner(procRunner{}, t.TempDir())
	return err == nil && encoder != nil
}

func TestProcess_RealFFmpeg_ScaledOutputWidth(t *testing.T) {
	if !realFFmpegAvailable(t) {
		t.Skip("real ffmpeg not available")
	}
	src := filepath.Join(t.TempDir(), "source.png")
	createTestImage(t, src)

	p := NewEncoder(procRunner{}, t.TempDir())
	spec, err := transcode.NewSpec(1024, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, p.Process(context.Background(), src, spec, &out))
	assert.Equal(t, "1024", probeWidth(t, out.Bytes()), "output width should match requested scale")
}

func TestProcess_RealFFmpeg_MarksFullColorRange(t *testing.T) {
	if !realFFmpegAvailable(t) {
		t.Skip("real ffmpeg not available")
	}
	src := filepath.Join(t.TempDir(), "source.png")
	createTestImage(t, src)

	p := NewEncoder(procRunner{}, t.TempDir())
	spec, err := transcode.NewSpec(512, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, p.Process(context.Background(), src, spec, &out))

	// 产物必须标记全范围，与 magick 输出一致（否则黑位被压缩）
	outFile := filepath.Join(t.TempDir(), "result.avif")
	require.NoError(t, os.WriteFile(outFile, out.Bytes(), 0o644))
	probe := exec.Command("ffprobe", "-hide_banner", "-loglevel", "error",
		"-show_entries", "stream=color_range", "-of", "csv=p=0", outFile)
	var stdout, stderr bytes.Buffer
	probe.Stdout = &stdout
	probe.Stderr = &stderr
	require.NoError(t, probe.Run(), "ffprobe stderr: %s", stderr.String())
	assert.Equal(t, "pc", strings.TrimSpace(stdout.String()), "output must be full color range")
}

// createTestImage 用 magick 生成 4K 不透明测试图
func createTestImage(t *testing.T, path string) {
	t.Helper()
	runMagick(t, []string{"-size", "1200x800", "gradient:blue-red", path})
}

// probeWidth 用 ffprobe 读取产物宽度
func probeWidth(t *testing.T, data []byte) string {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "probe.avif")
	require.NoError(t, os.WriteFile(outFile, data, 0o644))
	cmd := exec.Command("ffprobe", "-hide_banner", "-loglevel", "error",
		"-show_entries", "stream=width", "-of", "csv=p=0", outFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "ffprobe stderr: %s", stderr.String())
	return strings.TrimSpace(stdout.String())
}

func runMagick(t *testing.T, args []string) {
	t.Helper()
	cmd := exec.Command("magick", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("magick generate test image: %v: %s", err, stderr.String())
	}
}

// #endregion

// 编译期确认 fakeRunner 仍满足端口
var _ CommandRunner = (*fakeRunner)(nil)
var _ CommandRunner = (*interactiveGuardRunner)(nil)
var _ io.Writer = (*bytes.Buffer)(nil)

// fakeRunner 测试用命令执行器：执行成功时把 payload 写入 args 最后一项
// 指定的输出文件，模拟真实编码产出
type fakeRunner struct {
	payload []byte
	stderr  string
	err     error
	mu      sync.Mutex
	calls   int
}

func (f *fakeRunner) Run(_ context.Context, args []string) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return fmt.Errorf("ffmpeg error: %w: stderr: %q", f.err, f.stderr)
	}
	if len(args) == 0 {
		return nil
	}
	return os.WriteFile(args[len(args)-1], f.payload, 0o644)
}

// interactiveGuardRunner 模拟真实 ffmpeg 在无终端环境下对已存在输出文件的处理：
// 缺少 -y 时 Overwrite? 提示读到 EOF 后失败，缺少 -nostdin 时会阻塞等待 stdin。
// 仅用于锁定探测命令的覆盖/无交互语义。
type interactiveGuardRunner struct {
	calls int
}

func (r *interactiveGuardRunner) Run(_ context.Context, args []string) error {
	r.calls++
	if len(args) == 0 {
		return nil
	}
	outPath := args[len(args)-1]
	if _, err := os.Stat(outPath); err == nil && !hasArg(args, "-y") {
		return fmt.Errorf("ffmpeg error: %w: stderr: %q", errors.New("exit status 1"), "Not overwriting - exiting")
	}
	if !hasArg(args, "-nostdin") {
		return errors.New("ffmpeg would block waiting for stdin without -nostdin")
	}
	return os.WriteFile(outPath, []byte("fake-avif-data"), 0o644)
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
