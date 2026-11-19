package samplearchive_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "golang.org/x/image/webp"

	"main/internal/infrastructure/samplearchive"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func createImage(t *testing.T, dir, name, size string, draw ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	args := []string{"-size", size, "xc:white"}
	args = append(args, draw...)
	args = append(args, path)
	require.NoError(t, exec.Command("magick", args...).Run())
	return path
}

func casPath(dataRoot, cid string) string {
	h := cid[1:]
	return filepath.Join(dataRoot, "cas", h[len(h)-3:len(h)-1], h+".data")
}

func decodeWebP(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, "webp", format)
	return img
}

func TestMagickSampleGenerator_DifferentResolution_UsesWholeImages(t *testing.T) {
	dir := t.TempDir()
	candidate := createImage(t, dir, "candidate.png", "300x200", "-fill", "red", "-draw", "rectangle 0,0 299,199")
	baseline := createImage(t, dir, "baseline.png", "100x100", "-fill", "blue", "-draw", "rectangle 0,0 99,99")

	gen := samplearchive.NewMagickSampleGenerator(dir, 1, zap.NewNop())
	candidateCID, baselineCIDs, err := gen.Generate(context.Background(), candidate, []string{baseline})
	require.NoError(t, err)
	require.Len(t, baselineCIDs, 1)

	candidateData, err := os.ReadFile(casPath(dir, candidateCID))
	require.NoError(t, err)
	decoded := decodeWebP(t, candidateData)
	assert.Equal(t, 224, decoded.Bounds().Dx())
	assert.Equal(t, 224, decoded.Bounds().Dy())
	assert.True(t, isRed(decoded.At(112, 112)), "不同分辨率时整图缩放，候选保持纯红")

	baselineData, err := os.ReadFile(casPath(dir, baselineCIDs[0]))
	require.NoError(t, err)
	baselineDecoded := decodeWebP(t, baselineData)
	assert.Equal(t, 224, baselineDecoded.Bounds().Dx())
	assert.True(t, isBlue(baselineDecoded.At(112, 112)))
}

func TestMagickSampleGenerator_DifferentResolution_KeepsWholeImageEdges(t *testing.T) {
	dir := t.TempDir()
	// 宽图两端各有色块：居中裁剪会丢掉两端，整图缩放必须都保留
	candidate := createImage(t, dir, "candidate.png", "300x100",
		"-fill", "red", "-draw", "rectangle 0,0 9,99",
		"-fill", "blue", "-draw", "rectangle 290,0 299,99",
	)
	baseline := createImage(t, dir, "baseline.png", "100x100")

	gen := samplearchive.NewMagickSampleGenerator(dir, 1, zap.NewNop())
	candidateCID, _, err := gen.Generate(context.Background(), candidate, []string{baseline})
	require.NoError(t, err)

	data, err := os.ReadFile(casPath(dir, candidateCID))
	require.NoError(t, err)
	img := decodeWebP(t, data)
	assert.True(t, isRed(img.At(3, 112)), "左边缘应保留")
	assert.True(t, isBlue(img.At(220, 112)), "右边缘应保留")
}

func TestMagickSampleGenerator_SameResolution_CropsDiffUnionWithSharedCoordinates(t *testing.T) {
	dir := t.TempDir()
	// 两张同分辨率图各有一处红块，差异区域并集覆盖两处（10,10)-(69,69)
	candidate := createImage(t, dir, "candidate.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")
	baseline := createImage(t, dir, "baseline.png", "100x100", "-fill", "red", "-draw", "rectangle 60,60 69,69")

	gen := samplearchive.NewMagickSampleGenerator(dir, 1, zap.NewNop())
	candidateCID, baselineCIDs, err := gen.Generate(context.Background(), candidate, []string{baseline})
	require.NoError(t, err)

	candidateData, err := os.ReadFile(casPath(dir, candidateCID))
	require.NoError(t, err)
	candidateImg := decodeWebP(t, candidateData)
	assert.Equal(t, 224, candidateImg.Bounds().Dx())
	assert.Equal(t, 224, candidateImg.Bounds().Dy())

	baselineData, err := os.ReadFile(casPath(dir, baselineCIDs[0]))
	require.NoError(t, err)
	baselineImg := decodeWebP(t, baselineData)

	// 同一绝对坐标裁剪 → 候选红块落在左上角、基准红块落在右下角，两块 224² 可直接对比
	assert.True(t, isRed(candidateImg.At(4, 4)))
	assert.True(t, isWhite(candidateImg.At(210, 210)))
	assert.True(t, isWhite(baselineImg.At(4, 4)))
	assert.True(t, isRed(baselineImg.At(210, 210)))
}

func TestMagickSampleGenerator_CachesCIDByImageAndRegion(t *testing.T) {
	dir := t.TempDir()
	candidate := createImage(t, dir, "candidate.png", "100x100")
	baseline := createImage(t, dir, "baseline.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")

	gen := samplearchive.NewMagickSampleGenerator(dir, 1, zap.NewNop())
	candidateCID, baselineCIDs, err := gen.Generate(context.Background(), candidate, []string{baseline})
	require.NoError(t, err)

	// 删除 CAS 文件后再次生成：命中缓存则不会重写文件，返回同一 CID
	require.NoError(t, os.Remove(casPath(dir, candidateCID)))
	require.NoError(t, os.Remove(casPath(dir, baselineCIDs[0])))

	secondCandidateCID, secondBaselineCIDs, err := gen.Generate(context.Background(), candidate, []string{baseline})
	require.NoError(t, err)
	assert.Equal(t, candidateCID, secondCandidateCID)
	assert.Equal(t, baselineCIDs, secondBaselineCIDs)
	_, statErr := os.Stat(casPath(dir, candidateCID))
	assert.True(t, os.IsNotExist(statErr), "缓存命中不应重复编码落盘")
}

// WebP 有损编码会带来轻微色偏，按主色通道判定而不是逐像素精确相等
func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xc000 && g < 0x3000 && b < 0x3000
}

func isBlue(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return b > 0xc000 && r < 0x3000 && g < 0x3000
}

func isWhite(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xd000 && g > 0xd000 && b > 0xd000
}

func TestMagickSampleGenerator_LogsStageDurationsAndCacheHits(t *testing.T) {
	dir := t.TempDir()
	candidate := createImage(t, dir, "candidate.png", "100x100")
	baseline := createImage(t, dir, "baseline.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")

	core, logs := observer.New(zap.DebugLevel)
	gen := samplearchive.NewMagickSampleGenerator(dir, 1, zap.New(core))
	_, _, err := gen.Generate(context.Background(), candidate, []string{baseline})
	require.NoError(t, err)

	for _, msg := range []string{
		"did generate sample patch set",
		"did read candidate dimensions",
		"did run magick identify",
		"did resolve sample region",
		"did compute sample diff rect",
		"did render sample patch",
		"did run magick render",
		"did store sample patch in CAS",
	} {
		entries := logs.FilterMessage(msg).All()
		require.NotEmpty(t, entries, "缺少阶段日志 %s", msg)
		for _, entry := range entries {
			assert.Contains(t, entry.ContextMap(), "duration", "阶段日志 %s 应带 duration 字段", msg)
		}
	}

	// 第二次生成同一组图：编码命中缓存，不应再次走 magick render
	_, _, err = gen.Generate(context.Background(), candidate, []string{baseline})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, logs.FilterMessage("sample patch cache hit").Len(), 2)
}
