// Package sentry 实现服务端错误上报适配器：把领域事件打包成 Sentry envelope，
// 发往配置 DSN 指定的端点。自托管 DSN 指向自身接收端点，外部 DSN 直发。
package sentry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	domsentry "main/internal/domain/sentry"
)

const reportTimeout = 5 * time.Second

// ErrReentrantReport 表示上报正在进行中，本次事件被丢弃以避免自激循环。
var ErrReentrantReport = errors.New("sentry report reentered")

// Reporter 是上报链路的实现。inFlight 阻止上报过程中的重入，避免自激循环。
type Reporter struct {
	endpoint  string
	publicKey string
	client    *http.Client
	version   string
	inFlight  atomic.Bool
}

var _ domsentry.Reporter = (*Reporter)(nil)

// NewReporter 依据配置产出的 DSN 构建客户端；所有依赖由最外层注入。
// DSN 为空返回空实现，不创建客户端、不产生任何事件。
// 上报失败由调用方用不带 Sentry hook 的安全 logger 记录，不回灌为新事件。
func NewReporter(dsn, version string, client *http.Client) (domsentry.Reporter, error) {
	if dsn == "" {
		return domsentry.NoopReporter, nil
	}
	endpoint, publicKey, err := domsentry.EnvelopeURL(dsn)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("sentry reporter requires a non-nil http client")
	}
	return &Reporter{
		endpoint:  endpoint,
		publicKey: publicKey,
		client:    client,
		version:   version,
	}, nil
}

// Report 上报一个服务端错误；失败只返回给调用方，不在此处记录日志。
func (r *Reporter) Report(ctx context.Context, event domsentry.Event) error {
	if !r.inFlight.CompareAndSwap(false, true) {
		return ErrReentrantReport // 重入被阻断，绝不把上报失败变成下一次上报
	}
	defer r.inFlight.Store(false)

	ctx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()

	payload, eventID, err := buildEvent(event, r.version)
	if err != nil {
		return err
	}
	body, err := buildEnvelope(eventID, payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create sentry request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-sentry-envelope")
	req.Header.Set("X-Sentry-Auth", fmt.Sprintf(
		"Sentry sentry_version=7, sentry_client=image-funnel-server/%s, sentry_key=%s",
		r.version, r.publicKey,
	))

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("send sentry event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sentry endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// buildEvent 生成已脱敏的事件 JSON 与 event_id。
func buildEvent(event domsentry.Event, version string) ([]byte, string, error) {
	salt := newSalt()
	eventID := newEventID()

	level, typ, message := "error", "error", "server error"
	switch {
	case event.Panic != nil:
		level, typ, message = "fatal", "panic", fmt.Sprintf("panic: %v", event.Panic)
	case event.Err != nil:
		message, typ = event.Err.Error(), fmt.Sprintf("%T", event.Err)
	}

	fingerprint := event.Fingerprint
	if len(fingerprint) == 0 {
		fingerprint = []string{"server", level, message}
	}
	// 指纹也要脱敏：显式指纹可能来自含路径或凭据的错误上下文
	scrubbedFingerprint := make([]any, len(fingerprint))
	for i, element := range fingerprint {
		scrubbedFingerprint[i] = scrubString(element, salt)
	}

	decoded := map[string]any{
		"event_id":    eventID,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"level":       level,
		"platform":    "go",
		"message":     message,
		"fingerprint": scrubbedFingerprint,
		"exception": map[string]any{
			"values": []any{map[string]any{
				"type":       typ,
				"value":      message,
				"stacktrace": map[string]any{"frames": stackFrames(event.Stack)},
			}},
		},
		"sdk": map[string]any{"name": "image-funnel-server", "version": version},
	}

	data, err := json.Marshal(scrubValue(decoded, salt))
	if err != nil {
		return nil, "", fmt.Errorf("encode sentry event: %w", err)
	}
	return data, eventID, nil
}

// stackFrames 把运行时栈转成 Sentry 帧：整行保留在 function 中，路径与行号单独可读。
func stackFrames(stack []byte) []any {
	lines := strings.Split(string(stack), "\n")
	frames := make([]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		frame := map[string]any{"function": line}
		if index := strings.LastIndex(line, ":"); index > 0 {
			frame["filename"] = line[:index]
			frame["context_line"] = line[index+1:]
		}
		frames = append(frames, frame)
	}
	return frames
}

// buildEnvelope 使用与前端 SDK 相同的逐行格式，使本地接收端可原样复用。
func buildEnvelope(eventID string, eventJSON []byte) ([]byte, error) {
	header, err := json.Marshal(map[string]string{
		"event_id": eventID,
		"sent_at":  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("encode envelope header: %w", err)
	}
	itemHeader, err := json.Marshal(map[string]any{
		"type":         "event",
		"content_type": "application/json",
		"length":       len(eventJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("encode envelope item header: %w", err)
	}
	body := make([]byte, 0, len(header)+len(itemHeader)+len(eventJSON)+3)
	body = append(body, header...)
	body = append(body, '\n')
	body = append(body, itemHeader...)
	body = append(body, '\n')
	body = append(body, eventJSON...)
	body = append(body, '\n')
	return body, nil
}
