package localfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"main/internal/domain/sentry"
)

// SentryStore 把自托管模式的 envelope 按指纹单层分片原样落盘到 <dir>/<fingerprint>/<eventID>.envelope。
type SentryStore struct {
	dir string
}

var _ sentry.EnvelopeStore = (*SentryStore)(nil)

func NewSentryStore(dir string) (*SentryStore, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &SentryStore{dir: abs}, nil
}

func (s *SentryStore) Dir() string { return s.dir }

// Save 原样写入收到的完整 envelope；eventID 为空时按内容摘要生成文件名，保证落盘总是可定位。
func (s *SentryStore) Save(ctx context.Context, fingerprintDir, eventID string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	name := sentry.Sanitize(eventID)
	if name == "" {
		sum := sha256.Sum256(data)
		name = hex.EncodeToString(sum[:])
	}

	dir := filepath.Join(s.dir, sentry.Sanitize(fingerprintDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".envelope")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
