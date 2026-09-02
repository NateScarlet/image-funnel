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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	appimage "main/internal/application/image"
	"main/internal/infrastructure/urlconv"
	"main/internal/shared"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// mockProcessor 模拟图像处理器
type mockProcessor struct {
	processFunc func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error)
}

func (m *mockProcessor) Process(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
	return m.processFunc(ctx, srcPath, width, quality, format)
}

func (m *mockProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	return nil, nil
}

func calculateTestSignature(secret []byte, relPath, timestamp, size, w, q, format string) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%s|%s|%s|%s", relPath, timestamp, size, w, q, format)
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

func TestHandleImage_NotFound(t *testing.T) {
	secret := "test-secret"
	rootDir := "/tmp"
	signer := urlconv.NewSigner(secret, rootDir)

	// 创建带 Observer 的 zap Logger，以便验证是否有 Error 级别的日志
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	// 模拟返回 os.ErrNotExist 错误
	mockProc := &mockProcessor{
		processFunc: func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
			return nil, os.ErrNotExist
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	// 构建请求参数并签名
	relPath := "deleted_image.jpg"
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "", "")

	u, err := url.Parse("/image")
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
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
	rootDir := "/tmp"
	signer := urlconv.NewSigner(secret, rootDir)

	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	// 模拟返回 context.Canceled 错误
	mockProc := &mockProcessor{
		processFunc: func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
			return nil, context.Canceled
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	relPath := "image.jpg"
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "", "")

	u, _ := url.Parse("/image")
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
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
	rootDir := "/tmp"
	signer := urlconv.NewSigner(secret, rootDir)

	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	// 模拟返回其他一般错误
	expectedErr := errors.New("some processor internal error")
	mockProc := &mockProcessor{
		processFunc: func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
			return nil, expectedErr
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	relPath := "image.jpg"
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "", "")

	u, _ := url.Parse("/image")
	q := u.Query()
	q.Set("path", relPath)
	q.Set("t", tVal)
	q.Set("s", sVal)
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
	rootDir := "/tmp"
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	// 记录传给 processor 的格式参数
	var capturedFormat appimage.ImageFormat
	mockProc := &mockProcessor{
		processFunc: func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
			capturedFormat = format
			return &mockFile{data: []byte("fake-avif")}, nil
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	relPath := "image.jpg"
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
	rootDir := "/tmp"
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	mockProc := &mockProcessor{
		processFunc: func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
			return &mockFile{data: []byte("fake-image")}, nil
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	tests := []struct {
		name            string
		formatParam     string
		expectedCT      string
		formatForSig    string
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
	rootDir := "/tmp"
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	mockProc := &mockProcessor{
		processFunc: func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
			t.Fatal("processor should not be called for invalid format")
			return nil, nil
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	relPath := "image.jpg"
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
	rootDir := "/tmp"
	signer := urlconv.NewSigner(secret, rootDir)
	logger := zap.NewNop()

	mockProc := &mockProcessor{
		processFunc: func(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
			t.Fatal("processor should not be called for tampered signature")
			return nil, nil
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	relPath := "image.jpg"
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

// mockFile 模拟缓存文件
type mockFile struct {
	data []byte
}

func (f *mockFile) Open() (io.ReadSeekCloser, error) {
	return &mockReadSeekCloser{bytes.NewReader(f.data)}, nil
}

type mockReadSeekCloser struct {
	*bytes.Reader
}

func (m *mockReadSeekCloser) Close() error { return nil }
