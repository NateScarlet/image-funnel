package image

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"time"
	"main/internal/shared"
	"path/filepath"

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
	queue     TranscodeQueue
	// encoderID 当前生效的编码器标识，参与变体 key 计算
	encoderID string
	logger    *zap.Logger
}

func NewTranscodeCoordinator(cache Cache, processor Processor, queue TranscodeQueue, encoderID string, logger *zap.Logger) *TranscodeCoordinator {
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
func VariantKey(absPath string, spec Spec, encoder string) (string, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	timestamp := fmt.Sprintf("%d", info.ModTime().UnixNano())
	size := fmt.Sprintf("%d", info.Size())
	wStr := ""
	if spec.Width() > 0 {
		wStr = fmt.Sprintf("%d", spec.Width())
	}
	qStr := ""
	if spec.Quality() > 0 {
		qStr = fmt.Sprintf("%d", spec.Quality())
	}

	hash := sha256.New()
	fmt.Fprintf(hash, "%s|%s|%s|%s|%s|%s|%s", filepath.Base(absPath), timestamp, size, wStr, qStr, spec.Format().String(), encoder)
	return base64.URLEncoding.EncodeToString(hash.Sum(nil)), nil
}

// Acquire 获取一个图片变体：缓存命中直接返回；未命中入队等待转码完成。
// priority 表达本次需求类别（用户查看=High，预载=Low）。
// ctx 结束（客户端断开/请求放弃）时自动撤回需求登记——「撤回淘汰未启动任务」
// 的语义由此保证，调用方无需手动 Cancel
func (c *TranscodeCoordinator) Acquire(ctx context.Context, absPath string, spec Spec, priority Prio) (File, error) {
	// WebP 且无缩放/质量参数时返回原始文件（向后兼容）；AVIF 全分辨率也需要转码
	if spec.Width() == 0 && spec.Quality() == 0 && spec.Format() == ImageFormatWebP {
		return &rawFile{path: absPath}, nil
	}

	key, err := VariantKey(absPath, spec, c.encoderID)
	if err != nil {
		return nil, err
	}

	file, err := c.cache.Lookup(ctx, key)
	if err != nil {
		return nil, err
	}
	if file != nil {
		return file, nil
	}

	item, err := NewTranscodeItem(key, absPath, spec, priority)
	if err != nil {
		return nil, err
	}
	waiter, err := c.queue.Enqueue(ctx, item)
	if err != nil {
		return nil, err
	}
	// ctx 结束即撤回需求（HEAD abort / GET 放弃），无论 Wait 结果如何
	defer waiter.Cancel()
	return waiter.Wait(ctx)
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
		go c.worker(ctx, PrioLow)
	}
	if n > 1 {
		go c.worker(ctx, PrioHigh)
	} else {
		go c.worker(ctx, PrioLow)
	}
}

// worker 从队列拉取任务并执行转码。Consume 保证产出的任务都有活跃等待者
func (c *TranscodeCoordinator) worker(ctx context.Context, minPriority Prio) {
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
func (c *TranscodeCoordinator) runJob(ctx context.Context, job TranscodeJob) {
	item := job.Item()
	spec := item.Spec()

	start := time.Now()
	c.logger.Info("will transcode variant",
		zap.String("key", item.Key()),
		zap.Int("width", spec.Width()),
		zap.Int("quality", spec.Quality()),
		zap.String("format", spec.Format().String()),
	)

	// 转码与请求生命周期解耦：使用独立 ctx，客户端断开不取消已开始的转码
	// （已确认语义：已启动的任务跑完入缓存）
	var buf transcodeBuffer
	err := c.processor.Process(ctx, item.SrcPath(), spec, &buf)
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

func (c *TranscodeCoordinator) saveToCache(key string, file File) error {
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

// Open 实现 File 接口，返回产物内容的读取器
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

// rawFile 原始文件直通句柄（webp 无参捷径）
type rawFile struct {
	path string
}

// Open 实现 File 接口，直接打开源文件
func (f *rawFile) Open() (io.ReadSeekCloser, error) {
	return os.Open(f.path)
}
