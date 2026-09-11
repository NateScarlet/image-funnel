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
	return nil, nil
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

func calculateTestSignature(secret []byte, relPath, timestamp, size, w, q, format string) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%s|%s|%s|%s", relPath, timestamp, size, w, q, format)
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

func TestHandleImage_NotFound(t *testing.T) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)

	// 创建带 Observer 的 zap Logger，以便验证是否有 Error 级别的日志
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	// 源文件不存在 → coordinator 返回 os.ErrNotExist
	coordinator := newTestCoordinator(os.ErrNotExist)

	// 创建真实源文件（coordinator 需要读取源图元信息）
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	handler := handleImage(logger, signer, coordinator, rootDir)

	// 构建请求参数并签名
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "1024", "", "")

	u, err := url.Parse("/image")
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
	q.Set("w", "1024")
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	req := httptest.NewRequest("GET", u.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// 验证状态码是否为 404
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}

	// 验证未记录错误日志
	if recorded.Len() > 0 {
		t.Errorf("expected no error logs, but got %d", recorded.Len())
		for _, log := range recorded.All() {
			t.Logf("logged error: %s", log.Message)
		}
	}
}

func TestHandleImage_Canceled(t *testing.T) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)

	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	// 模拟返回 context.Canceled 错误
	coordinator := newTestCoordinator(context.Canceled)

	// 创建真实源文件（coordinator 需要读取源图元信息）
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	handler := handleImage(logger, signer, coordinator, rootDir)
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "1024", "", "")

	u, _ := url.Parse("/image")
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
	q.Set("w", "1024")
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	req := httptest.NewRequest("GET", u.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// 验证状态码是否为 408 (Request Timeout)
	if rr.Code != http.StatusRequestTimeout {
		t.Errorf("expected status 408, got %d", rr.Code)
	}

	// 验证未记录错误日志
	if recorded.Len() > 0 {
		t.Errorf("expected no error logs for canceled request, but got %d", recorded.Len())
	}
}

func TestHandleImage_OtherError(t *testing.T) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)

	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	// 模拟返回其他一般错误
	expectedErr := errors.New("some processor internal error")
	coordinator := newTestCoordinator(expectedErr)

	// 创建真实源文件（coordinator 需要读取源图元信息）
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	handler := handleImage(logger, signer, coordinator, rootDir)
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "1024", "", "")

	u, _ := url.Parse("/image")
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
	q.Set("w", "1024")
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	req := httptest.NewRequest("GET", u.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// 验证状态码是否为 500 (Internal Server Error)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rr.Code)
	}

	// 验证已记录 Error 级别的错误日志
	if recorded.Len() != 1 {
		t.Errorf("expected exactly 1 error log, but got %d", recorded.Len())
	} else {
		log := recorded.All()[0]
		if log.Message != "process image" {
			t.Errorf("expected log message 'process image', got %s", log.Message)
		}
	}
}

// #region 格式参数测试

func TestHandleImage_FormatForwardedToProcessor(t *testing.T) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	// 记录传给处理链的格式参数
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

	// 创建真实源文件（coordinator 需要读取源图元信息）
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	handler := handleImage(logger, signer, coordinator, rootDir)
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "", "avif")

	u, _ := url.Parse("/image")
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
	q.Set("fmt", "avif")
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	req := httptest.NewRequest("GET", u.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if capturedFormat != appimage.ImageFormatAVIF {
		t.Errorf("expected format AVIF forwarded to processor, got %v", capturedFormat)
	}
}

func TestHandleImage_SetsContentTypeByFormat(t *testing.T) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	// 创建真实源文件（coordinator 需要读取源图元信息）
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	handler := handleImage(logger, signer, coordinator, rootDir)

	tests := []struct {
		name         string
		formatParam  string
		expectedCT   string
		formatForSig string
	}{
		{"default webp when no format", "", "image/webp", ""},
		{"explicit webp", "webp", "image/webp", "webp"},
		{"explicit avif", "avif", "image/avif", "avif"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relPath := "image.jpg"
			tVal := "1719660000"
			sVal := "1024"
			sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "", tt.formatForSig)

			u, _ := url.Parse("/image")
			q := u.Query()
			q.Set("path", relPath)
			q.Set("t", tVal)
			q.Set("s", sVal)
			q.Set("sig", sig)
			if tt.formatParam != "" {
				q.Set("fmt", tt.formatParam)
			}
			u.RawQuery = q.Encode()

			req := httptest.NewRequest("GET", u.String(), nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
			}
			if got := rr.Header().Get("Content-Type"); got != tt.expectedCT {
				t.Errorf("expected Content-Type %q, got %q", tt.expectedCT, got)
			}
		})
	}
}

func TestHandleImage_InvalidFormatReturns400(t *testing.T) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	// 创建真实源文件（coordinator 需要读取源图元信息）
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	handler := handleImage(logger, signer, coordinator, rootDir)
	tVal := "1719660000"
	sVal := "1024"
	// 使用篡改的格式参数签名
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "", "png")

	u, _ := url.Parse("/image")
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
	q.Set("fmt", "png")
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	req := httptest.NewRequest("GET", u.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleImage_TamperedFormatReturns403(t *testing.T) {
	secret := "test-secret"
	rootDir := t.TempDir()
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	// 创建真实源文件（coordinator 需要读取源图元信息）
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath))

	handler := handleImage(logger, signer, coordinator, rootDir)
	tVal := "1719660000"
	sVal := "1024"
	// 签名按 webp 计算，但请求传 avif
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "", "webp")

	u, _ := url.Parse("/image")
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
	q.Set("fmt", "avif")
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	req := httptest.NewRequest("GET", u.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

// #endregion

// requireWriteSource 在指定路径写入一个真实源图文件
func requireWriteSource(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fake-source-image"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
}