// Package sentry 定义上报模式与 envelope 的领域规则。
// 服务端是唯一决策者：环境变量决定模式，下发给客户端的总是可直接使用的 DSN 或空串。
package sentry

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"main/internal/shared"
)

// Mode 上报模式。定义位于 shared/enums.go，领域层只做别名暴露。
type Mode = shared.SentryMode

var (
	ModeOff      = shared.SentryModeOff
	ModeSelf     = shared.SentryModeSelf
	ModeExternal = shared.SentryModeExternal
)

// EnvelopeStore 是 envelope handler 唯一需要的落盘端口，由最外层注入具体实现。
type EnvelopeStore interface {
	Save(ctx context.Context, fingerprintDir, eventID string, data []byte) (string, error)
}

// SelfDSNValue 是自托管模式的精确标识。带任何后缀的变体都不是自托管模式。
const SelfDSNValue = "internal.self:"

// SelfPublicKey / SelfProjectID 组成自托管 DSN 与服务端 envelope 路由的公共契约。
// SDK 按 DSN 生成 {scheme}://{host}/api/{projectID}/envelope/，与 EnvelopePath 对齐。
const (
	SelfPublicKey = "self"
	SelfProjectID = "0"
	EnvelopePath  = "/api/" + SelfProjectID + "/envelope/"
)

// Parse 把环境变量的原始值归约为三种模式。非法值报错，由调用方快速失败。
func Parse(raw string) (Mode, error) {
	if raw == "" {
		return ModeOff, nil
	}
	if raw == SelfDSNValue {
		return ModeSelf, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ModeOff, fmt.Errorf("not a valid sentry DSN: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ModeOff, fmt.Errorf("unsupported sentry DSN scheme %q", u.Scheme)
	}
	if u.User == nil || u.User.Username() == "" {
		return ModeOff, fmt.Errorf("sentry DSN must contain a public key")
	}
	if u.Host == "" {
		return ModeOff, fmt.Errorf("sentry DSN must contain a host")
	}
	projectID := strings.Trim(u.Path, "/")
	if projectID == "" || strings.Contains(projectID, "/") {
		return ModeOff, fmt.Errorf("sentry DSN must contain exactly one project id")
	}
	return ModeExternal, nil
}

// SelfDSN 生成一个指向自身 envelope 端点的标准 DSN。
// baseURL 只用其 scheme 与 host：路径前缀不参与 SDK 计算的 envelope URL。
func SelfDSN(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base URL must use http or https, got %q", baseURL)
	}
	if u.Host == "" {
		return "", fmt.Errorf("base URL must contain a host: %q", baseURL)
	}
	return fmt.Sprintf("%s://%s@%s/%s", u.Scheme, SelfPublicKey, u.Host, SelfProjectID), nil
}
