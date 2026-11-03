package image

import (
	"bytes"
	"context"
	stdimage "image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubMeta 供探针在本地无法解码的格式上回退取宽度
type stubMeta struct {
	width int
}

func (m stubMeta) Meta(_ context.Context, _ string) (*shared.ImageMeta, error) {
	return &shared.ImageMeta{Width: m.width, Height: m.width}, nil
}

func TestSourceProbe_ReturnsWidthContentTypeAndReplayableReader(t *testing.T) {
	path := writeProbePNG(t, 40, 30)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	source, err := NewSourceProbe(stubMeta{width: 1}).Probe(context.Background(), path)
	require.NoError(t, err)
	defer source.Reader.Close()

	assert.Equal(t, 40, source.Width)
	assert.Equal(t, "image/png", source.ContentType)

	// 嗅探推进了句柄偏移，探针必须回绕，使调用方从文件开头读取
	replayed, err := io.ReadAll(source.Reader)
	require.NoError(t, err)
	assert.Equal(t, data, replayed)
}

func TestSourceProbe_OpenSkipsWidthDecoding(t *testing.T) {
	// raw 路径只需 open + 嗅探：可嗅探为 JPEG 但无法解码尺寸的截断文件必须仍能打开
	path := writeTruncatedJPEG(t)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	source, err := NewSourceProbe(stubMeta{width: 1}).Open(path)
	require.NoError(t, err)
	defer source.Reader.Close()

	assert.Equal(t, "image/jpeg", source.ContentType)
	assert.Zero(t, source.Width)

	got, err := io.ReadAll(source.Reader)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestSourceProbe_TruncatedSourceWidthError(t *testing.T) {
	// Probe 需要宽度；截断源无法解码时必须显式失败，而不是猜测宽度
	_, err := NewSourceProbe(stubMeta{width: 1}).Probe(context.Background(), writeTruncatedJPEG(t))
	require.Error(t, err)
}

func TestSourceProbe_FallsBackToMetaForNonDecodableFormat(t *testing.T) {
	// 本地无解码器的格式（如 AVIF）宽度来自注入的 Meta 端口，MIME 仍由嗅探得出
	path := writeFakeAVIF(t)

	source, err := NewSourceProbe(stubMeta{width: 640}).Probe(context.Background(), path)
	require.NoError(t, err)
	defer source.Reader.Close()

	assert.Equal(t, 640, source.Width)
	assert.Equal(t, "image/avif", source.ContentType)
}

func TestSourceProbe_MissingFile(t *testing.T) {
	_, err := NewSourceProbe(stubMeta{width: 1}).Probe(context.Background(), filepath.Join(t.TempDir(), "missing.png"))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func writeProbePNG(t *testing.T, width, height int) string {
	t.Helper()
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xFF})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	path := filepath.Join(t.TempDir(), "probe.png")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
	return path
}

// writeTruncatedJPEG 写入可被嗅探为 JPEG、但缺少可解码帧信息的截断文件
func writeTruncatedJPEG(t *testing.T) string {
	t.Helper()
	jpegHeader := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
	}
	path := filepath.Join(t.TempDir(), "truncated.jpg")
	require.NoError(t, os.WriteFile(path, jpegHeader, 0o644))
	return path
}

// writeFakeAVIF 写入带 AVIF 扩展名的占位文件：嗅探为 octet-stream 后由扩展名判定为 image/avif
func writeFakeAVIF(t *testing.T) string {
	t.Helper()
	data := []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00avif")
	path := filepath.Join(t.TempDir(), "photo.avif")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}
