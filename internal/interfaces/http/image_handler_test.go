package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	appimage "main/internal/application/image"
	"main/internal/infrastructure/urlconv"
	"main/internal/shared"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// stubProcessor 模拟纯转码处理器（供 coordinator 使用）
type stubProcessor struct {
	processFunc func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error
}

func (m *stubProcessor) Process(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
	return m.processFunc(ctx, srcPath, spec, w)
}

func (m *stubProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	// 返回固定尺寸用于测试
	return &shared.ImageMeta{Width: 2048, Height: 1536}, nil
}

// stubCache 模拟缓存：可预设已存在条目
type stubCache struct {
	mu    sync.Mutex
	files map[string]appimage.File
}

func newStubCache() *stubCache {
	return &stubCache{files: make(map[string]appimage.File)}
}

func (c *stubCache) Lookup(_ context.Context, key string) (appimage.File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f, ok := c.files[key]; ok {
		return f, nil
	}
	return nil, nil
}

func (c *stubCache) Save(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[key] = &stubFile{data: data}
	return nil
}

// stubQueue 模拟队列：Enqueue 即同步执行并 resolve（模拟 worker 立即处理）
type stubQueue struct {
	processFunc func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error
}

func (q *stubQueue) Enqueue(ctx context.Context, item appimage.TranscodeItem) (appimage.TranscodeWaiter, error) {
	w := &stubWaiter{done: make(chan struct{})}
	var buf bytes.Buffer
	err := q.processFunc(ctx, item.SrcPath(), item.Spec(), &buf)
	if err == nil {
		w.file = &stubFile{data: buf.Bytes()}
	}
	w.err = err
	close(w.done)
	return w, nil
}

func (q *stubQueue) Consume(ctx context.Context, _ appimage.Prio) iter.Seq2[appimage.TranscodeJob, error] {
	return func(yield func(appimage.TranscodeJob, error) bool) {
		<-ctx.Done()
	}
}

type stubWaiter struct {
	done chan struct{}
	file appimage.File
	err  error
}

func (w *stubWaiter) Wait(ctx context.Context) (appimage.File, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.done:
	}
	return w.file, w.err
}

func (w *stubWaiter) Cancel() {}

// stubFile 模拟缓存文件
type stubFile struct {
	data []byte
}

func (f *stubFile) Open() (io.ReadSeekCloser, error) {
	return &stubReadSeekCloser{bytes.NewReader(f.data)}, nil
}

type stubReadSeekCloser struct {
	*bytes.Reader
}

func (m *stubReadSeekCloser) Close() error { return nil }

// calculateTestSignature 计算测试用签名（不包含 format，包含 raw）
func calculateTestSignature(secret []byte, relPath, timestamp, size, w, q, raw string) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%s|%s|%s|%s", relPath, timestamp, size, w, q, raw)
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

// newTestCoordinator 构造挂接 stub 依赖的 coordinator（缓存为空、队列同步执行）
func newTestCoordinator(processErr error) *appimage.TranscodeCoordinator {
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		if processErr != nil {
			return processErr
		}
		_, err := w.Write([]byte("fake-image"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	return appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)
}

// testSetup 创建测试所需的通用组件，返回 signer、rootDir、relPath
func testSetup(t *testing.T, logger *zap.Logger, coordinator *appimage.TranscodeCoordinator) (*urlconv.Signer, string, string) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)

	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	return signer, rootDir, relPath
}

// signedRequest 使用 signer 生成有效签名的请求
// opts: 传递给 GenerateSignedURL 的选项（如 WithWidth, WithQuality）
// extraParams: 生成 URL 后额外添加/修改的查询参数（如 raw=true, 或覆盖 w/q）
// acceptHeader: 请求的 Accept 头
func signedRequest(t *testing.T, signer *urlconv.Signer, rootDir, relPath string, opts []appimage.SignOption, extraParams map[string]string, acceptHeader string) *http.Request {
	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath), opts...)
	require.NoError(t, err)

	parsedURL, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := parsedURL.Query()

	// 应用额外参数
	for k, v := range extraParams {
		q.Set(k, v) // 空字符串也设置，表示参数存在但无值
	}
	parsedURL.RawQuery = q.Encode()

	// httptest.NewRequest 需要完整的 URL（包含 scheme 和 host）
	fullURL := "http://localhost" + parsedURL.String()
	req := httptest.NewRequest("GET", fullURL, nil)
	if acceptHeader != "" {
		req.Header.Set("Accept", acceptHeader)
	}
	return req
}

func TestHandleImage_NotFound(t *testing.T) {
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	coordinator := newTestCoordinator(os.ErrNotExist)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}

	if recorded.Len() > 0 {
		t.Errorf("expected no error logs, but got %d", recorded.Len())
		for _, log := range recorded.All() {
			t.Logf("logged error: %s", log.Message)
		}
	}
}

func TestHandleImage_Canceled(t *testing.T) {
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	coordinator := newTestCoordinator(context.Canceled)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestTimeout {
		t.Errorf("expected status 408, got %d", rr.Code)
	}

	if recorded.Len() > 0 {
		t.Errorf("expected no error logs for canceled request, but got %d", recorded.Len())
	}
}

func TestHandleImage_OtherError(t *testing.T) {
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	expectedErr := errors.New("some processor internal error")
	coordinator := newTestCoordinator(expectedErr)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rr.Code)
	}

	if recorded.Len() != 1 {
		t.Errorf("expected exactly 1 error log, but got %d", recorded.Len())
	} else {
		log := recorded.All()[0]
		if log.Message != "process image" {
			t.Errorf("expected log message 'process image', got %s", log.Message)
		}
	}
}

// #region Accept 头协商测试

func TestHandleImage_AcceptAVIF(t *testing.T) {
	logger := zap.NewNop()

	var capturedFormat appimage.ImageFormat
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		capturedFormat = spec.Format()
		_, err := w.Write([]byte("fake-avif"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "image/avif, image/webp")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if capturedFormat != appimage.ImageFormatAVIF {
		t.Errorf("expected format AVIF for Accept: image/avif, got %v", capturedFormat)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/avif" {
		t.Errorf("expected Content-Type image/avif, got %q", got)
	}
	if got := rr.Header().Get("Vary"); got != "Accept" {
		t.Errorf("expected Vary: Accept, got %q", got)
	}
}

func TestHandleImage_AcceptWebP(t *testing.T) {
	logger := zap.NewNop()

	var capturedFormat appimage.ImageFormat
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		capturedFormat = spec.Format()
		_, err := w.Write([]byte("fake-webp"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "image/webp")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if capturedFormat != appimage.ImageFormatWebP {
		t.Errorf("expected format WebP for Accept: image/webp, got %v", capturedFormat)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %q", got)
	}
}

func TestHandleImage_AcceptWithQValues(t *testing.T) {
	logger := zap.NewNop()

	var capturedFormat appimage.ImageFormat
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		capturedFormat = spec.Format()
		_, err := w.Write([]byte("fake-avif"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// WebP has higher q-value, should be preferred
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "image/webp;q=0.9, image/avif;q=0.8")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if capturedFormat != appimage.ImageFormatWebP {
		t.Errorf("expected format WebP (higher q-value), got %v", capturedFormat)
	}
}

func TestHandleImage_ReturnsOriginalWhenNoResizeAndNoQualityLoss(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// 请求 width=1024 (< 源图 2048) 需要缩放，所以不会返回原图
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "image/jpeg")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	// 需要缩放，所以会转码为 WebP（默认）
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/webp" {
		t.Errorf("expected Content-Type image/webp (transcoded), got %q", got)
	}
}

func TestHandleImage_ReturnsOriginalWhenWidthExceedsSource(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// 源图 2048x1536，不指定 width (w=0) 无需缩放
	req := signedRequest(t, signer, rootDir, relPath, nil, nil, "image/jpeg, image/webp")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	// 无需缩放，无质量压缩，源图格式在 Accept 中 → 返回原图
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg (original), got %q", got)
	}
}

func TestHandleImage_RawTrueReturnsOriginal(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// raw=true，不需要宽度/质量参数
	req := signedRequest(t, signer, rootDir, relPath, nil, map[string]string{"raw": ""}, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	// raw=true 返回原图，Content-Type 应为检测到的类型
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg for raw, got %q", got)
	}
}

func TestHandleImage_RawTrueWithWidthReturns400(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// raw=true 与 w 冲突
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, map[string]string{"raw": ""}, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for raw with width, got %d", rr.Code)
	}
}

func TestHandleImage_RawTrueWithQualityReturns400(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// raw=true 与 q 冲突
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithQuality(80)}, map[string]string{"raw": ""}, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for raw with quality, got %d", rr.Code)
	}
}

func TestHandleImage_FmtParamReturns400(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// fmt 参数已废弃，手动添加到 URL
	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath))
	require.NoError(t, err)

	parsedURL, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := parsedURL.Query()
	q.Set("fmt", "avif")
	parsedURL.RawQuery = q.Encode()

	// httptest.NewRequest 需要完整的 URL
	fullURL := "http://localhost" + parsedURL.String()
	req := httptest.NewRequest("GET", fullURL, nil)
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for fmt param, got %d", rr.Code)
	}
}

func TestHandleImage_DefaultToWebPWhenNoAccept(t *testing.T) {
	logger := zap.NewNop()

	var capturedFormat appimage.ImageFormat
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		capturedFormat = spec.Format()
		_, err := w.Write([]byte("fake-webp"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, nil, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if capturedFormat != appimage.ImageFormatWebP {
		t.Errorf("expected default WebP when no Accept, got %v", capturedFormat)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %q", got)
	}
}

// #endregion

// requireWriteSource 在指定路径写入一个真实源图文件（带有 JPEG 头以便 MIME 检测）
func requireWriteSource(t *testing.T, path string) {
	t.Helper()
	// 最小有效 JPEG 头：SOI + APP0 + 最小数据
	jpegHeader := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
	}
	if err := os.WriteFile(path, jpegHeader, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
}