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

	"github.com/chai2010/webp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"main/internal/domain/relpath"
	"main/internal/domain/session"
	"main/internal/infrastructure/samplearchive"
	"main/internal/scalar"

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

func sampleImage(t *testing.T, dir, name string) session.SampleImage {
	t.Helper()
	rel, err := relpath.New(dir, name)
	require.NoError(t, err)
	return session.SampleImage{ID: scalar.ToID(name), RelPath: rel}
}

func casPath(dataRoot, cid string) string {
	h := cid[1:]
	return filepath.Join(dataRoot, "cas", h[len(h)-3:len(h)-1], h+".data")
}

func decodeWebP(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := webp.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return img
}

func readPatch(t *testing.T, dir, cid string) image.Image {
	t.Helper()
	data, err := os.ReadFile(casPath(dir, cid))
	require.NoError(t, err)
	return decodeWebP(t, data)
}

func newGenerator(t *testing.T, dir string, logger *zap.Logger) *samplearchive.InProcessSampleGenerator {
	t.Helper()
	return samplearchive.NewInProcessSampleGenerator(dir,
		samplearchive.NewCommitImageDecoder(dir), samplearchive.NewRegionUnion(), logger)
}

func TestInProcessSampleGenerator_DifferentResolution_UsesWholeImages(t *testing.T) {
	dir := t.TempDir()
	createImage(t, dir, "candidate.png", "300x200", "-fill", "red", "-draw", "rectangle 0,0 299,199")
	createImage(t, dir, "baseline.png", "100x100", "-fill", "blue", "-draw", "rectangle 0,0 99,99")

	candidate := sampleImage(t, dir, "candidate.png")
	baseline := sampleImage(t, dir, "baseline.png")

	gen := newGenerator(t, dir, zap.NewNop())
	candidateCID, baselineCIDs, err := gen.Generate(context.Background(), candidate, []session.SampleImage{baseline})
	require.NoError(t, err)
	require.Len(t, baselineCIDs, 1)

	candidateImg := readPatch(t, dir, candidateCID)
	assert.Equal(t, 224, candidateImg.Bounds().Dx())
	assert.Equal(t, 224, candidateImg.Bounds().Dy())
	assert.True(t, isRed(candidateImg.At(112, 112)), "不同分辨率时整图缩放，候选保持纯红")

	baselineImg := readPatch(t, dir, baselineCIDs[0])
	assert.Equal(t, 224, baselineImg.Bounds().Dx())
	assert.True(t, isBlue(baselineImg.At(112, 112)))
}

func TestInProcessSampleGenerator_DifferentResolution_KeepsWholeImageEdges(t *testing.T) {
	dir := t.TempDir()
	// 宽图两端各有色块：居中裁剪会丢掉两端，整图缩放必须都保留
	createImage(t, dir, "candidate.png", "300x100",
		"-fill", "red", "-draw", "rectangle 0,0 9,99",
		"-fill", "blue", "-draw", "rectangle 290,0 299,99",
	)
	createImage(t, dir, "baseline.png", "100x100")

	candidate := sampleImage(t, dir, "candidate.png")
	baseline := sampleImage(t, dir, "baseline.png")

	gen := newGenerator(t, dir, zap.NewNop())
	candidateCID, _, err := gen.Generate(context.Background(), candidate, []session.SampleImage{baseline})
	require.NoError(t, err)

	img := readPatch(t, dir, candidateCID)
	assert.True(t, isRed(img.At(3, 112)), "左边缘应保留")
	assert.True(t, isBlue(img.At(220, 112)), "右边缘应保留")
}

func TestInProcessSampleGenerator_SameResolution_CropsDiffUnionWithSharedCoordinates(t *testing.T) {
	dir := t.TempDir()
	// 两张同分辨率图各有一处红块，差异区域并集覆盖两处（10,10)-(69,69)
	createImage(t, dir, "candidate.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")
	createImage(t, dir, "baseline.png", "100x100", "-fill", "red", "-draw", "rectangle 60,60 69,69")

	candidate := sampleImage(t, dir, "candidate.png")
	baseline := sampleImage(t, dir, "baseline.png")

	gen := newGenerator(t, dir, zap.NewNop())
	candidateCID, baselineCIDs, err := gen.Generate(context.Background(), candidate, []session.SampleImage{baseline})
	require.NoError(t, err)

	candidateImg := readPatch(t, dir, candidateCID)
	assert.Equal(t, 224, candidateImg.Bounds().Dx())
	assert.Equal(t, 224, candidateImg.Bounds().Dy())

	baselineImg := readPatch(t, dir, baselineCIDs[0])

	// 同一绝对坐标裁剪 → 候选红块落在左上角、基准红块落在右下角，两块 224² 可直接对比
	assert.True(t, isRed(candidateImg.At(4, 4)))
	assert.True(t, isWhite(candidateImg.At(210, 210)))
	assert.True(t, isWhite(baselineImg.At(4, 4)))
	assert.True(t, isRed(baselineImg.At(210, 210)))
}

func TestInProcessSampleGenerator_CachesCIDByImageAndRegion(t *testing.T) {
	dir := t.TempDir()
	createImage(t, dir, "candidate.png", "100x100")
	createImage(t, dir, "baseline.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")

	candidate := sampleImage(t, dir, "candidate.png")
	baseline := sampleImage(t, dir, "baseline.png")

	gen := newGenerator(t, dir, zap.NewNop())
	candidateCID, baselineCIDs, err := gen.Generate(context.Background(), candidate, []session.SampleImage{baseline})
	require.NoError(t, err)

	// 删除 CAS 文件后再次生成：命中缓存则不会重写文件，返回同一 CID
	require.NoError(t, os.Remove(casPath(dir, candidateCID)))
	require.NoError(t, os.Remove(casPath(dir, baselineCIDs[0])))

	secondCandidateCID, secondBaselineCIDs, err := gen.Generate(context.Background(), candidate, []session.SampleImage{baseline})
	require.NoError(t, err)
	assert.Equal(t, candidateCID, secondCandidateCID)
	assert.Equal(t, baselineCIDs, secondBaselineCIDs)
	_, statErr := os.Stat(casPath(dir, candidateCID))
	assert.True(t, os.IsNotExist(statErr), "缓存命中不应重复编码落盘")
}

// 第二次 diff 的裁剪区域不同（编码缓存未命中），但解码器仍应复用候选原图：
// 删除候选文件后再次生成仍能成功，说明解码走的是按图像 ID 缓存的 ImageDecoder。
func TestInProcessSampleGenerator_ReusesDecodedImageWithinCommit(t *testing.T) {
	dir := t.TempDir()
	createImage(t, dir, "candidate.png", "100x100")
	createImage(t, dir, "baselineA.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")
	createImage(t, dir, "baselineB.png", "100x100", "-fill", "blue", "-draw", "rectangle 60,60 69,69")

	candidate := sampleImage(t, dir, "candidate.png")
	baselineA := sampleImage(t, dir, "baselineA.png")
	baselineB := sampleImage(t, dir, "baselineB.png")

	gen := newGenerator(t, dir, zap.NewNop())
	_, _, err := gen.Generate(context.Background(), candidate, []session.SampleImage{baselineA})
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(dir, "candidate.png")))

	// 与 B 的 diff 区域不同，编码缓存不会命中；候选图只能来自解码器缓存
	_, _, err = gen.Generate(context.Background(), candidate, []session.SampleImage{baselineB})
	require.NoError(t, err, "同一 Commit 内再次使用已解码的候选图不应触发磁盘读取")
}

// 并集已由前一个样本扩展后，后续候选的第一个 diff 被完全包含时：
// 裁剪必须使用并集坐标，跳过其余基准的 diff，不能把基准 B 的差异块纳入裁剪。
func TestInProcessSampleGenerator_EarlyStopUsesUnionCrop(t *testing.T) {
	dir := t.TempDir()
	createImage(t, dir, "baselineA.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")
	createImage(t, dir, "baselineB.png", "100x100", "-fill", "blue", "-draw", "rectangle 80,80 89,89")
	createImage(t, dir, "candidate1.png", "100x100")
	// 与 A 的差异仍落在 (10,10)-(19,19)，完全包含于并集
	createImage(t, dir, "candidate2.png", "100x100", "-fill", "red", "-draw", "rectangle 15,15 16,16")

	baselineA := sampleImage(t, dir, "baselineA.png")
	baselineB := sampleImage(t, dir, "baselineB.png")
	candidate1 := sampleImage(t, dir, "candidate1.png")
	candidate2 := sampleImage(t, dir, "candidate2.png")

	gen := newGenerator(t, dir, zap.NewNop())
	_, _, err := gen.Generate(context.Background(), candidate1, []session.SampleImage{baselineA})
	require.NoError(t, err)

	_, baselineCIDs, err := gen.Generate(context.Background(), candidate2, []session.SampleImage{baselineA, baselineB})
	require.NoError(t, err)
	require.Len(t, baselineCIDs, 2)

	// 跳过 B 的 diff → 基准 B 块按并集 (10,10,10,10) 裁剪，蓝块（80,80）不在其中
	baselineBImg := readPatch(t, dir, baselineCIDs[1])
	assert.True(t, isWhite(baselineBImg.At(210, 210)), "提前终止后不应把基准 B 的差异块纳入裁剪")
}

// 无基准的样本使用整图缩放，且不得用整图坐标初始化并集；
// 否则后续真实 diff 会被判定为「完全包含」，退化成整图缩放。
func TestInProcessSampleGenerator_NoBaselineDoesNotSeedUnion(t *testing.T) {
	dir := t.TempDir()
	createImage(t, dir, "candidate1.png", "100x100")
	createImage(t, dir, "candidate2.png", "100x100")
	createImage(t, dir, "baseline.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")

	candidate1 := sampleImage(t, dir, "candidate1.png")
	candidate2 := sampleImage(t, dir, "candidate2.png")
	baseline := sampleImage(t, dir, "baseline.png")

	gen := newGenerator(t, dir, zap.NewNop())
	_, _, err := gen.Generate(context.Background(), candidate1, nil)
	require.NoError(t, err)

	_, baselineCIDs, err := gen.Generate(context.Background(), candidate2, []session.SampleImage{baseline})
	require.NoError(t, err)

	// 真实 diff 区域 (10,10)-(19,19) 应成为裁剪区域：
	// 基准块被红块铺满，左上角为红；若并集被整图污染，左上角会是白色。
	baselineImg := readPatch(t, dir, baselineCIDs[0])
	assert.True(t, isRed(baselineImg.At(4, 4)), "无基准样本不得污染并集坐标")
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

func TestInProcessSampleGenerator_LogsStageDurationsAndCacheHits(t *testing.T) {
	dir := t.TempDir()
	createImage(t, dir, "candidate.png", "100x100")
	createImage(t, dir, "baseline.png", "100x100", "-fill", "red", "-draw", "rectangle 10,10 19,19")

	candidate := sampleImage(t, dir, "candidate.png")
	baseline := sampleImage(t, dir, "baseline.png")

	core, logs := observer.New(zap.DebugLevel)
	gen := newGenerator(t, dir, zap.New(core))
	_, _, err := gen.Generate(context.Background(), candidate, []session.SampleImage{baseline})
	require.NoError(t, err)

	for _, msg := range []string{
		"did generate sample patch set",
		"did decode source image",
		"did compute sample diff rect",
		"did render sample patch",
		"did store sample patch in CAS",
	} {
		entries := logs.FilterMessage(msg).All()
		require.NotEmpty(t, entries, "缺少阶段日志 %s", msg)
		for _, entry := range entries {
			assert.Contains(t, entry.ContextMap(), "duration", "阶段日志 %s 应带 duration 字段", msg)
		}
	}

	// 第二次生成同一组图：编码命中缓存，不应再次渲染
	_, _, err = gen.Generate(context.Background(), candidate, []session.SampleImage{baseline})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, logs.FilterMessage("sample patch cache hit").Len(), 2)
}
