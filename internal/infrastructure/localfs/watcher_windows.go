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
			windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED,
			0,
		)
		if err != nil {
			w.logger.Error("failed to create directory watch handle", zap.String("dir", dir), zap.Error(err))
			yield(nil, err)
			return
		}
		defer windows.CloseHandle(handle)

		// 缓冲区大小减小到 32KB 以提高兼容性
		bufLen := uint32(32768)
		// 使用 windows.LocalAlloc 分配 DWORD 对齐的内存，避免 Go GC 对齐限制
		hMem, err := windows.LocalAlloc(windows.LMEM_FIXED, bufLen)
		if err != nil {
			w.logger.Error("failed to allocate aligned memory", zap.Error(err))
			yield(nil, err)
			return
		}
		defer windows.LocalFree(windows.Handle(hMem))
		bufPtr := (*byte)(unsafe.Add(unsafe.Pointer(nil), uintptr(hMem)))

		// 初始化 OVERLAPPED 用于异步 I/O，以支持安全的资源取消和避免竞态
		overlapped := windows.Overlapped{}
		hevent, err := windows.CreateEvent(nil, 0, 0, nil)
		if err != nil {
			w.logger.Error("failed to create watcher event", zap.Error(err))
			yield(nil, err)
			return
		}
		defer windows.CloseHandle(hevent)
		overlapped.HEvent = hevent

		stopChan := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				// 取消挂起的异步 IO 请求
				windows.CancelIoEx(handle, &overlapped)
			case <-stopChan:
			}
		}()

		w.logger.Info("started watching directory recursively on windows", zap.String("dir", dir))
		defer func() {
			w.logger.Info("stopped watching directory", zap.String("dir", dir))
			close(stopChan)
			wg.Wait()
		}()

		for {
			if ctx.Err() != nil {
				return
			}

			// 异步调用 ReadDirectoryChanges，传入 overlapped 结构
			err := windows.ReadDirectoryChanges(
				handle,
				bufPtr,
				bufLen,
				true, // watchSubTree = true
				windows.FILE_NOTIFY_CHANGE_FILE_NAME|
					windows.FILE_NOTIFY_CHANGE_DIR_NAME|
					windows.FILE_NOTIFY_CHANGE_ATTRIBUTES|
					windows.FILE_NOTIFY_CHANGE_SIZE|
					windows.FILE_NOTIFY_CHANGE_LAST_WRITE,
				nil, // 异步操作时，lpBytesReturned 必须为 nil
				&overlapped,
				0,
			)

			if err != nil && err != windows.ERROR_IO_PENDING {
				if ctx.Err() != nil {
					return
				}
				w.logger.Error("failed to initiate ReadDirectoryChanges", zap.Error(err))
				if !yield(nil, err) {
					return
				}
				return
			}

			// 等待异步 I/O 完成
			state, err := windows.WaitForSingleObject(hevent, windows.INFINITE)
			if err != nil {
				w.logger.Error("failed to wait for watcher event", zap.Error(err))
				if !yield(nil, err) {
					return
				}
				return
			}

			if state != windows.WAIT_OBJECT_0 {
				w.logger.Error("unexpected wait state in watcher", zap.Uint32("state", state))
				return
			}

			var ret uint32
			err = windows.GetOverlappedResult(handle, &overlapped, &ret, true)
			if err != nil {
				if err == windows.ERROR_OPERATION_ABORTED || ctx.Err() != nil {
					return
				}
				// 处理缓冲区溢出错误，避免直接退出
				if err == windows.ERROR_MORE_DATA || err == windows.ERROR_INSUFFICIENT_BUFFER {
					w.logger.Warn("watcher buffer overflow, some events might be lost", zap.Error(err))
					continue
				}
				w.logger.Error("watcher overlapped error", zap.Error(err))
				if !yield(nil, err) {
					return
				}
				return
			}

			if ret == 0 {
				// 处理空事件/缓冲区溢出情况
				w.logger.Warn("watcher buffer overflow or empty event, some events might be lost")
				continue
			}

			offset := uint32(0)
			for {
				if offset+12 > ret {
					break // 数据不足以解析 FILE_NOTIFY_INFORMATION 头部
				}
				info := (*windows.FileNotifyInformation)(unsafe.Add(unsafe.Pointer(bufPtr), offset))
				nameLen := info.FileNameLength / 2
				if offset+12+info.FileNameLength > ret {
					break // 防止越界
				}

				namePtr := (*[0xffff]uint16)(unsafe.Pointer(&info.FileName))[:nameLen:nameLen]
				name := windows.UTF16ToString(namePtr)

				// 忽略临时文件 (包含 .tmp 和以 ~ 开头的临时文件)
				baseName := filepath.Base(name)
				if !strings.HasSuffix(name, ".tmp") && !strings.HasPrefix(baseName, "~") {
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

					var absPath string
					if filepath.IsAbs(name) {
						absPath = filepath.Clean(name)
					} else {
						absPath = filepath.Join(dir, name)
					}
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
