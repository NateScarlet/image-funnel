package image

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"main/internal/shared"
	"main/internal/shared/imageversion"
	"main/internal/shared/transcode"
	"main/internal/util"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// 编码器标识：进入变体 key，避免不同编码器产物混用缓存。
// ffmpeg 实现的标识引用此处的共享常量
const (
	EncoderMagick = "magick"
	EncoderSVTAV1 = "svt-av1"
)

// TranscodeCoordinator 图片变体取回与转码的编排入口：
// 对外只暴露 Variant 一个 seam——计算一次 key、查一次缓存、未命中探源一次，
// 并直接产出完整的响应形态决策；需要转码时用同一个 key 入队并等待结果。
// 同时负责启动 worker 池从队列拉取任务执行转码并写入缓存
type TranscodeCoordinator struct {
	cache     Cache
	processor Processor
	queue     transcode.Queue
	probe     SourceProbe
	// encoderID 当前生效的编码器标识，参与变体 key 计算
	encoderID string
	logger    *zap.Logger
}

func NewTranscodeCoordinator(cache Cache, processor Processor, queue transcode.Queue, probe SourceProbe, encoderID string, logger *zap.Logger) *TranscodeCoordinator {
	if probe == nil {
		panic("transcode coordinator requires a source probe")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TranscodeCoordinator{
		cache:     cache,
		processor: processor,
		queue:     queue,
		probe:     probe,
		encoderID: encoderID,
		logger:    logger,
	}
}

// VariantKey 计算图片变体的缓存键：源图文件名、修改时间、大小 + 规格 + 编码器标识。
// 使用文件名而非绝对路径，从而允许文件移动后复用已转码的缓存（只要文件名、修改时间和大小不变）
func VariantKey(absPath string, spec transcode.Spec, encoder string) (string, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	// 图片版本只由 shared/imageversion 定义；变体 key 取其中的纳秒级+大小视图。
	// 与签名的秒级视图精度不同是 ADR 0004 接受的已知权衡，不在此处各自取字段
	version := imageversion.New(info).Nano()
	wStr := ""
	if spec.Width() > 0 {
		wStr = fmt.Sprintf("%d", spec.Width())
	}

	hash := sha256.New()
	fmt.Fprintf(hash, "%s|%d|%d|%s|%s|%s", filepath.Base(absPath), version.ModNano, version.Size, wStr, spec.Format().String(), encoder)
	return base64.URLEncoding.EncodeToString(hash.Sum(nil)), nil
}

// EncoderID 当前生效的编码器标识，供响应头 desc 区分编码器
func (c *TranscodeCoordinator) EncoderID() string { return c.encoderID }

// Variant 取回图片变体，或直接给出重定向决策——应用层唯一的变体取回 seam。
//
// 一次调用完成：计算一次变体 key、执行一次缓存 Lookup、未命中探源一次，
// 并产出完整响应形态；需要转码时用与 Lookup 同一个 key 入队并等待，
// 不再二次计算 key 或二次查询缓存。HEAD/GET 优先级由输入表达。
// ctx 结束（客户端断开/请求放弃）时由入队的 ctx 撤回需求登记
func (c *TranscodeCoordinator) Variant(ctx context.Context, in VariantInput) (VariantResult, error) {
	// 查询用请求宽度——源宽此时不可知，变体键形状因此保持不变。
	// w=0 是「全尺寸诉求」，沿用既有规则：不查询变体缓存，探源后决定直出/转码
	var (
		spec transcode.Spec
		key  string
		err  error
	)
	if in.Width > 0 {
		spec, key, err = c.keyFor(in.AbsPath, in.Width, in.Format)
		if err != nil {
			return VariantResult{}, err
		}
	}

	var cachedFile transcode.File
	if key != "" {
		cachedFile, err = c.cache.Lookup(ctx, key)
		if err != nil {
			return VariantResult{}, err
		}
	}

	result := VariantResult{CacheHit: cachedFile != nil}
	if cachedFile != nil {
		// 命中即服务缓存：不探源、不重定向、不入队
		result.Kind = ResponseServeCached
		result.Format = in.Format
		result.File = cachedFile
		return result, nil
	}

	// 未命中：一次打开源图，同时取得宽度与 MIME；解码耗时就地测量
	decodeStart := time.Now()
	source, err := c.probe.Probe(ctx, in.AbsPath)
	result.DecodeDuration = time.Since(decodeStart)
	if err != nil {
		return VariantResult{}, err
	}
	defer source.Reader.Close()

	decision := DecideResponse(ResponseInput{
		Accept:           in.Accept,
		Width:            in.Width,
		SourceWidth:      source.Width,
		SourceAcceptable: util.Accepts(in.Accept, source.ContentType),
		Format:           in.Format,
	})
	result.Kind = decision.Kind
	result.Format = decision.Format
	result.CanonicalWidth = decision.CanonicalWidth
	result.EffectiveWidth = decision.EffectiveWidth
	if decision.Kind != ResponseTranscode {
		return result, nil
	}

	if key == "" {
		// w=0 没有缓存查询，入队 key 在此计算一次
		spec, key, err = c.keyFor(in.AbsPath, decision.EffectiveWidth, in.Format)
		if err != nil {
			return VariantResult{}, err
		}
	}

	// 计时指针随 item 进入队列：入队时刻由这里给出，排队时长由队列认领时写入，
	// 编码时长由 worker 写入，三处共享同一结构避免额外的传递通道。
	// spec 与 Lookup 共用（转码分支的有效宽度恒等于请求宽度），key 因此完全相同
	timing := &transcode.VariantTiming{EnqueuedAt: time.Now()}
	item, err := transcode.NewItem(key, in.AbsPath, spec, in.Priority, timing)
	if err != nil {
		return VariantResult{}, err
	}
	waiter, err := c.queue.Enqueue(ctx, item)
	if err != nil {
		return VariantResult{}, err
	}
	file, err := waiter.Wait(ctx)
	variantTiming := derefTiming(waiter.Timing())
	result.QueueDuration = variantTiming.QueueDuration
	result.EncodeDuration = variantTiming.EncodeDuration
	if err != nil {
		return VariantResult{}, err
	}
	result.File = file
	return result, nil
}

// keyFor 是 seam 内唯一的变体 key 计算入口：规格与 key 成对产出，
// 调用方拿到的 spec 与 key 必然一致，避免分别计算导致的键与产物错配
func (c *TranscodeCoordinator) keyFor(absPath string, width int, format transcode.ImageFormat) (transcode.Spec, string, error) {
	spec, err := transcode.NewSpec(width, format)
	if err != nil {
		return transcode.Spec{}, "", err
	}
	key, err := VariantKey(absPath, spec, c.encoderID)
	if err != nil {
		return transcode.Spec{}, "", err
	}
	return spec, key, nil
}

func derefTiming(t *transcode.VariantTiming) transcode.VariantTiming {
	if t == nil {
		return transcode.VariantTiming{}
	}
	return *t
}

// StartWorkers 启动 worker 池：n-1 个通用 worker（收全部优先级）+ 1 个高优先级
// 专职 worker（只收 High，保证用户查看请求永远有立即可用的执行槽位）。
// n<=1 时仅启动 1 个通用 worker（预热与查看共享唯一槽位，无预热可用性可言）。
// 随 ctx 取消停止
func (c *TranscodeCoordinator) StartWorkers(ctx context.Context, n int) {
	if n < 1 {
		n = 1
	}
	general := n - 1
	for i := 0; i < general; i++ {
		go c.worker(ctx, transcode.PrioLow)
	}
	if n > 1 {
		go c.worker(ctx, transcode.PrioHigh)
	} else {
		go c.worker(ctx, transcode.PrioLow)
	}
}

// worker 从队列拉取任务并执行转码。Consume 保证产出的任务都有活跃等待者
func (c *TranscodeCoordinator) worker(ctx context.Context, minPriority transcode.Prio) {
	for job, err := range c.queue.Consume(ctx, minPriority) {
		if err != nil {
			if ctx.Err() == nil {
				c.logger.Error("transcode queue consume failed", zap.Error(err))
			}
			return
		}
		c.runJob(ctx, job)
	}
}

// runJob 执行单个转码任务：Process（纯转码）→ cache.Save → Resolve 广播结果
func (c *TranscodeCoordinator) runJob(ctx context.Context, job transcode.Job) {
	item := job.Item()
	spec := item.Spec()

	start := time.Now()
	c.logger.Info("will transcode variant",
		zap.String("key", item.Key()),
		zap.Int("width", spec.Width()),
		zap.String("format", spec.Format().String()),
	)

	// 转码与请求生命周期解耦：使用独立 ctx，客户端断开不取消已开始的转码
	// （已确认语义：已启动的任务跑完入缓存）
	encodeStart := time.Now()
	var buf transcodeBuffer
	err := c.processor.Process(ctx, item.SrcPath(), spec, &buf)
	if timing := item.Timing(); timing != nil {
		timing.EncodeDuration = time.Since(encodeStart)
	}
	c.logger.Info("did transcode variant",
		zap.String("key", item.Key()),
		zap.Duration("duration", time.Since(start)),
		zap.Error(err),
	)
	if err != nil {
		job.Resolve(nil, err)
		return
	}
	file := &bufferFile{data: buf.bytes()}
	if saveErr := c.saveToCache(item.Key(), file); saveErr != nil {
		c.logger.Error("failed to save transcode result to cache", zap.String("key", item.Key()), zap.Error(saveErr))
	}
	job.Resolve(file, nil)
}

// #region 内部辅助

func (c *TranscodeCoordinator) saveToCache(key string, file transcode.File) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	return c.cache.Save(context.Background(), key, reader)
}

// #endregion

// transcodeBuffer 收集转码输出的内存缓冲。
// 变体产物体积有限（缩放图几十 KB~几 MB，全尺寸图十几 MB），内存缓冲可接受，
// 且能保证 Save 失败时不产生半成品缓存文件
type transcodeBuffer struct {
	data []byte
}

func (b *transcodeBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *transcodeBuffer) bytes() []byte { return b.data }

// bufferFile 已转码产物的内存句柄
type bufferFile struct {
	data []byte
}

// Open 实现 transcode.File 接口，返回产物内容的读取器
func (f *bufferFile) Open() (io.ReadSeekCloser, error) {
	return &seeker{Reader: bytes.NewReader(f.data)}, nil
}

type seeker struct {
	Reader *bytes.Reader
}

func (s *seeker) Read(p []byte) (int, error) { return s.Reader.Read(p) }

func (s *seeker) Seek(offset int64, whence int) (int64, error) {
	return s.Reader.Seek(offset, whence)
}

func (s *seeker) Close() error { return nil }

// Meta 委托给底层处理链（domain Factory 也依赖 Meta 读取图片尺寸）
func (c *TranscodeCoordinator) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	return c.processor.Meta(ctx, srcPath)
}
