package localfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"main/internal/domain/sentry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentryStoreSavesIntoFingerprintDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := NewSentryStore(filepath.Join(root, "sentry"))
	require.NoError(t, err)

	// 原样存储整个 envelope，不做 JSON 重序列化或筛选
	envelope := []byte(`{"event_id":"abc123"}` + "\n" + `{"type":"event"}` + "\n" + `{"message":"boom"}` + "\n" +
		`{"type":"attachment","length":4}` + "\n" + `data`)
	path, err := store.Save(context.Background(), "submit-nav_stalled", "abc123", envelope)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(store.Dir(), "submit-nav_stalled", "abc123.envelope"), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, envelope, data)
}

func TestSentryStoreSanitizesEventID(t *testing.T) {
	store, err := NewSentryStore(t.TempDir())
	require.NoError(t, err)

	path, err := store.Save(context.Background(), "fingerprint", "../evil/id", []byte("{}"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(store.Dir(), "fingerprint", "\ufffd\ufffdevil\ufffdid.envelope"), path)
}

func TestSentryStorePreservesTruncationMarkerInDirectoryName(t *testing.T) {
	fingerprint := sentry.DirectoryName([]string{strings.Repeat("a", sentry.MaxDirectoryNameBytes*2)})
	require.Contains(t, fingerprint, "...(")

	store, err := NewSentryStore(t.TempDir())
	require.NoError(t, err)

	path, err := store.Save(context.Background(), fingerprint, "abc123", []byte("{}"))
	require.NoError(t, err)
	assert.Equal(t, fingerprint, filepath.Base(filepath.Dir(path)))
}

func TestSentryStoreRejectsPathLikeFingerprintDirectory(t *testing.T) {
	store, err := NewSentryStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.Save(context.Background(), "../evil", "abc123", []byte("{}"))
	require.Error(t, err)
}

func TestSentryStoreGeneratesIDWhenMissing(t *testing.T) {
	store, err := NewSentryStore(t.TempDir())
	require.NoError(t, err)

	path, err := store.Save(context.Background(), "fingerprint", "", []byte(`{"message":"boom"}`))
	require.NoError(t, err)
	assert.NotEqual(t, filepath.Join(store.Dir(), "fingerprint", ".envelope"), path)
	assert.FileExists(t, path)
}
