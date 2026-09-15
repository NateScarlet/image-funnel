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

// signedRequest 使用 signer 生成有效签名的请求。
//
// 以相对目标构造请求，使 RequestURI 与真实浏览器发送的 origin-form 一致
// （签名校验读取 RequestURI 中的原始字形）
//
// acceptHeader: 请求的 Accept 头
func signedRequest(t *testing.T, signer *urlconv.Signer, rootDir, relPath string, opts []appimage.SignOption, acceptHeader string) *http.Request {
	t.Helper()
	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath), opts...)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", signedURL.String(), nil)
	if acceptHeader != "" {
		req.Header.Set("Accept", acceptHeader)
	}
	return req
}

// rawURLRequest 生成 rawURL（原图直通）对应的请求
func rawURLRequest(t *testing.T, signer *urlconv.Signer, rootDir, relPath, acceptHeader string) *http.Request {
	t.Helper()
	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath), appimage.WithRaw())
	require.NoError(t, err)

	req := httptest.NewRequest("GET", signedURL.String(), nil)
	if acceptHeader != "" {
		req.Header.Set("Accept", acceptHeader)
	}
	return req
}

// tamperedRequest 在合法 URL 上篡改查询串，用于断言签名失效
func tamperedRequest(t *testing.T, signer *urlconv.Signer, rootDir, relPath string, mutate func(url.Values)) *http.Request {
	t.Helper()
	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath))
	require.NoError(t, err)

	u, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := u.Query()
	mutate(q)
	u.RawQuery = q.Encode()

	return httptest.NewRequest("GET", u.String(), nil)
}

func TestHandleImage_NotFound(t *testing.T) {
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	coordinator := newTestCoordinator(os.ErrNotExist)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "")
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

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "")
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

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "")
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

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/avif, image/webp")
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

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/webp")
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
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/webp;q=0.9, image/avif;q=0.8")
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

func TestHandleImage_TranscodesWhenResizeNeeded(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// 请求 width=1024 (< 源图 2048) 需要缩放，所以不会返回原图
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/jpeg")
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
	req := signedRequest(t, signer, rootDir, relPath, nil, "image/jpeg, image/webp")
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

	// rawURL：原图直通，不参与宽高变体
	req := rawURLRequest(t, signer, rootDir, relPath, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	// raw 返回原图，Content-Type 应为检测到的类型
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg for raw, got %q", got)
	}
}

func TestHandleImage_RawWithWidthReturns400(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// raw 与 width 都是已签名参数，同时出现属生成侧错误（宽高变体 vs 原图）
	req := signedRequest(t, signer, rootDir, relPath,
		[]appimage.SignOption{appimage.WithWidth(1024), appimage.WithRaw()}, "")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for raw with width, got %d", rr.Code)
	}
}

func TestHandleImage_MutatedParamReturns403(t *testing.T) {
	// 签名覆盖签名之后的整个原始字符串，因此任何未参与生成的参数都会使签名失效。
	// 这取代了此前对 fmt/q 的显式 400 白名单：无需枚举非法参数
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	cases := map[string]func(url.Values){
		"追加 q":    func(v url.Values) { v.Set("q", "80") },
		"追加 fmt":  func(v url.Values) { v.Set("fmt", "avif") },
		"追加未知参数":  func(v url.Values) { v.Set("unknown_param", "x") },
		"篡改已签名 w": func(v url.Values) { v.Set("w", "4096") },
		"追加裸 raw": func(v url.Values) { v.Set("raw", "") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := tamperedRequest(t, signer, rootDir, relPath, mutate)
			rr := httptest.NewRecorder()

			handler := handleImage(logger, signer, coordinator, rootDir)
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("expected status 403 for tampered url, got %d", rr.Code)
			}
		})
	}
}

func TestHandleImage_PathTraversalReturns403(t *testing.T) {
	// 纵深防御：签名只证明 URL 由本服务签发，不证明目标位于根目录内。
	// 签发侧已拒绝越界路径（见 urlconv 的签名测试），此处用同一密钥手工构造
	// 「签名有效但路径越界」的请求，验证 handler 的围栏独立生效
	//
	// 说明：含 .. 的路径会被 mux 的 cleanPath 提前 301（fail-closed），
	// 因此这里选用能真正到达 handler 的形式——Windows 盘符相对路径
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, _ := testSetup(t, logger, coordinator)

	cases := map[string]string{
		"Windows 盘符相对路径": "C:/Windows/win.ini",
		"Windows 盘符反斜杠":  "C:\\Windows\\win.ini",
		"UNC 路径":         "//server/share/x.jpg",
	}
	for name, escaped := range cases {
		t.Run(name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			handler := handleImage(logger, signer, coordinator, rootDir)
			handler.ServeHTTP(rr, forgeSignedRequest(t, "test-secret", escaped))

			if rr.Code != http.StatusForbidden {
				t.Errorf("expected status 403 for out-of-root path %q, got %d", escaped, rr.Code)
			}
		})
	}
}

// forgeSignedRequest 用已知密钥为任意路径构造签名有效的请求，
// 用于验证「签名之外」的防御（签名本身无法覆盖目标是否在根目录内）
func forgeSignedRequest(t *testing.T, secret, escapedPath string) *http.Request {
	t.Helper()

	signed := escapedPath + "?t=1&s=1"
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s", signed)
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return httptest.NewRequest("GET", urlconv.ImageURLPrefix+sig+"/"+signed, nil)
}

func TestHandleImage_HeadIsValidatedAndRegistersDemand(t *testing.T) {
	// 前端两阶段预载用 HEAD 登记需求，GET 取回内容，两者共用同一签名 URL。
	// 验签前置于所有分支后，HEAD 也必须通过验签
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := handleImage(logger, signer, coordinator, rootDir)

	t.Run("合法 HEAD 返回 200", func(t *testing.T) {
		req := signedRequest(t, signer, rootDir, relPath,
			[]appimage.SignOption{appimage.WithWidth(256)}, "image/webp")
		req.Method = http.MethodHead
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200 for valid HEAD, got %d", rr.Code)
		}
	})

	t.Run("无签名 HEAD 返回 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodHead, urlconv.ImageURLPrefix+"bogus/"+relPath, nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status 403 for unsigned HEAD, got %d", rr.Code)
		}
	})
}

func TestHandleImage_NotModifiedRequiresValidSignature(t *testing.T) {
	// 304 短路必须位于验签之后，否则无签名请求也能探得资源存在
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := handleImage(logger, signer, coordinator, rootDir)

	t.Run("合法签名得到 304", func(t *testing.T) {
		req := signedRequest(t, signer, rootDir, relPath, nil, "image/jpeg")
		req.Header.Set("If-None-Match", `"immutable"`)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotModified {
			t.Errorf("expected status 304, got %d", rr.Code)
		}
	})

	t.Run("无签名即使带 If-None-Match 也返回 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, urlconv.ImageURLPrefix+"bogus/"+relPath, nil)
		req.Header.Set("If-None-Match", `"immutable"`)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status 403 before 304 short-circuit, got %d", rr.Code)
		}
	})
}

func TestHandleImage_ReturnsOriginalRegardlessOfSourceWidth(t *testing.T) {
	// 直出原图的判据只看宽度：width 未超过源图宽度即无需缩放，直接返回原图。
	// 源图 MIME 必须在 Accept 内，否则仍需转码
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// stubProcessor.Meta 报告源图宽度 2048；请求 4096 超过源图 → 无需缩放
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(4096)}, "image/jpeg")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg (original), got %q", got)
	}
}

func TestHandleImage_TranscodesWhenSourceNotAcceptable(t *testing.T) {
	// 无需缩放但源图 MIME 不被接受时仍需转码（例如 Accept 只收 avif）
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

	// 源图是 JPEG，Accept 只要 avif → 必须转码
	req := signedRequest(t, signer, rootDir, relPath, nil, "image/avif")
	rr := httptest.NewRecorder()

	handler := handleImage(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if capturedFormat != appimage.ImageFormatAVIF {
		t.Errorf("expected AVIF transcode when source MIME not accepted, got %v", capturedFormat)
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

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "")
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
