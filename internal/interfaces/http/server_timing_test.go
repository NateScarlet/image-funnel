package http

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	appimage "main/internal/application/image"
	"main/internal/shared/transcode"
)

func TestFormatServerTiming(t *testing.T) {
	got := FormatServerTiming(ServerTiming{
		QueueMs:  12.34,
		DecodeMs: 0.5,
		EncodeMs: 88,
		CacheHit: true,
		Encoder:  "svt-av1",
	})
	assert.Equal(t,
		`queue;dur=12.34, decode;dur=0.50, encode;dur=88.00;desc="svt-av1", cache;dur=0;desc="hit"`,
		got,
	)
}

func TestFormatServerTimingCacheMiss(t *testing.T) {
	got := FormatServerTiming(ServerTiming{CacheHit: false, Encoder: "magick"})
	assert.Contains(t, got, `cache;dur=0;desc="miss"`)
	assert.Contains(t, got, `encode;dur=0.00;desc="magick"`)
}

func TestFormatServerTimingEscapesDesc(t *testing.T) {
	got := FormatServerTiming(ServerTiming{Encoder: `a"b\c`})
	assert.Contains(t, got, `desc="a\"b\\c"`)
}

func TestHandleImageSetsServerTimingHeader(t *testing.T) {
	logger := zap.NewNop()

	// 注入窄端口测试替身：返回一个转码结果，验证各分段耗时与编码器描述被写入响应头
	fake := newFakeVariantRetriever(func(context.Context, appimage.VariantInput) (appimage.VariantResult, error) {
		return transcodeResult(transcode.ImageFormatWebP, "fake-webp"), nil
	})
	fake.encoderID = "test-encoder"

	signer, rootDir, relPath := testSetup(t)
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/webp")

	rr := httptest.NewRecorder()
	testHandler(logger, signer, fake, rootDir).ServeHTTP(rr, req)

	header := rr.Header().Get("Server-Timing")
	require.NotEmpty(t, header, "每张图片响应都应带分段耗时")
	assert.Contains(t, header, "queue;dur=")
	assert.Contains(t, header, "decode;dur=")
	assert.Contains(t, header, "encode;dur=")
	assert.Contains(t, header, `desc="test-encoder"`)
	assert.Contains(t, header, `cache;dur=0;desc="miss"`)
}
