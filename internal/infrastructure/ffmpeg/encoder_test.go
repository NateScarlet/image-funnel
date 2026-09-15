package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	appimage "main/internal/application/image"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildArgs_Scaled(t *testing.T) {
	spec, err := appimage.NewSpec(2048, appimage.ImageFormatAVIF)
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
	spec, err := appimage.NewSpec(0, appimage.ImageFormatAVIF)
	require.NoError(t, err)

	args := buildArgs("src/img.png", "out/result.avif", spec)
	joined := strings.Join(args, " ")

	assert.NotContains(t, joined, "scale=")
	assert.Contains(t, joined, "-f avif")
}

func TestBuildArgs_PreservesFullColorRange(t *testing.T) {
	// magick 输出标记 color_range=pc（全范围）；ffmpeg 默认 tv（受限范围）会让
	// 黑位与对比度被压缩。实测此标记影响 PSNR 约 0.6dB，必须显式设为 pc
	spec, err := appimage.NewSpec(1024, appimage.ImageFormatAVIF)
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

	spec, err := appimage.NewSpec(1024, appimage.ImageFormatAVIF)
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

	spec, err := appimage.NewSpec(0, appimage.ImageFormatAVIF)
	require.NoError(t, err)

	var out bytes.Buffer
	err = p.Process(context.Background(), "src/a.png", spec, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encoder failed")
}

func TestDetect_SpawnsValidationEncode(t *testing.T) {
	runner := &fakeRunner{payload: []byte("ok")}
	encoder := DetectWithRunner(runner, t.TempDir())
	require.NotNil(t, encoder)
	assert.Equal(t, encoderSVTAV1, encoder.ID())
	assert.Equal(t, 1, runner.calls, "detection must run one validation encode")
}

func TestDetect_Unavailable(t *testing.T) {
	runner := &fakeRunner{err: errors.New("executable file not found")}
	encoder := DetectWithRunner(runner, t.TempDir())
	assert.Nil(t, encoder, "unavailable ffmpeg must return nil encoder")
}

// #region 真实编码器集成测试（本机存在 ffmpeg 时运行）

func realFFmpegAvailable(t *testing.T) bool {
	t.Helper()
	return DetectWithRunner(procRunner{}, t.TempDir()) != nil
}

func TestProcess_RealFFmpeg_ScaledOutputWidth(t *testing.T) {
	if !realFFmpegAvailable(t) {
		t.Skip("real ffmpeg not available")
	}
	src := filepath.Join(t.TempDir(), "source.png")
	createTestImage(t, src)

	p := NewEncoder(procRunner{}, t.TempDir())
	spec, err := appimage.NewSpec(1024, appimage.ImageFormatAVIF)
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
	spec, err := appimage.NewSpec(512, appimage.ImageFormatAVIF)
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
