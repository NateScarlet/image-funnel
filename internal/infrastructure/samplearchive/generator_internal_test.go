package samplearchive

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"main/internal/domain/relpath"
	"main/internal/domain/session"
	"main/internal/scalar"
)

func createImageInternal(t *testing.T, dir, name, size string, draw ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	args := []string{"-size", size, "xc:white"}
	args = append(args, draw...)
	args = append(args, path)
	require.NoError(t, exec.Command("magick", args...).Run())
	return path
}

func sampleImageInternal(t *testing.T, dir, name string) session.SampleImage {
	t.Helper()
	rel, err := relpath.New(dir, name)
	require.NoError(t, err)
	return session.SampleImage{ID: scalar.ToID(name), RelPath: rel}
}

// magickDiffRect 复现旧的 diff 判据，用于验证进程内路径的裁剪坐标没有漂移。
func magickDiffRect(t *testing.T, candidate, baseline string) imageRect {
	t.Helper()
	output, err := exec.Command("magick",
		candidate, baseline,
		"-compose", "difference", "-composite",
		"-colorspace", "Gray", "-threshold", "0",
		"-background", "black",
		"-format", "%@", "info:",
	).Output()
	require.NoError(t, err)

	match := regexp.MustCompile(`(\d+)x(\d+)([+-]\d+)([+-]\d+)`).FindStringSubmatch(string(output))
	require.NotNil(t, match, "unexpected magick geometry: %q", output)
	values := make([]int, 4)
	for i := range values {
		v, err := strconv.Atoi(match[i+1])
		require.NoError(t, err)
		values[i] = v
	}
	return imageRect{W: values[0], H: values[1], X: values[2], Y: values[3]}
}

func TestDiffRect_MatchesMagickForKnownPair(t *testing.T) {
	dir := t.TempDir()
	createImageInternal(t, dir, "candidate.png", "100x100",
		"-fill", "red", "-draw", "rectangle 10,10 19,19")
	createImageInternal(t, dir, "baseline.png", "100x100",
		"-fill", "red", "-draw", "rectangle 60,60 69,69")

	decoder := NewCommitImageDecoder(dir)
	candidateImg, err := decoder.Decode(sampleImageInternal(t, dir, "candidate.png"))
	require.NoError(t, err)
	baselineImg, err := decoder.Decode(sampleImageInternal(t, dir, "baseline.png"))
	require.NoError(t, err)

	gen := NewInProcessSampleGenerator(dir, decoder, NewRegionUnion(), zap.NewNop())
	got := gen.diffRect(candidateImg, baselineImg)
	assert.Equal(t, magickDiffRect(t, filepath.Join(dir, "candidate.png"), filepath.Join(dir, "baseline.png")), got)
}

func TestRegionUnion_EmptyDoesNotContainAndExpands(t *testing.T) {
	union := NewRegionUnion()
	rect := imageRect{X: 10, Y: 10, W: 5, H: 5}
	assert.False(t, union.Contains(rect), "空并集不得包含任何矩形")
	union.Expand(rect)
	assert.Equal(t, rect, union.Rect())
	assert.True(t, union.Contains(imageRect{X: 11, Y: 11, W: 2, H: 2}))
	assert.False(t, union.Contains(imageRect{X: 9, Y: 11, W: 2, H: 2}), "越出左边界不应算包含")
	assert.False(t, union.Contains(imageRect{X: 11, Y: 11, W: 6, H: 2}), "越出右边界不应算包含")
	assert.False(t, union.Contains(imageRect{}), "零尺寸矩形不应算包含")
}

// 解码缓存以版本化图像 ID 为键：同一 ID 第二次解码不读盘。
func TestCommitImageDecoder_CachesByImageID(t *testing.T) {
	dir := t.TempDir()
	createImageInternal(t, dir, "image.png", "100x100")
	img := sampleImageInternal(t, dir, "image.png")

	decoder := NewCommitImageDecoder(dir)
	first, err := decoder.Decode(img)
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(dir, "image.png")))
	second, err := decoder.Decode(img)
	require.NoError(t, err, "命中缓存不应重新读盘")
	assert.Same(t, first, second)
}
