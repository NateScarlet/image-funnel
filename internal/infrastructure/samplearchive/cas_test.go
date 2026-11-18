package samplearchive_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"main/internal/infrastructure/samplearchive"
)

func TestCAS_Put_WritesContentAddressedFileWithShard(t *testing.T) {
	root := t.TempDir()
	cas := samplearchive.NewCAS(filepath.Join(root, "cas"))

	data := []byte("hello")
	cid, err := cas.Put(data)
	require.NoError(t, err)

	// 独立期望值：CIDv1 / raw / sha2-256 multihash 的 base32 大写形式，由 pwsh 预先算出
	assert.Equal(t, "BAFKREIBM6JG3UX5QUMHCN2B3FLC3TYU6DMLB4XA7U5BF44YEGNRJHC4YEQ", cid)

	h := cid[1:]
	require.Greater(t, len(h), 4)
	shard := h[len(h)-3 : len(h)-1]
	assert.Equal(t, "YE", shard)

	path := filepath.Join(root, "cas", shard, h+".data")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestCAS_Put_SameBytesOnlyWrittenOnce(t *testing.T) {
	root := t.TempDir()
	cas := samplearchive.NewCAS(filepath.Join(root, "cas"))

	first, err := cas.Put([]byte("thumbnail-bytes"))
	require.NoError(t, err)
	second, err := cas.Put([]byte("thumbnail-bytes"))
	require.NoError(t, err)
	assert.Equal(t, first, second)

	h := first[1:]
	entries, err := os.ReadDir(filepath.Join(root, "cas", h[len(h)-3:len(h)-1]))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestCAS_Put_DifferentBytesGetDifferentCID(t *testing.T) {
	cas := samplearchive.NewCAS(filepath.Join(t.TempDir(), "cas"))

	first, err := cas.Put([]byte("a"))
	require.NoError(t, err)
	second, err := cas.Put([]byte("b"))
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}
