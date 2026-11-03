package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

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
	metaCalls   int
}

func (m *stubProcessor) Process(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
	return m.processFunc(ctx, srcPath, spec, w)
}

func (m *stubProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	m.metaCalls++
	// 返回固定尺寸用于测试
	return &shared.ImageMeta{Width: 2048, Height: 1536}, nil
}

// stubCache 模拟缓存：可预设已存在条目
type stubCache struct {
	mu      sync.Mutex
	files   map[string]appimage.File
	lookups int
}

func newStubCache() *stubCache {
	return &stubCache{files: make(map[string]appimage.File)}
}

func (c *stubCache) Lookup(_ context.Context, key string) (appimage.File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookups++
	if f, ok := c.files[key]; ok {
		return f, nil
	}
	return nil, nil
}

// lookupCount 已发生的变体缓存查询次数
func (c *stubCache) lookupCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lookups
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
	enqueues    int
}

func (q *stubQueue) Enqueue(ctx context.Context, item appimage.TranscodeItem) (appimage.TranscodeWaiter, error) {
	q.enqueues++
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

// newCountingCoordinator 构造挂接可计数 stub 的 coordinator，
// 用于断言「请求未查询变体缓存、未登记转码」
func newCountingCoordinator() (coordinator *appimage.TranscodeCoordinator, cache *stubCache, queue *stubQueue, processor *stubProcessor) {
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		_, err := w.Write([]byte("fake-image"))
		return err
	}
	cache = newStubCache()
	queue = &stubQueue{processFunc: processFn}
	processor = &stubProcessor{processFunc: processFn}
	return appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil), cache, queue, processor
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
	requireWriteSource(t, filepath.Join(rootDir, relPath), 2048, 1536)

	return signer, rootDir, relPath
}

// stubMetaReader 提供固定的源图宽度，用于非标准库可解码格式的宽度回退
type stubMetaReader struct {
	width int
}

func (m stubMetaReader) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	return &shared.ImageMeta{Width: m.width, Height: m.width}, nil
}

// countingProbe 包装真实探针并记录 Open/Probe 调用次数
type countingProbe struct {
	inner  appimage.SourceProbe
	opens  int
	probes int
}

func (p *countingProbe) Open(absPath string) (appimage.Source, error) {
	p.opens++
	return p.inner.Open(absPath)
}

func (p *countingProbe) Probe(ctx context.Context, absPath string) (appimage.Source, error) {
	p.probes++
	return p.inner.Probe(ctx, absPath)
}

func newCountingProbe() *countingProbe {
	return &countingProbe{inner: newTestProbe()}
}

// newTestProbe 构造带固定宽度回退的真实探针（回退仅在本地不可解码的格式上发生）
func newTestProbe() appimage.SourceProbe {
	return appimage.NewSourceProbe(stubMetaReader{width: 2048})
}

// testHandler 使用真实源图探针构造 handler
func testHandler(
	logger *zap.Logger,
	signer *urlconv.Signer,
	coordinator *appimage.TranscodeCoordinator,
	rootDir string,
) http.HandlerFunc {
	return handleImage(logger, signer, coordinator, newTestProbe(), rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	// 需要缩放，所以会转码为 WebP（默认）
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/webp" {
		t.Errorf("expected Content-Type image/webp (transcoded), got %q", got)
	}
}

func TestHandleImage_FullSizeWithoutWidthRedirectsToRaw(t *testing.T) {
	// 不带 w 且源图 MIME 可接受 → 302 到 raw URL；重定向形状与 rawURL 逐字节一致
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	req := signedRequest(t, signer, rootDir, relPath, nil, "image/jpeg, image/webp")
	rr := httptest.NewRecorder()

	handler := testHandler(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", rr.Code)
	}
	// 重定向决策依赖 Accept，必须保留 Vary
	if got := rr.Header().Get("Vary"); got != "Accept" {
		t.Errorf("expected Vary: Accept, got %q", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("expected immutable Cache-Control, got %q", got)
	}
	if got := rr.Header().Get("ETag"); got != `"immutable"` {
		t.Errorf("expected ETag immutable, got %q", got)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("expected empty redirect body, got %d bytes", rr.Body.Len())
	}

	rawURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath), appimage.WithRaw())
	require.NoError(t, err)
	if got := rr.Header().Get("Location"); got != rawURL.String() {
		t.Errorf("Location 应与 resolver 生成的 rawURL 一致\n got: %q\nwant: %q", got, rawURL.String())
	}

	// 跟随 Location 得到源图字节
	follow := httptest.NewRequest(http.MethodGet, rawURL.String(), nil)
	follow.Header.Set("Accept", "image/jpeg, image/webp")
	rrFollow := httptest.NewRecorder()
	handler.ServeHTTP(rrFollow, follow)

	if rrFollow.Code != http.StatusOK {
		t.Errorf("expected status 200 when following raw URL, got %d", rrFollow.Code)
	}
	if got := rrFollow.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %q", got)
	}
	if got := rrFollow.Header().Get("Vary"); got != "" {
		t.Errorf("raw 响应不应带 Vary, got %q", got)
	}
	require.Equal(t, requireReadFile(t, filepath.Join(rootDir, relPath)), rrFollow.Body.Bytes())
}

func TestHandleImage_RawTrueReturnsOriginal(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// rawURL：原图直通，不参与宽高变体
	req := rawURLRequest(t, signer, rootDir, relPath, "")
	rr := httptest.NewRecorder()

	handler := testHandler(logger, signer, coordinator, rootDir)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	// raw 返回原图，Content-Type 应为检测到的类型
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg for raw, got %q", got)
	}
	// raw 的内容与 Accept 无关，不参与内容协商分叉
	if got := rr.Header().Get("Vary"); got != "" {
		t.Errorf("raw 响应不应带 Vary, got %q", got)
	}
	require.Equal(t, requireReadFile(t, filepath.Join(rootDir, relPath)), rr.Body.Bytes())
}

func TestHandleImage_RawWithWidthReturns400(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)

	// raw 与 width 都是已签名参数，同时出现属生成侧错误（宽高变体 vs 原图）
	req := signedRequest(t, signer, rootDir, relPath,
		[]appimage.SignOption{appimage.WithWidth(1024), appimage.WithRaw()}, "")
	rr := httptest.NewRecorder()

	handler := testHandler(logger, signer, coordinator, rootDir)
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

			handler := testHandler(logger, signer, coordinator, rootDir)
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
	// 「签名有效但路径越界」的请求，验证验签侧返回的根相对路径自带围栏保证：
	// 越界载荷在构造时就失败，handler 拿不到可用的路径，因此只能返回 403
	//
	// 说明：含 .. 的路径会被 mux 的 cleanPath 提前 301（fail-closed），
	// 因此这里选用能真正到达 handler 的形式——UNC 与 Windows 盘符路径
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	signer, rootDir, _ := testSetup(t, logger, coordinator)

	cases := map[string]string{
		"UNC 路径": "//server/share/x.jpg",
	}
	// 盘符只在 Windows 上是卷的一部分，在其他平台它只是根目录内的普通
	// 相对路径（根相对路径按 filepath 语义判定，故同理）
	if runtime.GOOS == "windows" {
		cases["Windows 盘符相对路径"] = "C:/Windows/win.ini"
		cases["Windows 盘符反斜杠"] = "C:\\Windows\\win.ini"
	}
	for name, escaped := range cases {
		t.Run(name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			handler := testHandler(logger, signer, coordinator, rootDir)
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

// forgeQueryRequest 用已知密钥为同一路径构造带任意查询串的签名有效请求，
// 用于测试无法由签发侧正常生成的参数值（如非法 w）
func forgeQueryRequest(t *testing.T, secret, relPath, rawQuery string) *http.Request {
	t.Helper()

	payload := relPath
	if rawQuery != "" {
		payload += "?" + rawQuery
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s", payload)
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return httptest.NewRequest(http.MethodGet, urlconv.ImageURLPrefix+sig+"/"+payload, nil)
}

func TestHandleImage_HeadIsValidatedAndRegistersDemand(t *testing.T) {
	// 前端两阶段预载用 HEAD 登记需求，GET 取回内容，两者共用同一签名 URL。
	// 验签前置于所有分支后，HEAD 也必须通过验签
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)

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
	handler := testHandler(logger, signer, coordinator, rootDir)

	t.Run("合法签名得到 304", func(t *testing.T) {
		req := signedRequest(t, signer, rootDir, relPath, nil, "image/jpeg")
		req.Header.Set("If-None-Match", `"immutable"`)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotModified {
			t.Errorf("expected status 304, got %d", rr.Code)
		}
		// 非 raw 的 304 仍依赖 Accept，必须保留 Vary
		if got := rr.Header().Get("Vary"); got != "Accept" {
			t.Errorf("expected Vary: Accept on 304, got %q", got)
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

func TestHandleImage_WidthAboveSourceRedirectsToRaw(t *testing.T) {
	// w >= 源宽 且源图 MIME 可接受 → 302 到 raw URL；不返回图片字节、不写缓存，
	// 且全过程只打开源图一次
	logger := zap.NewNop()

	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		_, err := w.Write([]byte("fake-image"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	probe := newCountingProbe()
	handler := handleImage(logger, signer, coordinator, probe, rootDir)

	// 源图 2048 宽；请求 4096 超过源图
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(4096)}, "image/jpeg")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", rr.Code)
	}
	if probe.probes != 1 {
		t.Errorf("expected exactly 1 probe (single open), got %d", probe.probes)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("expected empty redirect body, got %d bytes", rr.Body.Len())
	}
	cache.mu.Lock()
	cached := len(cache.files)
	cache.mu.Unlock()
	if cached != 0 {
		t.Errorf("redirect must not write variant cache, got %d entries", cached)
	}

	rawURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath), appimage.WithRaw())
	require.NoError(t, err)
	if got := rr.Header().Get("Location"); got != rawURL.String() {
		t.Errorf("Location 应与 rawURL 一致\n got: %q\nwant: %q", got, rawURL.String())
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

	handler := testHandler(logger, signer, coordinator, rootDir)
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

func TestHandleImage_ExplicitlyRejectedSourceIsNotServed(t *testing.T) {
	// q=0 是明文拒绝：即使通配符也在列表里，源图 MIME 也不得被判为可接受（RFC 9110 §12.5.1）
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)

	for _, accept := range []string{"image/jpeg;q=0, image/*", "image/jpeg;q=0, */*"} {
		t.Run(accept, func(t *testing.T) {
			// 不带 width → 无需缩放，命中的是「源图能否直出」这一判定
			req := signedRequest(t, signer, rootDir, relPath, nil, accept)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
			}
			// 通配符使 avif/webp 同 q 可接受，按 ADR 0010 取 AVIF
			if got := rr.Header().Get("Content-Type"); got != "image/avif" {
				t.Errorf("Accept %q 拒绝源图 jpeg，应转码为 image/avif，got %q", accept, got)
			}
		})
	}
}

func TestHandleImage_SpecificityOverrideAppliesToBothDecisions(t *testing.T) {
	// "image/webp;q=0, */*;q=0.9" 中 webp 的有效 q 由更 specific 的 range 决定为 0：
	// 转码目标不得包含 webp；而源图 jpeg 未被显式提及，仍按 */* 的 q 接受
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)
	const accept = "image/webp;q=0, */*;q=0.9"

	t.Run("转码目标不含被拒绝的 webp", func(t *testing.T) {
		// 需要缩放才会进入转码目标选择
		req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, accept)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if got := rr.Header().Get("Content-Type"); got != "image/avif" {
			t.Errorf("webp 被 q=0 拒绝，应转码为 image/avif，got %q", got)
		}
	})

	t.Run("源图仍按通配符被接受", func(t *testing.T) {
		req := signedRequest(t, signer, rootDir, relPath, nil, accept)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		// jpeg 经 */* 被接受且未指定宽度 → 全尺寸诉求重定向到 raw URL
		if rr.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", rr.Code)
		}
	})
}

func TestHandleImage_EqualQTieBreakIsOrderIndependent(t *testing.T) {
	// ADR 0010：同 q 值的书写顺序不承载偏好。avif 与 webp 并列时固定取 AVIF，
	// 因此以下四种写法必须产出同一结果
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)

	for _, accept := range []string{
		"image/avif, image/webp",
		"image/webp, image/avif",
		"*/*, image/webp",
		"image/webp, */*",
	} {
		t.Run(accept, func(t *testing.T) {
			req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, accept)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
			}
			if got := rr.Header().Get("Content-Type"); got != "image/avif" {
				t.Errorf("Accept %q 同 q 并列应取 AVIF，got %q", accept, got)
			}
		})
	}
}

// #region 一次打开、304 与非法参数

func TestHandleImage_CacheHitDoesNotProbeSource(t *testing.T) {
	// w>0 且变体已在缓存时，请求不应打开源图
	logger := zap.NewNop()

	processCalls := 0
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		processCalls++
		_, err := w.Write([]byte("fresh-variant"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	probe := newCountingProbe()
	handler := handleImage(logger, signer, coordinator, probe, rootDir)

	// 预置 w=1024、WebP 的缓存产物
	spec, err := appimage.NewSpec(1024, appimage.ImageFormatWebP)
	require.NoError(t, err)
	key, err := appimage.VariantKey(filepath.Join(rootDir, relPath), spec, "test-encoder")
	require.NoError(t, err)
	require.NoError(t, cache.Save(context.Background(), key, bytes.NewReader([]byte("cached-variant"))))

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/jpeg")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if probe.probes != 0 {
		t.Errorf("缓存命中不应打开源图，探针调用 %d 次", probe.probes)
	}
	if processCalls != 0 {
		t.Errorf("缓存命中不应转码，处理器调用 %d 次", processCalls)
	}
	if got := rr.Body.String(); got != "cached-variant" {
		t.Errorf("expected cached variant body, got %q", got)
	}
}

func TestHandleImage_WidthAboveSourceTranscodesWithClampedWidth(t *testing.T) {
	// 源图 512 宽：w=1024/2048/4096 都超过源宽且源图 MIME 不被接受，
	// 有效宽度应统一夹到 min(w, 源宽)=512
	logger := zap.NewNop()

	var capturedWidths []int
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		capturedWidths = append(capturedWidths, spec.Width())
		_, err := w.Write([]byte("fake-avif"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	rootDir := t.TempDir()
	signer := urlconv.NewSigner("test-secret", rootDir)
	relPath := "image.jpg"
	requireWriteSource(t, filepath.Join(rootDir, relPath), 512, 384)

	handler := testHandler(logger, signer, coordinator, rootDir)

	for _, width := range []int{1024, 2048, 4096} {
		t.Run(fmt.Sprintf("w=%d", width), func(t *testing.T) {
			req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(width)}, "image/avif")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
			}
			if got := rr.Header().Get("Content-Type"); got != "image/avif" {
				t.Errorf("expected image/avif, got %q", got)
			}
		})
	}

	if len(capturedWidths) != 3 {
		t.Fatalf("expected one transcode per request, got %d", len(capturedWidths))
	}
	for i, got := range capturedWidths {
		if got != 512 {
			t.Errorf("第 %d 个请求的有效宽度应夹到源宽 512，got %d", i, got)
		}
	}
}

func TestHandleImage_InvalidWidthReturns400(t *testing.T) {
	// w 非数字、0、负数都属生成/客户端错误，不得静默退化为全尺寸
	logger := zap.NewNop()

	processCalls := 0
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		processCalls++
		_, err := w.Write([]byte("fake-image"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)

	// 身份参数必须与磁盘一致，否则请求会先被 404 拒绝，无法验证 w 的校验分支
	tStr, sStr := requireIdentityQuery(t, filepath.Join(rootDir, relPath))

	for _, widthParam := range []string{"w=abc", "w=0", "w=-1"} {
		rawQuery := widthParam + "&t=" + tStr + "&s=" + sStr
		t.Run(rawQuery, func(t *testing.T) {
			req := forgeQueryRequest(t, "test-secret", relPath, rawQuery)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400 for %q, got %d", rawQuery, rr.Code)
			}
		})
	}

	if processCalls != 0 {
		t.Errorf("非法 w 不应进入转码，处理器调用 %d 次", processCalls)
	}
	cache.mu.Lock()
	cached := len(cache.files)
	cache.mu.Unlock()
	if cached != 0 {
		t.Errorf("非法 w 不应写缓存，got %d entries", cached)
	}
}

func TestHandleImage_HeadRedirectHasNoBody(t *testing.T) {
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)

	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(4096)}, "image/jpeg")
	req.Method = http.MethodHead
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected status 302 for HEAD redirect, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got == "" {
		t.Error("HEAD redirect 必须带 Location")
	}
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD redirect 不应有 body, got %d bytes", rr.Body.Len())
	}
}

func TestHandleImage_RawNotModifiedHasNoVary(t *testing.T) {
	// raw 的内容与 Accept 无关：其 200 与 304 都不带 Vary
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)

	req := rawURLRequest(t, signer, rootDir, relPath, "")
	req.Header.Set("If-None-Match", `"immutable"`)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotModified {
		t.Errorf("expected status 304, got %d", rr.Code)
	}
	if got := rr.Header().Get("Vary"); got != "" {
		t.Errorf("raw 304 不应带 Vary, got %q", got)
	}
}

func TestHandleImage_RawServesTruncatedSourceWithoutDecoding(t *testing.T) {
	// raw 只要求「一次 open → 嗅探 MIME → 服务」；源图能否解码尺寸与 raw 无关
	logger := zap.NewNop()

	coordinator := newTestCoordinator(nil)

	rootDir := t.TempDir()
	signer := urlconv.NewSigner("test-secret", rootDir)
	relPath := "truncated.jpg"
	requireWriteTruncatedJPEG(t, filepath.Join(rootDir, relPath))

	handler := testHandler(logger, signer, coordinator, rootDir)
	req := rawURLRequest(t, signer, rootDir, relPath, "")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 for truncated raw source, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got %q", got)
	}
	if got := rr.Header().Get("Vary"); got != "" {
		t.Errorf("raw 响应不应带 Vary, got %q", got)
	}
	require.Equal(t, requireReadFile(t, filepath.Join(rootDir, relPath)), rr.Body.Bytes())
}

func TestHandleImage_NonDecodableSourceUsesMetaFallbackForRedirect(t *testing.T) {
	// AVIF 源本地无法解码宽度，探针回退到 Meta；w >= 源宽 且 MIME 可接受时仍应 302 到 raw
	logger := zap.NewNop()

	processCalls := 0
	processFn := func(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
		processCalls++
		_, err := w.Write([]byte("fake-avif"))
		return err
	}
	cache := newStubCache()
	queue := &stubQueue{processFunc: processFn}
	processor := &stubProcessor{processFunc: processFn}
	coordinator := appimage.NewTranscodeCoordinator(cache, processor, queue, "test-encoder", nil)

	rootDir := t.TempDir()
	signer := urlconv.NewSigner("test-secret", rootDir)
	relPath := "photo.avif"
	requireWriteFakeAVIF(t, filepath.Join(rootDir, relPath))

	probe := newCountingProbe()
	handler := handleImage(logger, signer, coordinator, probe, rootDir)

	// 回退宽度 2048；请求 4096 超过源图
	req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(4096)}, "image/avif")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected status 302 for non-decodable source, got %d", rr.Code)
	}
	if probe.probes != 1 {
		t.Errorf("expected exactly 1 probe, got %d", probe.probes)
	}
	rawURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath), appimage.WithRaw())
	require.NoError(t, err)
	if got := rr.Header().Get("Location"); got != rawURL.String() {
		t.Errorf("Location 应与 rawURL 一致\n got: %q\nwant: %q", got, rawURL.String())
	}
	if processCalls != 0 {
		t.Errorf("重定向不应触发转码，处理器调用 %d 次", processCalls)
	}
	cache.mu.Lock()
	cached := len(cache.files)
	cache.mu.Unlock()
	if cached != 0 {
		t.Errorf("重定向不应写缓存，got %d entries", cached)
	}
}

// #endregion

// #region 过期图片身份

// assertNoWork 断言过期身份请求既未读取源图，也未查询变体缓存或登记转码，
// 且错误响应不携带 immutable 缓存头
func assertNoWork(t *testing.T, rr *httptest.ResponseRecorder, probe *countingProbe, cache *stubCache, queue *stubQueue, processor *stubProcessor) {
	t.Helper()

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}
	if probe.opens != 0 || probe.probes != 0 {
		t.Errorf("过期身份不得打开源图，open=%d probe=%d", probe.opens, probe.probes)
	}
	if cache.lookupCount() != 0 {
		t.Errorf("过期身份不得查询变体缓存，lookup=%d", cache.lookupCount())
	}
	if queue.enqueues != 0 {
		t.Errorf("过期身份不得登记转码，enqueue=%d", queue.enqueues)
	}
	if processor.metaCalls != 0 {
		t.Errorf("过期身份不得读取源图元数据，meta=%d", processor.metaCalls)
	}
	for _, header := range []string{"Cache-Control", "ETag"} {
		if got := rr.Header().Get(header); got != "" {
			t.Errorf("过期身份的错误响应不得带 %s, got %q", header, got)
		}
	}
}

func TestHandleImage_StaleIdentityReturns404(t *testing.T) {
	logger := zap.NewNop()

	t.Run("raw 请求（修改时间变化）", func(t *testing.T) {
		coordinator, cache, queue, processor := newCountingCoordinator()
		signer, rootDir, relPath := testSetup(t, logger, coordinator)
		probe := newCountingProbe()
		handler := handleImage(logger, signer, coordinator, probe, rootDir)

		req := rawURLRequest(t, signer, rootDir, relPath, "")
		requireBumpModTime(t, filepath.Join(rootDir, relPath), 2*time.Second)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assertNoWork(t, rr, probe, cache, queue, processor)
	})

	t.Run("变体请求（大小变化）", func(t *testing.T) {
		coordinator, cache, queue, processor := newCountingCoordinator()
		signer, rootDir, relPath := testSetup(t, logger, coordinator)
		probe := newCountingProbe()
		handler := handleImage(logger, signer, coordinator, probe, rootDir)

		req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/jpeg")
		requireChangeSizeKeepModTime(t, filepath.Join(rootDir, relPath))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assertNoWork(t, rr, probe, cache, queue, processor)
	})

	t.Run("HEAD 请求", func(t *testing.T) {
		coordinator, cache, queue, processor := newCountingCoordinator()
		signer, rootDir, relPath := testSetup(t, logger, coordinator)
		probe := newCountingProbe()
		handler := handleImage(logger, signer, coordinator, probe, rootDir)

		req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(256)}, "image/webp")
		req.Method = http.MethodHead
		requireBumpModTime(t, filepath.Join(rootDir, relPath), 2*time.Second)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assertNoWork(t, rr, probe, cache, queue, processor)
	})

	t.Run("If-None-Match 不绕过身份校验", func(t *testing.T) {
		// 若过期身份先命中 304，浏览器会继续使用旧内容的本地缓存，
		// 因此身份比对必须先于 304 短路
		coordinator, cache, queue, processor := newCountingCoordinator()
		signer, rootDir, relPath := testSetup(t, logger, coordinator)
		probe := newCountingProbe()
		handler := handleImage(logger, signer, coordinator, probe, rootDir)

		req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/jpeg")
		req.Header.Set("If-None-Match", `"immutable"`)
		requireBumpModTime(t, filepath.Join(rootDir, relPath), 2*time.Second)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assertNoWork(t, rr, probe, cache, queue, processor)
	})

	t.Run("文件已删除", func(t *testing.T) {
		// 签名仍有效，但文件不存在：与身份不一致同为 404，且同样不产生转码工作
		coordinator, cache, queue, processor := newCountingCoordinator()
		signer, rootDir, relPath := testSetup(t, logger, coordinator)
		probe := newCountingProbe()
		handler := handleImage(logger, signer, coordinator, probe, rootDir)

		req := signedRequest(t, signer, rootDir, relPath, []appimage.SignOption{appimage.WithWidth(1024)}, "image/jpeg")
		require.NoError(t, os.Remove(filepath.Join(rootDir, relPath)))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assertNoWork(t, rr, probe, cache, queue, processor)
	})

	t.Run("t/s 缺失或非法", func(t *testing.T) {
		// 这类 URL 不可能由签发侧生成，但不得因此退化为「跳过身份校验」
		cases := map[string]func(tStr, sStr string) string{
			"缺少 t 与 s": func(_, _ string) string { return "w=1024" },
			"缺少 t":     func(_, s string) string { return "w=1024&s=" + s },
			"缺少 s":     func(t, _ string) string { return "w=1024&t=" + t },
			"t 为空串":    func(_, s string) string { return "w=1024&t=&s=" + s },
			"s 为空串":    func(t, _ string) string { return "w=1024&t=" + t + "&s=" },
			"t 非整数":    func(_, s string) string { return "w=1024&t=abc&s=" + s },
			"s 非整数":    func(t, _ string) string { return "w=1024&t=" + t + "&s=abc" },
		}
		for name, rawQueryFor := range cases {
			t.Run(name, func(t *testing.T) {
				coordinator, cache, queue, processor := newCountingCoordinator()
				signer, rootDir, relPath := testSetup(t, logger, coordinator)
				probe := newCountingProbe()
				handler := handleImage(logger, signer, coordinator, probe, rootDir)

				tStr, sStr := requireIdentityQuery(t, filepath.Join(rootDir, relPath))
				req := forgeQueryRequest(t, "test-secret", relPath, rawQueryFor(tStr, sStr))

				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)

				assertNoWork(t, rr, probe, cache, queue, processor)
			})
		}
	})
}

// 对照组：身份未变更时行为不变，合法请求仍正常转码
func TestHandleImage_MatchingIdentityStillServes(t *testing.T) {
	logger := zap.NewNop()

	coordinator, _, queue, _ := newCountingCoordinator()
	signer, rootDir, relPath := testSetup(t, logger, coordinator)
	handler := testHandler(logger, signer, coordinator, rootDir)

	tStr, sStr := requireIdentityQuery(t, filepath.Join(rootDir, relPath))
	req := forgeQueryRequest(t, "test-secret", relPath, "w=1024&t="+tStr+"&s="+sStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 for matching identity, got %d", rr.Code)
	}
	if queue.enqueues != 1 {
		t.Errorf("身份一致时可变体请求应登记 1 次转码，got %d", queue.enqueues)
	}
}

// #endregion

// requireIdentityQuery 返回与文件当前状态一致的 t/s 查询值
func requireIdentityQuery(t *testing.T, path string) (tStr, sStr string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return fmt.Sprintf("%d", info.ModTime().Unix()), fmt.Sprintf("%d", info.Size())
}

// requireBumpModTime 只推后文件的修改时间（内容与大小不变），模拟文件被替换
func requireBumpModTime(t *testing.T, path string, d time.Duration) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime().Add(d)))
}

// requireChangeSizeKeepModTime 写入不同长度的内容后恢复修改时间，
// 模拟「大小变化而秒级修改时间不变」
func requireChangeSizeKeepModTime(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("replaced-image-bytes"), 0o644))
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
}

// requireReadFile 读取文件字节，用于断言响应体与源文件一致
func requireReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

// requireWriteTruncatedJPEG 写入可嗅探为 image/jpeg、但缺少可解码帧信息的截断文件
func requireWriteTruncatedJPEG(t *testing.T, path string) {
	t.Helper()
	jpegHeader := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
	}
	require.NoError(t, os.WriteFile(path, jpegHeader, 0o644))
}

// requireWriteFakeAVIF 写入带 .avif 扩展名的占位文件：
// 内容嗅探为 octet-stream 后回退到扩展名，得到 image/avif
func requireWriteFakeAVIF(t *testing.T, path string) {
	t.Helper()
	data := []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00avif")
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// requireWriteSource 在指定路径写入一个真实 JPEG 源图，使探针能从同一句柄解码出宽度
func requireWriteSource(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xFF})
		}
	}
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode source image: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
}
