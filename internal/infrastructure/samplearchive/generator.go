package samplearchive

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"path/filepath"
	"sync"
	"time"

	"main/internal/domain/session"
	"main/internal/scalar"

	"github.com/chai2010/webp"
	"github.com/n7olkachev/imgdiff/pkg/imgdiff"
	"go.uber.org/zap"
	"golang.org/x/image/draw"
)

const (
	patchSize    = 224
	patchQuality = 75
)

// patchMode 决定取块方式：patch 按 diff 区域裁剪，full 直接缩放整图。
type patchMode string

const (
	patchModeFull  patchMode = "full"
	patchModePatch patchMode = "patch"
)

// patchKey 是编码缓存的键：(版本化图像 ID, 取块模式, 裁剪坐标)。
// 同一张图同一区域在多轮比较中反复出现时直接复用块 CID。
type patchKey struct {
	imageID scalar.ID
	mode    patchMode
	rect    imageRect
}

// InProcessSampleGenerator 在进程内完成采样：解码 → diff → 裁剪缩放 → WebP 编码。
// 热路径上没有 magick 子进程。生成器不持有解码缓存：解码只走注入的 ImageDecoder；
// 差异区域并集由每次 Commit 新建的 RegionUnion 注入。
//
// 取块判据仍是原图分辨率本身：
//   - 同分辨率（局部重绘）：求候选与各基准的差异区域并集，用同一绝对坐标裁候选与各基准，再各自缩放到 224²；
//   - 不同分辨率（全新生成）：差异即全图，整图缩放到 224²。
type InProcessSampleGenerator struct {
	cas     *CAS
	decoder session.ImageDecoder
	union   *RegionUnion
	mu      sync.Mutex
	cache   map[patchKey]string
	logger  *zap.Logger
}

var _ SampleImageGenerator = (*InProcessSampleGenerator)(nil)

// NewInProcessSampleGenerator 在 dataRoot/cas 下落盘图像块。
// decoder 与 union 均由 Commit 工厂创建并注入，生命周期限于单次 Commit。
func NewInProcessSampleGenerator(dataRoot string, decoder session.ImageDecoder, union *RegionUnion, logger *zap.Logger) *InProcessSampleGenerator {
	return &InProcessSampleGenerator{
		cas:     NewCAS(filepath.Join(dataRoot, "cas")),
		decoder: decoder,
		union:   union,
		cache:   make(map[patchKey]string),
		logger:  logger,
	}
}

// Generate 返回候选块 CID 与基准块 CID 列表（顺序与 baselines 一致）。
// 各阶段耗时以 Debug 级记录：解码 → 定位差异区域 → 逐块渲染编码（含缓存/CAS）。
func (g *InProcessSampleGenerator) Generate(ctx context.Context, candidate session.SampleImage, baselines []session.SampleImage) (string, []string, error) {
	start := time.Now()
	defer func() {
		g.logger.Debug("did generate sample patch set",
			zap.Duration("duration", time.Since(start)),
			zap.Stringer("candidateID", candidate.ID),
			zap.Int("baselines", len(baselines)),
		)
	}()
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	candidateImg, err := g.decode(ctx, candidate)
	if err != nil {
		return "", nil, fmt.Errorf("decode candidate image: %w", err)
	}
	width, height := candidateImg.Bounds().Dx(), candidateImg.Bounds().Dy()

	regionStart := time.Now()
	mode, region, err := g.resolveRegion(ctx, candidateImg, width, height, baselines)
	g.logger.Debug("did resolve sample region",
		zap.Duration("duration", time.Since(regionStart)),
		zap.String("mode", string(mode)),
		zap.Int("baselines", len(baselines)),
		zap.Error(err),
	)
	if err != nil {
		return "", nil, err
	}

	candidateCID, err := g.encode(ctx, candidate.ID, candidateImg, mode, region)
	if err != nil {
		return "", nil, fmt.Errorf("encode candidate patch: %w", err)
	}

	baselineCIDs := make([]string, 0, len(baselines))
	for i, baseline := range baselines {
		baselineImg, err := g.decode(ctx, baseline)
		if err != nil {
			return "", nil, fmt.Errorf("decode baseline image: %w", err)
		}
		cid, err := g.encode(ctx, baseline.ID, baselineImg, mode, region)
		if err != nil {
			return "", nil, fmt.Errorf("encode baseline patch: %w", err)
		}
		g.logger.Debug("did encode baseline patch",
			zap.Int("index", i),
			zap.Stringer("baselineID", baseline.ID),
		)
		baselineCIDs = append(baselineCIDs, cid)
	}
	return candidateCID, baselineCIDs, nil
}

// resolveRegion 判定取块模式与区域。无基准时不做比较，整图缩放且不触碰并集。
func (g *InProcessSampleGenerator) resolveRegion(ctx context.Context, candidate image.Image, width, height int, baselines []session.SampleImage) (patchMode, imageRect, error) {
	if len(baselines) == 0 {
		return patchModeFull, imageRect{W: width, H: height}, nil
	}
	if err := ctx.Err(); err != nil {
		return "", imageRect{}, err
	}

	// 先解码全部基准并判定分辨率：解码器按版本化 ID 缓存，重复图像不读盘。
	images := make([]image.Image, 0, len(baselines))
	sameResolution := true
	for _, baseline := range baselines {
		img, err := g.decode(ctx, baseline)
		if err != nil {
			return "", imageRect{}, fmt.Errorf("decode baseline image: %w", err)
		}
		images = append(images, img)
		if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
			sameResolution = false
		}
	}
	if !sameResolution {
		return patchModeFull, imageRect{W: width, H: height}, nil
	}

	for i, baselineImg := range images {
		rect := g.diffRect(candidate, baselineImg)
		// 每个候选至少做一次 diff：仅当第一个 diff 被已有并集完全包含时提前终止。
		if i == 0 && g.union.Contains(rect) {
			return patchModePatch, g.union.Rect(), nil
		}
		g.union.Expand(rect)
	}

	region := g.union.Rect()
	if region.W <= 0 || region.H <= 0 {
		// 完全一致（无差异）时没有 diff 区域，退化为整图缩放，不伪造 patch 坐标
		return patchModeFull, imageRect{W: width, H: height}, nil
	}
	return patchModePatch, region, nil
}

// diffRect 用 imgdiff 在进程内求两张同分辨率原图的差异包围盒。
// 阈值 0 表示任意非零差异都计入；完全一致时返回零尺寸矩形。
func (g *InProcessSampleGenerator) diffRect(candidate, baseline image.Image) imageRect {
	start := time.Now()
	defer func() {
		g.logger.Debug("did compute sample diff rect",
			zap.Duration("duration", time.Since(start)),
		)
	}()

	result := imgdiff.Diff(candidate, baseline, &imgdiff.Options{Threshold: 0})
	bounds := result.Image.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, _, _, _ := result.Image.At(x, y).RGBA()
			if r == 0 {
				continue
			}
			minX = min(minX, x)
			minY = min(minY, y)
			maxX = max(maxX, x)
			maxY = max(maxY, y)
		}
	}
	if maxX < minX || maxY < minY {
		return imageRect{}
	}
	return imageRect{
		X: minX - bounds.Min.X,
		Y: minY - bounds.Min.Y,
		W: maxX - minX + 1,
		H: maxY - minY + 1,
	}
}

func (g *InProcessSampleGenerator) decode(ctx context.Context, image session.SampleImage) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	img, err := g.decoder.Decode(image)
	g.logger.Debug("did decode source image",
		zap.Duration("duration", time.Since(start)),
		zap.Stringer("imageID", image.ID),
		zap.Error(err),
	)
	return img, err
}

// encode 先查 (图像 ID, 模式, 坐标) 缓存；未命中时渲染并写入 CAS。
func (g *InProcessSampleGenerator) encode(ctx context.Context, imageID scalar.ID, img image.Image, mode patchMode, rect imageRect) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	key := patchKey{imageID: imageID, mode: mode, rect: rect}
	g.mu.Lock()
	cid, cached := g.cache[key]
	g.mu.Unlock()
	if cached {
		g.logger.Debug("sample patch cache hit",
			zap.Stringer("imageID", imageID),
			zap.String("mode", string(mode)),
		)
		return cid, nil
	}

	renderStart := time.Now()
	data, err := g.render(img, mode, rect)
	g.logger.Debug("did render sample patch",
		zap.Duration("duration", time.Since(renderStart)),
		zap.Stringer("imageID", imageID),
		zap.String("mode", string(mode)),
		zap.Error(err),
	)
	if err != nil {
		return "", err
	}

	casStart := time.Now()
	cid, err = g.cas.Put(data)
	g.logger.Debug("did store sample patch in CAS",
		zap.Duration("duration", time.Since(casStart)),
		zap.Stringer("imageID", imageID),
		zap.Error(err),
	)
	if err != nil {
		return "", fmt.Errorf("store sample patch: %w", err)
	}

	g.mu.Lock()
	g.cache[key] = cid
	g.mu.Unlock()
	return cid, nil
}

// render 生成 224² WebP 字节。两种模式都把整块内容强制缩放到 224²：
// patch 模式先按 diff 区域裁剪，full 模式直接缩放整张原图；不做居中裁剪，避免丢掉边缘差异。
func (g *InProcessSampleGenerator) render(img image.Image, mode patchMode, rect imageRect) ([]byte, error) {
	src := img
	if mode == patchModePatch {
		bounds := img.Bounds()
		crop := image.Rect(
			bounds.Min.X+rect.X,
			bounds.Min.Y+rect.Y,
			bounds.Min.X+rect.X+rect.W,
			bounds.Min.Y+rect.Y+rect.H,
		)
		cropped := image.NewRGBA(image.Rect(0, 0, rect.W, rect.H))
		draw.Draw(cropped, cropped.Bounds(), img, crop.Min, draw.Src)
		src = cropped
	}

	resized := image.NewRGBA(image.Rect(0, 0, patchSize, patchSize))
	draw.CatmullRom.Scale(resized, resized.Bounds(), src, src.Bounds(), draw.Src, nil)

	var buf bytes.Buffer
	if err := webp.Encode(&buf, resized, &webp.Options{Quality: patchQuality}); err != nil {
		return nil, fmt.Errorf("encode WebP patch: %w", err)
	}
	return buf.Bytes(), nil
}
