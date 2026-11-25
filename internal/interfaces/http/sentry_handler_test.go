package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

func TestSentryEnvelopeHandlerComputesFingerprintWhenMissing(t *testing.T) {
	store := &fakeSentryStore{}
	handler := NewSentryEnvelopeHandler(zap.NewNop(), store)

	req := httptest.NewRequest(http.MethodPost, "/api/0/envelope/", bytes.NewReader(envelopeBody("")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, store.fingerprintDir, "sha256-")
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
