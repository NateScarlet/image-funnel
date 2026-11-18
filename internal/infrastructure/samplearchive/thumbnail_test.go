package samplearchive_test

import (
	"bytes"
	"context"
	"image"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "golang.org/x/image/webp"

	"main/internal/infrastructure/samplearchive"
)

func TestMagickThumbnailer_ProducesExact224SquareWebP(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.png")
	if err := exec.Command("magick", "-size", "800x600", "xc:red", src).Run(); err != nil {
		t.Skipf("magick unavailable: %v", err)
	}

	data, err := samplearchive.NewMagickThumbnailer(1).Thumbnail(context.Background(), src)
	require.NoError(t, err)

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, "webp", format)
	assert.Equal(t, 224, config.Width)
	assert.Equal(t, 224, config.Height)
}
