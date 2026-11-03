package graphql

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"main/internal/infrastructure/urlconv"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageResolver_URLBlindSignsWidth(t *testing.T) {
	rootDir := t.TempDir()
	absPath := filepath.Join(rootDir, "photo.jpg")
	require.NoError(t, os.WriteFile(absPath, []byte("image"), 0o644))

	resolver := &imageResolver{&Resolver{signer: urlconv.NewSigner("secret", rootDir)}}
	obj := &shared.ImageDTO{AbsPath: absPath, Width: 100}

	t.Run("任意正数宽度都签 w，与源图宽度无关", func(t *testing.T) {
		for _, width := range []int{1, 100, 101, 4096} {
			uri, err := resolver.URL(context.Background(), obj, &width)
			require.NoError(t, err)
			assert.Equal(t, strconv.Itoa(width), parseQuery(t, uri.String()).Get("w"))
		}
	})

	t.Run("未提供宽度不签 w", func(t *testing.T) {
		uri, err := resolver.URL(context.Background(), obj, nil)
		require.NoError(t, err)
		assert.False(t, parseQuery(t, uri.String()).Has("w"))
	})

	t.Run("非正宽度返回错误", func(t *testing.T) {
		for _, width := range []int{0, -1} {
			_, err := resolver.URL(context.Background(), obj, &width)
			assert.Error(t, err)
		}
	})
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u.Query()
}
