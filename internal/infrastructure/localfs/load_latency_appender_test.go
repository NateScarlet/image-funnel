package localfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"main/internal/domain/loadlatency"
)

const appendRunID = "0123456789abcdef0123456789abcdef"

func TestLoadLatencyAppenderAppendsWithoutReadingExistingContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "load-latency")
	a, err := NewLoadLatencyAppender(dir)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(a.Dir()), "落盘目录必须是绝对路径")

	first := "{\"segmentSeq\":1}\n"
	path, err := a.Append(context.Background(), appendRunID, []byte(first))
	require.NoError(t, err)
	assert.Equal(t, loadlatency.SegmentPath(a.Dir(), appendRunID), path)
	assert.Equal(t, first, readFile(t, path))

	second := "{\"segmentSeq\":2}\n"
	_, err = a.Append(context.Background(), appendRunID, []byte(second))
	require.NoError(t, err)
	assert.Equal(t, first+second, readFile(t, path), "重试导致同段重复是可接受的，服务端只追加不去重")
}

func TestLoadLatencyAppenderSeparatesRunIDs(t *testing.T) {
	a, err := NewLoadLatencyAppender(t.TempDir())
	require.NoError(t, err)

	other := "fedcba9876543210fedcba9876543210"
	p1, err := a.Append(context.Background(), appendRunID, []byte("a\n"))
	require.NoError(t, err)
	p2, err := a.Append(context.Background(), other, []byte("b\n"))
	require.NoError(t, err)

	assert.NotEqual(t, p1, p2)
	assert.Equal(t, "a\n", readFile(t, p1))
	assert.Equal(t, "b\n", readFile(t, p2))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
