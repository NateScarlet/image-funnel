//go:build windows

package localfs

import (
	"context"
	"iter"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"main/internal/domain/directory"
	"main/internal/shared"

	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

// Watcher 文件系统监控器 (Windows 原生实现)
// 避免 fsnotify 在 Windows 下监控子目录导致的目录被占用问题
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
func (w *Watcher) Watch(ctx context.Context, dir string) iter.Seq2[*directory.FileChange, error] {
	return func(yield func(*directory.FileChange, error) bool) {
		dirPath16, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			yield(nil, err)
			return
		}

		// 使用 ReadDirectoryChangesW 来实现递归监控
		// 这样只需要对根目录持有句柄，而不需要对每个子目录都持有句柄
		// 极大减少因为我们持有文件句柄而导致 Windows Explorer 无法删除文件夹的问题
		handle, err := windows.CreateFile(
			dirPath16,
			windows.FILE_LIST_DIRECTORY,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil,
			windows.OPEN_EXISTING,
			windows.FILE_FLAG_BACKUP_SEMANTICS,
			0,
		)
		if err != nil {
			w.logger.Error("failed to create directory watch handle", zap.String("dir", dir), zap.Error(err))
			yield(nil, err)
			return
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()
			// 取消挂起的异步或同步 IO 请求
			windows.CancelIoEx(handle, nil)
			windows.CloseHandle(handle)
		}()

		w.logger.Info("started watching directory recursively on windows", zap.String("dir", dir))
		defer func() {
			w.logger.Info("stopped watching directory", zap.String("dir", dir))
			wg.Wait()
		}()

		buf := make([]byte, 64*1024)
		var ret uint32

		for {
			err := windows.ReadDirectoryChanges(
				handle,
				&buf[0],
				uint32(len(buf)),
				true, // watchSubTree = true, Windows 原生支持递归监控
				windows.FILE_NOTIFY_CHANGE_FILE_NAME|
					windows.FILE_NOTIFY_CHANGE_DIR_NAME|
					windows.FILE_NOTIFY_CHANGE_ATTRIBUTES|
					windows.FILE_NOTIFY_CHANGE_SIZE|
					windows.FILE_NOTIFY_CHANGE_LAST_WRITE,
				&ret,
				nil,
				0,
			)

			if err != nil || ret == 0 {
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					w.logger.Error("watcher error", zap.Error(err))
					if !yield(nil, err) {
						return
					}
				}
				return
			}

			offset := uint32(0)
			for {
				if offset+12 > ret {
					break // 数据不足以解析 FILE_NOTIFY_INFORMATION 头部
				}
				info := (*windows.FileNotifyInformation)(unsafe.Pointer(&buf[offset]))
				nameLen := info.FileNameLength / 2
				if offset+12+info.FileNameLength > ret {
					break // 防止越界
				}

				namePtr := (*[0xffff]uint16)(unsafe.Pointer(&info.FileName))[:nameLen:nameLen]
				name := windows.UTF16ToString(namePtr)

				// 忽略临时文件
				if !strings.HasSuffix(name, ".tmp") {
					var action shared.FileAction
					switch info.Action {
					case windows.FILE_ACTION_ADDED, windows.FILE_ACTION_RENAMED_NEW_NAME:
						action = shared.FileActionCreate
					case windows.FILE_ACTION_MODIFIED:
						action = shared.FileActionWrite
					case windows.FILE_ACTION_REMOVED:
						action = shared.FileActionRemove
					case windows.FILE_ACTION_RENAMED_OLD_NAME:
						action = shared.FileActionRename // 重命名的旧文件，按照规范处理为 Rename，等价于 Remove
					default:
						goto next
					}

					absPath := filepath.Join(dir, name)
					if !yield(directory.NewFileChange(absPath, action, time.Now()), nil) {
						return
					}
				}

			next:
				if info.NextEntryOffset == 0 {
					break
				}
				offset += info.NextEntryOffset
			}
		}
	}
}

// 确保实现了接口
var _ directory.Watcher = (*Watcher)(nil)
