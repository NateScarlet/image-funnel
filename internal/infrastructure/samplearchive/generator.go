package samplearchive

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"golang.org/x/sync/semaphore"
)

const (
	patchSize    = 224
	patchQuality = 75
)

// imageRect 是原图绝对坐标系下的矩形。W/H 为零表示无差异区域。
type imageRect struct {
	X, Y, W, H int
}

// patchMode 决定取块方式：patch 按 diff 区域裁剪，full 直接缩放整图。
type patchMode string

const (
	patchModeFull  patchMode = "full"
	patchModePatch patchMode = "patch"
)

// patchKey 是编码缓存的键：(原图路径, 取块模式, 裁剪坐标)。
// 同一张图同一区域在多轮比较中反复出现时直接复用块 CID。
type patchKey struct {
	sourcePath string
	mode       patchMode
	rect       imageRect
}

// MagickSampleGenerator 用项目已有的 ImageMagick 在后台生成一组 224² 块。
//
// 取块判据是原图分辨率本身，不使用启发式阈值、不依赖 rating 或生成方式约定：
//   - 同分辨率（局部重绘）：在原图上算候选与所有基准的差异区域并集，用同一绝对坐标
//     裁候选与各基准，再各自缩放到 224²，逐块对齐；
//   - 不同分辨率（全新生成）：差异即全图，不做 diff 与裁剪，整图缩放到 224²。
//
// 224² 块只作为训练时的对比输入，不用于计算 diff 坐标。
type MagickSampleGenerator struct {
	sem        *semaphore.Weighted
	cas        *CAS
	mu         sync.Mutex
	cache      map[patchKey]string
	dimensions *magickDimensionReader
}

var _ SampleImageGenerator = (*MagickSampleGenerator)(nil)

// NewMagickSampleGenerator 在 dataRoot/cas 下落盘图像块；concurrency 限制并发的 magick 进程数。
func NewMagickSampleGenerator(dataRoot string, concurrency int64) *MagickSampleGenerator {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &MagickSampleGenerator{
		sem:        semaphore.NewWeighted(concurrency),
		cas:        NewCAS(filepath.Join(dataRoot, "cas")),
		cache:      make(map[patchKey]string),
		dimensions: &magickDimensionReader{},
	}
}

// Generate 返回候选块 CID 与基准块 CID 列表（顺序与 baselinePaths 一致）。
func (g *MagickSampleGenerator) Generate(ctx context.Context, candidatePath string, baselinePaths []string) (string, []string, error) {
	candidateW, candidateH, err := g.dimensions.Dimensions(ctx, candidatePath)
	if err != nil {
		return "", nil, fmt.Errorf("read candidate dimensions: %w", err)
	}

	mode, region, err := g.resolveRegion(ctx, candidatePath, candidateW, candidateH, baselinePaths)
	if err != nil {
		return "", nil, err
	}

	candidateCID, err := g.encode(ctx, candidatePath, mode, region)
	if err != nil {
		return "", nil, fmt.Errorf("encode candidate patch: %w", err)
	}

	baselineCIDs := make([]string, 0, len(baselinePaths))
	for _, baselinePath := range baselinePaths {
		cid, err := g.encode(ctx, baselinePath, mode, region)
		if err != nil {
			return "", nil, fmt.Errorf("encode baseline patch: %w", err)
		}
		baselineCIDs = append(baselineCIDs, cid)
	}
	return candidateCID, baselineCIDs, nil
}

// resolveRegion 判定取块模式与区域。无基准时不做比较，整图缩放。
func (g *MagickSampleGenerator) resolveRegion(ctx context.Context, candidatePath string, width, height int, baselinePaths []string) (patchMode, imageRect, error) {
	if len(baselinePaths) == 0 {
		return patchModeFull, imageRect{W: width, H: height}, nil
	}

	sameResolution := true
	for _, baselinePath := range baselinePaths {
		w, h, err := g.dimensions.Dimensions(ctx, baselinePath)
		if err != nil {
			return "", imageRect{}, fmt.Errorf("read baseline dimensions: %w", err)
		}
		if w != width || h != height {
			sameResolution = false
			break
		}
	}
	if !sameResolution {
		return patchModeFull, imageRect{W: width, H: height}, nil
	}

	// 同分辨率：对每一对候选/基准求差异包围盒，再取并集，得到唯一裁剪区域。
	union := imageRect{}
	for _, baselinePath := range baselinePaths {
		rect, err := g.diffRect(ctx, candidatePath, baselinePath)
		if err != nil {
			return "", imageRect{}, err
		}
		union = unionRect(union, rect)
	}
	if union.W <= 0 || union.H <= 0 {
		// 完全一致（无差异）时没有 diff 区域，退化为整图缩放，不伪造 patch 坐标
		return patchModeFull, imageRect{W: width, H: height}, nil
	}
	return patchModePatch, union, nil
}

func unionRect(a, b imageRect) imageRect {
	if a.W <= 0 || a.H <= 0 {
		return b
	}
	if b.W <= 0 || b.H <= 0 {
		return a
	}
	x1 := min(a.X, b.X)
	y1 := min(a.Y, b.Y)
	x2 := max(a.X+a.W, b.X+b.W)
	y2 := max(a.Y+a.H, b.Y+b.H)
	return imageRect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
}

// diffRect 用 magick 求两张同分辨率原图的差异包围盒。
// difference 后 threshold 0 把「任意非零差异」变成白、无差异变成黑；背景设为黑后
// %@ 返回白色区域的 trim 包围盒。完全一致时 magick 返回 0x0，由调用方当作无差异。
func (g *MagickSampleGenerator) diffRect(ctx context.Context, candidatePath, baselinePath string) (imageRect, error) {
	if err := g.sem.Acquire(ctx, 1); err != nil {
		return imageRect{}, err
	}
	defer g.sem.Release(1)

	args := []string{
		candidatePath, baselinePath,
		"-compose", "difference", "-composite",
		"-colorspace", "Gray", "-threshold", "0",
		"-background", "black",
		"-format", "%@", "info:",
	}
	output, err := runMagick(ctx, args...)
	if err != nil {
		return imageRect{}, fmt.Errorf("ImageMagick diff error: %w", err)
	}
	return parseRect(string(output))
}

var rectPattern = regexp.MustCompile(`(\d+)x(\d+)([+-]\d+)([+-]\d+)`)

func parseRect(output string) (imageRect, error) {
	match := rectPattern.FindStringSubmatch(output)
	if match == nil {
		return imageRect{}, fmt.Errorf("unexpected ImageMagick diff geometry: %q", output)
	}
	values := make([]int, 4)
	for i := range values {
		v, err := strconv.Atoi(match[i+1])
		if err != nil {
			return imageRect{}, fmt.Errorf("parse ImageMagick diff geometry %q: %w", output, err)
		}
		values[i] = v
	}
	return imageRect{W: values[0], H: values[1], X: values[2], Y: values[3]}, nil
}

// encode 先查 (图片, 模式, 坐标) 缓存；未命中时渲染并写入 CAS。
func (g *MagickSampleGenerator) encode(ctx context.Context, sourcePath string, mode patchMode, rect imageRect) (string, error) {
	key := patchKey{sourcePath: sourcePath, mode: mode, rect: rect}
	g.mu.Lock()
	if cid, ok := g.cache[key]; ok {
		g.mu.Unlock()
		return cid, nil
	}
	g.mu.Unlock()

	data, err := g.render(ctx, sourcePath, mode, rect)
	if err != nil {
		return "", err
	}
	cid, err := g.cas.Put(data)
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
func (g *MagickSampleGenerator) render(ctx context.Context, sourcePath string, mode patchMode, rect imageRect) ([]byte, error) {
	if err := g.sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer g.sem.Release(1)

	size := fmt.Sprintf("%dx%d", patchSize, patchSize)
	args := []string{sourcePath, "-coalesce"}
	if mode == patchModePatch {
		crop := fmt.Sprintf("%dx%d%+d%+d", rect.W, rect.H, rect.X, rect.Y)
		args = append(args, "-crop", crop, "+repage")
	}
	args = append(args, "-resize", size+"!", "-quality", fmt.Sprint(patchQuality), "webp:-")

	data, err := runMagick(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("ImageMagick sample patch error: %w", err)
	}
	return data, nil
}

func runMagick(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "magick", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w, stderr: %q", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// #region 原图尺寸读取

type magickDimensionReader struct{}

func (r *magickDimensionReader) Dimensions(ctx context.Context, path string) (int, int, error) {
	output, err := runMagick(ctx, "identify", "-ping", "-format", "%w %h", path)
	if err != nil {
		return 0, 0, fmt.Errorf("ImageMagick identify error: %w", err)
	}
	var width, height int
	if _, err := fmt.Sscanf(string(output), "%d %d", &width, &height); err != nil {
		return 0, 0, fmt.Errorf("parse ImageMagick identify output %q: %w", output, err)
	}
	return width, height, nil
}

// #endregion
