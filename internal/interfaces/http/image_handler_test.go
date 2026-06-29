package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
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
	processFunc func(ctx context.Context, srcPath string, width, quality int) (appimage.File, error)
}

func (m *mockProcessor) Process(ctx context.Context, srcPath string, width, quality int) (appimage.File, error) {
	return m.processFunc(ctx, srcPath, width, quality)
}

func (m *mockProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	return nil, nil
}

func calculateTestSignature(secret []byte, relPath, timestamp, size, w, q string) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%s|%s|%s", relPath, timestamp, size, w, q)
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
		processFunc: func(ctx context.Context, srcPath string, width, quality int) (appimage.File, error) {
			return nil, os.ErrNotExist
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	// 构建请求参数并签名
	relPath := "deleted_image.jpg"
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "")

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
		processFunc: func(ctx context.Context, srcPath string, width, quality int) (appimage.File, error) {
			return nil, context.Canceled
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	relPath := "image.jpg"
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "")

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
		processFunc: func(ctx context.Context, srcPath string, width, quality int) (appimage.File, error) {
			return nil, expectedErr
		},
	}

	handler := handleImage(logger, signer, mockProc, rootDir)

	relPath := "image.jpg"
	tVal := "1719660000"
	sVal := "1024"
	sig := calculateTestSignature([]byte(secret), relPath, tVal, sVal, "", "")

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
