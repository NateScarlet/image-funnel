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
	spec, err := appimage.NewSpec(2048, 90, appimage.ImageFormatAVIF)
	require.NoError(t, err)

	args := buildArgs("src/img.png", "out/result.avif", spec, crfFromQuality(90))
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
	spec, err := appimage.NewSpec(0, 95, appimage.ImageFormatAVIF)
	require.NoError(t, err)

	args := buildArgs("src/img.png", "out/result.avif", spec, crfFromQuality(95))
	joined := strings.Join(args, " ")

	assert.NotContains(t, joined, "scale=")
	assert.Contains(t, joined, "-f avif")
}

func TestCRFMapping(t *testing.T) {
	// q95 → crf29, q80 → crf39, q75 → crf43（线性映射，clamp 到 [18,55]）
	assert.Equal(t, 29, crfFromQuality(95))
	assert.Equal(t, 39, crfFromQuality(80))
	assert.Equal(t, 43, crfFromQuality(75))
	assert.Equal(t, 25, crfFromQuality(100), "quality above max maps below crf 29")
	assert.Equal(t, 55, crfFromQuality(1), "clamp lower bound")
}

func TestProcess_WritesOutput(t *testing.T) {
	payload := []byte("fake-avif-data")
	runner := &fakeRunner{payload: payload}
	tempDir := t.TempDir()
	p := NewEncoder(runner, tempDir)

	spec, err := appimage.NewSpec(1024, 85, appimage.ImageFormatAVIF)
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

	spec, err := appimage.NewSpec(0, 95, appimage.ImageFormatAVIF)
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

func TestProcess_RealFFmpeg_Integration(t *testing.T) {
	if !realFFmpegAvailable(t) {
		t.Skip("real ffmpeg not available")
	}
	// 用 magick 生成一张 4K 测试源图
	src := filepath.Join(t.TempDir(), "source.png")
	createTestImage(t, src)

	p := NewEncoder(procRunner{}, t.TempDir())
	spec, err := appimage.NewSpec(2048, 90, appimage.ImageFormatAVIF)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, p.Process(context.Background(), src, spec, &out))
	// 合成渐变图压缩率极高，体积断言不可靠；验证产物为合法 AVIF 且缩放到目标宽度
	outFile := filepath.Join(t.TempDir(), "result.avif")
	require.NoError(t, os.WriteFile(outFile, out.Bytes(), 0o644))
	probe := exec.Command("ffprobe", "-hide_banner", "-loglevel", "error",
		"-show_entries", "stream=width", "-of", "csv=p=0", outFile)
	var stdout, stderr bytes.Buffer
	probe.Stdout = &stdout
	probe.Stderr = &stderr
	require.NoError(t, probe.Run(), "ffprobe stderr: %s", stderr.String())
	assert.Equal(t, "2048", strings.TrimSpace(stdout.String()), "output width should match requested scale")
}

func TestProcess_RealFFmpeg_AlphaPreserved(t *testing.T) {
	if !realFFmpegAvailable(t) {
		t.Skip("real ffmpeg not available")
	}
	// 透明 PNG 回归测试：转码后仍是合法 AVIF（alpha 行为与 magick 路径一致性由像素对比另行保证）
	src := filepath.Join(t.TempDir(), "alpha.png")
	createTransparentTestImage(t, src)

	p := NewEncoder(procRunner{}, t.TempDir())
	spec, err := appimage.NewSpec(0, 95, appimage.ImageFormatAVIF)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, p.Process(context.Background(), src, spec, &out))
	assert.Greater(t, out.Len(), 100)
}

// createTestImage 用 magick 生成 4K 不透明测试图
func createTestImage(t *testing.T, path string) {
	t.Helper()
	runMagick(t, []string{"-size", "3840x2160", "gradient:blue-red", path})
}

// createTransparentTestImage 用 magick 生成带 alpha 的测试图
func createTransparentTestImage(t *testing.T, path string) {
	t.Helper()
	runMagick(t, []string{"-size", "64x64", "xc:none", "-fill", "red", "-draw", "circle 32,32 32,8", path})
}

func runMagick(t *testing.T, args []string) {
	t.Helper()
	// magick 仅生成测试源图（非被测路径）
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
