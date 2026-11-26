package http

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domsentry "main/internal/domain/sentry"
	"main/internal/infrastructure/localfs"
	infrasentry "main/internal/infrastructure/sentry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type fakeSentryStore struct {
	fingerprintDir string
	eventID        string
	data           []byte
	err            error
}

func (f *fakeSentryStore) Save(_ context.Context, fingerprintDir, eventID string, data []byte) (string, error) {
	f.fingerprintDir = fingerprintDir
	f.eventID = eventID
	f.data = data
	return "saved-path", f.err
}

func envelopeBody(fingerprint string) []byte {
	event := `{"message":"boom"`
	if fingerprint != "" {
		event += `,"fingerprint":["` + fingerprint + `"]`
	}
	event += `}`
	return envelopeBodyWithEvent(event)
}

func envelopeBodyWithEvent(event string) []byte {
	return []byte(`{"event_id":"abc123"}` + "\n" +
		`{"type":"event"}` + "\n" +
		event + "\n")
}

func TestSentryEnvelopeHandlerPersistsEventWithExplicitFingerprint(t *testing.T) {
	store := &fakeSentryStore{}
	handler := NewSentryEnvelopeHandler(zap.NewNop(), store)

	body := envelopeBody("submit-nav")
	req := httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "submit-nav", store.fingerprintDir)
	assert.Equal(t, "abc123", store.eventID)
	assert.Equal(t, body, store.data, "落盘内容必须是收到的完整 envelope 原样字节")
}

func TestSentryEnvelopeHandlerDerivesReadableFingerprintWhenMissing(t *testing.T) {
	for name, tc := range map[string]struct {
		event string
		want  string
	}{
		"message":   {`{"level":"error","message":"boom"}`, "error\ufffd boom"},
		"exception": {`{"exception":{"values":[{"type":"TypeError","value":"boom"}]}}`, "TypeError\ufffd boom"},
		"logentry":  {`{"level":"warn","logger":"scanner","logentry":"slow"}`, "warn\ufffd scanner\ufffd slow"},
		"chinese":   {`{"level":"error","message":"加载超时"}`, "error\ufffd 加载超时"},
		"timestamp": {`{"timestamp":1764163912.123456}`, "Event1764163912123456"},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeSentryStore{}
			handler := NewSentryEnvelopeHandler(zap.NewNop(), store)

			body := envelopeBodyWithEvent(tc.event)
			req := httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.want, store.fingerprintDir)
			assert.NotContains(t, store.fingerprintDir, "sha256-")
			assert.Equal(t, body, store.data, "落盘内容必须是收到的完整 envelope 原样字节")
		})
	}
}

func TestSentryEnvelopeHandlerEventFallbackVariesByEventTime(t *testing.T) {
	first := &fakeSentryStore{}
	NewSentryEnvelopeHandler(zap.NewNop(), first).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(envelopeBodyWithEvent(`{"timestamp":1764163912.123456}`))),
	)
	second := &fakeSentryStore{}
	NewSentryEnvelopeHandler(zap.NewNop(), second).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(envelopeBodyWithEvent(`{"timestamp":1764163999.654321}`))),
	)

	assert.NotEqual(t, first.fingerprintDir, second.fingerprintDir)
}

func TestSentryEnvelopeHandlerRejectsBadRequest(t *testing.T) {
	store := &fakeSentryStore{}
	handler := NewSentryEnvelopeHandler(zap.NewNop(), store)

	for _, body := range []string{"", "not an envelope"} {
		req := httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	}
}

func TestSentryEnvelopeHandlerIgnoresEnvelopesWithoutEvent(t *testing.T) {
	// SDK 会发送 client_report 等非事件 envelope：接收但不落盘，也不报错
	store := &fakeSentryStore{}
	handler := NewSentryEnvelopeHandler(zap.NewNop(), store)
	body := []byte(`{"event_id":"abc"}` + "\n" + `{"type":"client_report"}` + "\n" + `{"discarded_events":[]}` + "\n")

	req := httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Nil(t, store.data)
}

func TestSentryEnvelopeHandlerRejectsNonPost(t *testing.T) {
	handler := NewSentryEnvelopeHandler(zap.NewNop(), &fakeSentryStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/0/envelope/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestSentryEnvelopeHandlerReportsStoreFailure(t *testing.T) {
	store := &fakeSentryStore{err: errors.New("disk full")}
	handler := NewSentryEnvelopeHandler(zap.NewNop(), store)

	req := httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(envelopeBody("boom")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDevOriginCanReachEnvelopeEndpoint(t *testing.T) {
	// 开发模式下前端 dev server 与后端不同端口，需放行本地源才能浏览器直发
	assert.True(t, isDevOriginAllowed("http://localhost:8080"))
	assert.True(t, isDevOriginAllowed("http://127.0.0.1:5173"))
	assert.False(t, isDevOriginAllowed("https://evil.example"))
	assert.False(t, isDevOriginAllowed("not-a-url"))
}

func TestSentryEnvelopeHandlerStoreFailureDoesNotTriggerReport(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dsn, err := domsentry.SelfDSN(server.URL)
	require.NoError(t, err)
	reporter, err := infrasentry.NewReporter(dsn, "test", server.Client())
	require.NoError(t, err)
	safeLogger := zap.New(zapcore.NewNopCore())
	hookedCore, _ := observer.New(zap.ErrorLevel)
	hookedLogger := zap.New(infrasentry.NewReportingCore(hookedCore, reporter, safeLogger))

	store := &fakeSentryStore{err: errors.New("disk full")}
	body := bytes.NewReader(envelopeBody("boom"))
	NewSentryEnvelopeHandler(safeLogger, store).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, domsentry.EnvelopePath, body))
	assert.Zero(t, requests, "接收端落盘失败只能记日志，不能产生新的上报 envelope")

	hookedLogger.Error("infrastructure failure", zap.Error(errors.New("boom")))
	assert.Equal(t, 1, requests, "普通业务路径的意外错误仍应经接收端点上报")
}

func TestServerErrorLandsInFingerprintDirectoryScrubbed(t *testing.T) {
	store, err := localfs.NewSentryStore(filepath.Join(t.TempDir(), "sentry"))
	require.NoError(t, err)
	handler := NewSentryEnvelopeHandler(zap.NewNop(), store)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	dsn, err := domsentry.SelfDSN(server.URL)
	require.NoError(t, err)
	reporter, err := infrasentry.NewReporter(dsn, "test", server.Client())
	require.NoError(t, err)
	core, _ := observer.New(zap.ErrorLevel)
	logger := zap.New(infrasentry.NewReportingCore(core, reporter, zap.NewNop()))

	logger.Error("infrastructure failure", zap.Error(errors.New(`open C:\Users\alice\secret.key: token=s3cr3t-ABCDEFGHIJ`)))

	var path string
	require.NoError(t, filepath.WalkDir(store.Dir(), func(candidate string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".envelope") {
			path = candidate
		}
		return nil
	}))
	require.NotEmpty(t, path, "服务端错误应经接收端点落盘")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	envelope, err := domsentry.ParseEnvelope(data)
	require.NoError(t, err)
	assert.Equal(t, domsentry.DirectoryName(domsentry.EventFingerprint(envelope.EventJSON)), filepath.Base(filepath.Dir(path)))
	for _, secret := range []string{`C:\Users\alice\secret.key`, "s3cr3t-ABCDEFGHIJ"} {
		assert.NotContains(t, string(data), secret, "本地落盘内容同样必须脱敏")
	}
}

func TestSentryRouteOnlyExistsInSelfHostedMode(t *testing.T) {
	body := bytes.NewReader(envelopeBody("boom"))

	withRoute := (&Server{envelopeHandler: NewSentryEnvelopeHandler(zap.NewNop(), &fakeSentryStore{}), envelopePath: "/api/0/envelope/"}).newRouter()
	rec := httptest.NewRecorder()
	withRoute.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/0/envelope/", body))
	require.Equal(t, http.StatusOK, rec.Code)

	withoutRoute := (&Server{}).newRouter()
	rec = httptest.NewRecorder()
	withoutRoute.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(envelopeBody("boom"))))
	// 未注册时落入 SPA catch-all，绝不能落到 envelope handler 并返回 200
	assert.NotEqual(t, http.StatusOK, rec.Code)
}
