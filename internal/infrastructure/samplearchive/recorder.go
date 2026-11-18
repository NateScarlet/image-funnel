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
)

// Thumbnailer 在提交当下从原图生成 224×224 缩略图字节。
type Thumbnailer interface {
	Thumbnail(ctx context.Context, srcPath string) ([]byte, error)
}

// PromptReader 尽力读取 PNG 中的 ComfyUI prompt JSON；读不到时返回 nil。
type PromptReader interface {
	Prompt(path string) []byte
}

// Record 是 JSONL 中的一条归档记录，通过 CID 引用缩略图，不重复写字节。
type Record struct {
	ImageID      string    `json:"imageId"`
	DirectoryID  string    `json:"directoryId"`
	SessionID    string    `json:"sessionId"`
	Round        int       `json:"round"`
	Action       string    `json:"action"`
	Rating       int       `json:"rating"`
	Timestamp    time.Time `json:"timestamp"`
	ThumbnailCID string    `json:"thumbnailCid"`
	Fingerprint  string    `json:"fingerprint,omitempty"`
}

// Recorder 把标注决策蒸馏为只增不改的派生快照：缩略图按 CID 落盘，JSONL 只记引用。
type Recorder struct {
	cas     *CAS
	thumbs  Thumbnailer
	prompts PromptReader
	logPath string
	mu      sync.Mutex
}

// NewRecorder 在 dataRoot 下建立 cas/ 目录与 samples.jsonl；dataRoot 由装配方给定，
// 主应用固定传入项目内的 data 目录，使落盘布局为 data/cas/<分片>/<h>.data。
func NewRecorder(dataRoot string, thumbs Thumbnailer, prompts PromptReader) (*Recorder, error) {
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create sample archive directory: %w", err)
	}
	return &Recorder{
		cas:     NewCAS(filepath.Join(dataRoot, "cas")),
		thumbs:  thumbs,
		prompts: prompts,
		logPath: filepath.Join(dataRoot, "samples.jsonl"),
	}, nil
}

// Record 生成缩略图、按 CID 去重落盘并追加一条 JSONL 记录；任何失败都返回给调用者。
func (r *Recorder) Record(ctx context.Context, sample *session.Sample) error {
	thumb, err := r.thumbs.Thumbnail(ctx, sample.SourcePath)
	if err != nil {
		return fmt.Errorf("generate training thumbnail: %w", err)
	}

	cid, err := r.cas.Put(thumb)
	if err != nil {
		return fmt.Errorf("store training thumbnail: %w", err)
	}

	// 指纹是启发式分组键，解析不出（无 PNG 元数据 / 非法 JSON）时留空，不影响归档
	fingerprint := Fingerprint(r.prompts.Prompt(sample.SourcePath))

	line, err := json.Marshal(Record{
		ImageID:      sample.ImageID.String(),
		DirectoryID:  sample.DirectoryID.String(),
		SessionID:    sample.SessionID.String(),
		Round:        sample.Round,
		Action:       sample.Action.String(),
		Rating:       sample.Rating,
		Timestamp:    sample.Timestamp,
		ThumbnailCID: cid,
		Fingerprint:  fingerprint,
	})
	if err != nil {
		return fmt.Errorf("encode training sample: %w", err)
	}

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
	return nil
}

var _ session.SampleRecorder = (*Recorder)(nil)
