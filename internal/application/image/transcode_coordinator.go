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

// TranscodeCoordinator 图片变体转码的编排入口：
// 先查缓存（命中即返回，不碰队列），未命中则入队转码并等待结果。
// 同时负责启动 worker 池从队列拉取任务执行转码并写入缓存
type TranscodeCoordinator struct {
	cache     Cache
	processor Processor
	queue     transcode.Queue
	// encoderID 当前生效的编码器标识，参与变体 key 计算
	encoderID string
	logger    *zap.Logger
}

func NewTranscodeCoordinator(cache Cache, processor Processor, queue transcode.Queue, encoderID string, logger *zap.Logger) *TranscodeCoordinator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TranscodeCoordinator{
		cache:     cache,
		processor: processor,
		queue:     queue,
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

// CachedVariant 只查变体缓存：仅 stat 缓存文件，不打开源图、不入队。
// 返回 (nil, nil) 表示缓存未命中。用于在决定直出/重定向之前免去一次源图打开
func (c *TranscodeCoordinator) CachedVariant(ctx context.Context, absPath string, spec transcode.Spec) (transcode.File, transcode.VariantTiming, error) {
	key, err := VariantKey(absPath, spec, c.encoderID)
	if err != nil {
		return nil, transcode.VariantTiming{}, err
	}
	file, err := c.cache.Lookup(ctx, key)
	return file, transcode.VariantTiming{CacheHit: file != nil}, err
}

// Acquire 获取一个图片变体：缓存命中直接返回；未命中入队等待转码完成。
// priority 表达本次需求类别（用户查看=High，预载=Low）。
// ctx 结束（客户端断开/请求放弃）时自动撤回需求登记——「撤回淘汰未启动任务」
// 的语义由此保证：登记生命周期由入队时传入的 ctx 表达，无需额外句柄
func (c *TranscodeCoordinator) Acquire(ctx context.Context, absPath string, spec transcode.Spec, priority transcode.Prio) (transcode.File, transcode.VariantTiming, error) {
	key, err := VariantKey(absPath, spec, c.encoderID)
	if err != nil {
		return nil, transcode.VariantTiming{}, err
	}

	file, err := c.cache.Lookup(ctx, key)
	if err != nil {
		return nil, transcode.VariantTiming{}, err
	}
	if file != nil {
		return file, transcode.VariantTiming{CacheHit: true}, nil
	}

	// 计时指针随 item 进入队列：入队时刻由这里给出，排队时长由队列认领时写入，
	// 编码时长由 worker 写入，三处共享同一结构避免额外的传递通道
	timing := &transcode.VariantTiming{EnqueuedAt: time.Now()}
	item, err := transcode.NewItem(key, absPath, spec, priority, timing)
	if err != nil {
		return nil, transcode.VariantTiming{}, err
	}
	waiter, err := c.queue.Enqueue(ctx, item)
	if err != nil {
		return nil, transcode.VariantTiming{}, err
	}
	// ctx 结束即撤回需求（HEAD abort / GET 放弃），由 Enqueue 的 ctx 生命周期承担
	file, err = waiter.Wait(ctx)
	return file, derefTiming(waiter.Timing()), err
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
