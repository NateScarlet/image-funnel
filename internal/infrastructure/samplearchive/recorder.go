package samplearchive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"main/internal/domain/session"

	"go.uber.org/zap"
)

// SampleImageGenerator 在后台从候选与基准图像生成一组 224² 块，各自按 CID 落盘，
// 返回候选块 CID 与基准块 CID 列表（顺序与传入的基准一致）。
// 图像以版本化 SampleImage 传递；解码由生成器注入的 ImageDecoder 负责。
type SampleImageGenerator interface {
	Generate(ctx context.Context, candidate session.SampleImage, baselines []session.SampleImage) (candidateCID string, baselineCIDs []string, err error)
}

// PromptReader 尽力读取 PNG 中的 ComfyUI prompt JSON；读不到时返回 nil。
type PromptReader interface {
	Prompt(path string) []byte
}

// Record 是 v2 JSONL 中的一条归档记录。一条记录 = 一次 Mark 决策 = 一组图像产物：
// 候选块 + 本次 Mark 前全部基准块，各一个 CID，共用这条记录的 action 标签。
//
// BaselineImageIDs 与 BaselineCIDs 一一对应；无基准时两者都写空数组，不伪造。
// rating 不进入 v2 记录：它只是 action 的 XMP 载体，不是训练标签或置信度权重。
type Record struct {
	ImageID          string    `json:"imageId"`
	DirectoryID      string    `json:"directoryId"`
	SessionID        string    `json:"sessionId"`
	Round            int       `json:"round"`
	Action           string    `json:"action"`
	Timestamp        time.Time `json:"timestamp"`
	CandidateCID     string    `json:"candidateCid"`
	BaselineCIDs     []string  `json:"baselineCids"`
	BaselineImageIDs []string  `json:"baselineImageIds"`
	Fingerprint      string    `json:"fingerprint,omitempty"`
}

// Recorder 把标注决策蒸馏为只增不改的派生快照：图像块按 CID 落盘，JSONL 只记引用。
type Recorder struct {
	generator SampleImageGenerator
	prompts   PromptReader
	logPath   string
	baseDir   string
	logger    *zap.Logger
	mu        sync.Mutex
}

// NewRecorder 在 dataRoot 下建立 samples.jsonl；baseDir 用于把样本中的根相对路径解析为绝对路径。
// dataRoot 由装配方给定为 <DataDir>/training-samples-v2。
func NewRecorder(dataRoot string, baseDir string, generator SampleImageGenerator, prompts PromptReader, logger *zap.Logger) (*Recorder, error) {
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create sample archive directory: %w", err)
	}
	return &Recorder{
		generator: generator,
		prompts:   prompts,
		logPath:   filepath.Join(dataRoot, "samples.jsonl"),
		baseDir:   baseDir,
		logger:    logger,
	}, nil
}

// Record 生成候选块与基准块、按 CID 去重落盘并追加一条 JSONL 记录；任何失败都返回给调用者。
// 各阶段耗时以 Debug 级记录，便于定位采样缓慢的具体来源。
func (r *Recorder) Record(ctx context.Context, sample *session.Sample) error {
	stageStart := time.Now()
	defer func() {
		r.logger.Debug("did record training sample in archive",
			zap.Duration("duration", time.Since(stageStart)),
			zap.Stringer("imageID", sample.ImageID),
			zap.Int("baselines", len(sample.Baselines)),
		)
	}()

	baselineIDs := make([]string, 0, len(sample.Baselines))
	for _, baseline := range sample.Baselines {
		baselineIDs = append(baselineIDs, baseline.ID.String())
	}

	candidate := session.SampleImage{ID: sample.ImageID, RelPath: sample.SourceRelPath}
	sourcePath := sample.SourceRelPath.Abs(r.baseDir)
	generateStart := time.Now()
	candidateCID, baselineCIDs, err := r.generator.Generate(ctx, candidate, sample.Baselines)
	r.logger.Debug("did generate sample images",
		zap.Duration("duration", time.Since(generateStart)),
		zap.Stringer("imageID", sample.ImageID),
		zap.Int("baselines", len(sample.Baselines)),
		zap.Error(err),
	)
	if err != nil {
		return fmt.Errorf("generate training sample images: %w", err)
	}
	// 无基准时写空数组而不是 null，明确表达「本次没有参照」而非数据缺失
	if baselineCIDs == nil {
		baselineCIDs = make([]string, 0)
	}

	// 指纹降级为辅助元数据，不再作为切分主键；PNG 元数据读取可能很慢，单独计时。
	fingerprintStart := time.Now()
	fingerprint := Fingerprint(r.prompts.Prompt(sourcePath))
	r.logger.Debug("did read prompt and compute fingerprint",
		zap.Duration("duration", time.Since(fingerprintStart)),
		zap.Stringer("imageID", sample.ImageID),
	)

	line, err := json.Marshal(Record{
		ImageID:          sample.ImageID.String(),
		DirectoryID:      sample.DirectoryID.String(),
		SessionID:        sample.SessionID.String(),
		Round:            sample.Round,
		Action:           sample.Action.String(),
		Timestamp:        sample.Timestamp,
		CandidateCID:     candidateCID,
		BaselineCIDs:     baselineCIDs,
		BaselineImageIDs: baselineIDs,
		Fingerprint:      fingerprint,
	})
	if err != nil {
		return fmt.Errorf("encode training sample: %w", err)
	}

	appendStart := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := func() error {
		file, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.Write(append(line, '\n'))
		return err
	}(); err != nil {
		return fmt.Errorf("append training sample log: %w", err)
	}
	r.logger.Debug("did append training sample log",
		zap.Duration("duration", time.Since(appendStart)),
		zap.Stringer("imageID", sample.ImageID),
	)
	return nil
}

var _ session.SampleRecorder = (*Recorder)(nil)
