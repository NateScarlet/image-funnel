package localfs

import (
	"context"
	"os"
	"path/filepath"

	"main/internal/domain/loadlatency"
)

// LoadLatencyAppender 把诊断段追加到 <dir>/<runId>.jsonl。
// 目录在构造时解析为绝对路径：服务端没有工作区概念，运行期不得再依赖当前目录。
type LoadLatencyAppender struct {
	dir string
}

func NewLoadLatencyAppender(dir string) (*LoadLatencyAppender, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &LoadLatencyAppender{dir: abs}, nil
}

// Dir 返回落盘目录的绝对路径，供启动时打日志。
func (a *LoadLatencyAppender) Dir() string { return a.dir }

// Append 纯追加一段内容，不读已有内容、不去重。返回写出的绝对路径。
func (a *LoadLatencyAppender) Append(ctx context.Context, runID string, data []byte) (string, error) {
	path := loadlatency.SegmentPath(a.dir, runID)
	if err := os.MkdirAll(a.dir, 0o755); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 单次 Write 即完整追加一段；ctx 不参与取消，避免半途写入留下残段
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	return path, nil
}
