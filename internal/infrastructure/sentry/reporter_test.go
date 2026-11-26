package sentry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domsentry "main/internal/domain/sentry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestReporter(t *testing.T, dsn string, client *http.Client) domsentry.Reporter {
	t.Helper()
	reporter, err := NewReporter(dsn, "test", client)
	require.NoError(t, err)
	return reporter
}

func selfTestDSN(t *testing.T, server *httptest.Server) string {
	t.Helper()
	dsn, err := domsentry.SelfDSN(server.URL)
	require.NoError(t, err)
	return dsn
}

func TestReporterWithEmptyDSNSendsNothing(t *testing.T) {
	var calls int
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected request")
	})}
	reporter := newTestReporter(t, "", client)

	require.NoError(t, reporter.Report(context.Background(), domsentry.Event{Err: errors.New("boom")}))
	assert.Zero(t, calls, "DSN 为空时不得创建客户端或产生出站请求")
}

func TestReporterPostsScrubbedEventToSelfEndpoint(t *testing.T) {
	var (
		body []byte
		path string
		auth string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("X-Sentry-Auth")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reporter := newTestReporter(t, selfTestDSN(t, server), server.Client())

	require.NoError(t, reporter.Report(context.Background(), domsentry.Event{
		Err:         errors.New(`open C:\Users\alice\secret.key failed, token=s3cr3t-ABCDEFGHIJ setup_token=SETUP-TOKEN-UNIQUE Bearer BEARER-TOKEN-UNIQUE`),
		Fingerprint: []string{"server", "boom"},
	}))

	assert.Equal(t, domsentry.EnvelopePath, path, "自托管事件必须走现有接收端点")
	assert.Contains(t, auth, "sentry_key=self")
	envelope, err := domsentry.ParseEnvelope(body)
	require.NoError(t, err)
	assert.Equal(t, "server_boom", domsentry.DirectoryName(domsentry.EventFingerprint(envelope.EventJSON)))

	text := string(body)
	for _, secret := range []string{
		`C:\Users\alice\secret.key`,
		"s3cr3t-ABCDEFGHIJ",
		"SETUP-TOKEN-UNIQUE",
		"BEARER-TOKEN-UNIQUE",
	} {
		assert.NotContains(t, text, secret, "离开进程前必须脱敏：%s", secret)
	}
}

func TestReporterReportsPanicAsFatalEvent(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reporter := newTestReporter(t, selfTestDSN(t, server), server.Client())

	require.NoError(t, reporter.Report(context.Background(), domsentry.Event{
		Panic:       `boom C:\Users\alice\secret.key`,
		Stack:       []byte("main.go:42 +0x1f"),
		Fingerprint: []string{"server", "panic"},
	}))

	envelope, err := domsentry.ParseEnvelope(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(envelope.EventJSON, &decoded))
	assert.Equal(t, "fatal", decoded["level"])
	assert.Equal(t, "server_panic", domsentry.DirectoryName(domsentry.EventFingerprint(envelope.EventJSON)))
	assert.NotContains(t, string(body), `C:\Users\alice\secret.key`)
}

func TestReporterPostsToExternalEndpointWithoutLocalStore(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dsn := fmt.Sprintf("http://publickey@%s/42", strings.TrimPrefix(server.URL, "http://"))
	reporter := newTestReporter(t, dsn, server.Client())

	require.NoError(t, reporter.Report(context.Background(), domsentry.Event{Err: errors.New("boom")}))
	assert.Equal(t, "/api/42/envelope/", path, "外部 DSN 直接发往该端点，不写本地分片")
}

func TestReporterSendFailureReturnsError(t *testing.T) {
	var calls int
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("network down")
	})}
	reporter := newTestReporter(t, "https://publickey@example.com/1", client)

	err := reporter.Report(context.Background(), domsentry.Event{Err: errors.New("boom")})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "上报链路自身失败不得产生新的上报请求")
}

func TestReporterBlocksReentrantReport(t *testing.T) {
	var (
		reporter domsentry.Reporter
		calls    int
	)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			// 发送过程中再次被触发（如日志 hook 回灌）时，重入必须被阻断
			assert.ErrorIs(t, reporter.Report(context.Background(), domsentry.Event{Err: errors.New("nested")}), ErrReentrantReport)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	reporter = newTestReporter(t, "https://publickey@example.com/1", client)

	require.NoError(t, reporter.Report(context.Background(), domsentry.Event{Err: errors.New("boom")}))
	assert.Equal(t, 1, calls, "重入不得导致第二次上报")
}
