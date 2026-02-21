//go:build !windows

package localfs

import (
	"context"
	"io/fs"
	"iter"
	"main/internal/domain/directory"
	"main/internal/shared"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// Watcher 文件系统监控器
type Watcher struct {
	logger *zap.Logger
}

// NewWatcher 创建文件系统监控器
func NewWatcher(logger *zap.Logger) *Watcher {
	return &Watcher{
		logger: logger,
	}
}

// Watch 监听指定目录的文件变更
// 每次调用都会创建新的 fsnotify.Watcher 实例，避免共享通道
func (w *Watcher) Watch(ctx context.Context, dir string) iter.Seq2[*directory.FileChange, error] {
	return func(yield func(*directory.FileChange, error) bool) {
		// 创建 fsnotify watcher
		fsWatcher, err := fsnotify.NewWatcher()
		if err != nil {
			w.logger.Error("failed to create fsnotify watcher", zap.Error(err))
			yield(nil, err)
			return
		}
		defer fsWatcher.Close()

		// 递归添加监控路径
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// 如果是根目录无法访问，返回错误
				if path == dir {
					return err
				}
				// 子目录无法访问，记录警告但不中断
				w.logger.Warn("failed to walk directory", zap.String("path", path), zap.Error(err))
				return filepath.SkipDir
			}
			if d.IsDir() {
				if err := fsWatcher.Add(path); err != nil {
					w.logger.Warn("failed to add watch path", zap.String("path", path), zap.Error(err))
				}
			}
			return nil
		})

		if err != nil {
			w.logger.Error("failed to setup recursive watcher", zap.String("dir", dir), zap.Error(err))
			yield(nil, err)
			return
		}

		w.logger.Info("started watching directory recursively", zap.String("dir", dir))

		// 监听事件
		for {
			select {
			case <-ctx.Done():
				w.logger.Info("stopped watching directory", zap.String("dir", dir))
				return
			case fsEvent, ok := <-fsWatcher.Events:
				if !ok {
					return
				}

				fileChange, ok := w.handleEvent(fsWatcher, fsEvent)
				if ok {
					if !yield(fileChange, nil) {
						return
					}
				}
			case err, ok := <-fsWatcher.Errors:
				if !ok {
					return
				}
				w.logger.Error("watcher error", zap.Error(err))
				if !yield(nil, err) {
					return
				}
			}
		}
	}
}

// handleEvent 处理文件系统事件，转换为领域对象
func (w *Watcher) handleEvent(fsWatcher *fsnotify.Watcher, fsEvent fsnotify.Event) (*directory.FileChange, bool) {
	// 自动监控新建的子目录或重命名的目录
	if fsEvent.Op&fsnotify.Create == fsnotify.Create || fsEvent.Op&fsnotify.Rename == fsnotify.Rename {
		info, err := os.Stat(fsEvent.Name)
		if err != nil {
			// 文件可能在创建后立即被删除或不可访问，仅记录调试日志
			w.logger.Debug("failed to stat created file", zap.String("path", fsEvent.Name), zap.Error(err))
		} else if info.IsDir() {
			w.logger.Info("watching new directory", zap.String("dir", fsEvent.Name))
			err := filepath.WalkDir(fsEvent.Name, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					w.logger.Warn("failed to walk new directory", zap.String("path", path), zap.Error(err))
					return filepath.SkipDir
				}
				if d.IsDir() {
					if err := fsWatcher.Add(path); err != nil {
						w.logger.Warn("failed to add watch path", zap.String("path", path), zap.Error(err))
					}
				}
				return nil
			})
			if err != nil {
				w.logger.Error("failed to walk new directory structure", zap.String("dir", fsEvent.Name), zap.Error(err))
			}
		}
	}
	// 忽略临时文件
	if strings.HasSuffix(fsEvent.Name, ".tmp") {
		return nil, false
	}

	var action shared.FileAction
	switch {
	case fsEvent.Op&fsnotify.Create == fsnotify.Create:
		action = shared.FileActionCreate
	case fsEvent.Op&fsnotify.Write == fsnotify.Write:
		action = shared.FileActionWrite
	case fsEvent.Op&fsnotify.Remove == fsnotify.Remove:
		action = shared.FileActionRemove
	case fsEvent.Op&fsnotify.Rename == fsnotify.Rename:
		action = shared.FileActionRename
	default:
		return nil, false
	}

	return directory.NewFileChange(fsEvent.Name, action, time.Now()), true
}

// 确保实现了接口
var _ directory.Watcher = (*Watcher)(nil)
