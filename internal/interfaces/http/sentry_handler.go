package http

import (
	"errors"
	"io"
	"net/http"

	"main/internal/domain/sentry"

	"go.uber.org/zap"
)

const maxEnvelopeBytes = 10 << 20

// NewSentryEnvelopeHandler 只在自托管模式被注册：解析客户端直接发来的 envelope，
// 按显式指纹优先的规则计算单层目录名，再交给 store 落盘。不对外转发。
func NewSentryEnvelopeHandler(logger *zap.Logger, store sentry.EnvelopeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		data, err := io.ReadAll(io.LimitReader(r.Body, maxEnvelopeBytes))
		if err != nil {
			http.Error(w, "failed to read envelope", http.StatusBadRequest)
			return
		}
		envelope, err := sentry.ParseEnvelope(data)
		if err != nil {
			if errors.Is(err, sentry.ErrNoEventItem) {
				// 非事件 envelope（如 client_report）接收后丢弃，不给 SDK 制造重试/报错噪声
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 指纹只用于分片与文件命名；落盘内容保持收到的 envelope 原样
		fingerprintDir := sentry.DirectoryName(sentry.EventFingerprint(envelope.EventJSON))
		if _, err := store.Save(r.Context(), fingerprintDir, envelope.EventID, data); err != nil {
			logger.Error("failed to persist sentry event", zap.Error(err))
			http.Error(w, "failed to persist event", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
