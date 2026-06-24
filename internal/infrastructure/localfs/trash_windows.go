//go:build windows

package localfs

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shFileOpStructW 对应 Windows API 的 SHFILEOPSTRUCTW 结构
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted uint32 // 改为 uint32 以对应 Win32 BOOL，并保证 64 位对齐
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete          = 3
	fofAllowUndo      = 0x0040
	fofNoConfirmation = 0x0010
	fofSilent         = 0x0004
	fofNoErrorUI      = 0x0400
)

// shQueryRBInfo 对应 Windows API 的 SHQUERYRBINFO 结构
type shQueryRBInfo struct {
	cbSize      uint32
	i64Size     int64
	i64NumItems int64
}

// 定义全局的 API 调用函数指针，便于在单元测试中对其进行 Mock 验证
// 既能避免测试环境（无 GUI 消息循环）下调用 SHFileOperationW 导致跨线程挂起/超时的问题，
// 又能确保结构体传递、对齐以及 flags 参数的装配逻辑得到充分且一致的测试覆盖。
var shFileOperationAPI = func(fileOp *shFileOpStructW) error {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shFileOperation := shell32.NewProc("SHFileOperationW")
	ret, _, _ := shFileOperation.Call(uintptr(unsafe.Pointer(fileOp)))
	if ret != 0 {
		return fmt.Errorf("SHFileOperationW failed with return code %d", ret)
	}
	return nil
}

// 定义全局 of SHQueryRecycleBinW 调用函数指针，用于动态查询对应卷的回收站支持情况，便于测试 Mock。
var shQueryRecycleBinAPI = func(rootPath *uint16, info *shQueryRBInfo) uintptr {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shQueryRecycleBin := shell32.NewProc("SHQueryRecycleBinW")
	ret, _, _ := shQueryRecycleBin.Call(
		uintptr(unsafe.Pointer(rootPath)),
		uintptr(unsafe.Pointer(info)),
	)
	return ret
}

// checkRecycleBinSupport 检查一组路径所在的磁盘卷是否全部支持回收站。
// 如果任意路径不支持回收站，返回一个说明错误。
func checkRecycleBinSupport(paths []string) error {
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			absPath = p
		}
		cleanPath := filepath.Clean(absPath)
		vol := filepath.VolumeName(cleanPath)
		if vol == "" {
			return fmt.Errorf("failed to detect volume name for path: %s", p)
		}
		// 拼接成卷的根目录，形如 "C:\" 或 "\\server\share\"
		var rootPath string
		if len(vol) > 0 && vol[len(vol)-1] == '\\' {
			rootPath = vol
		} else {
			rootPath = vol + "\\"
		}

		u16, err := windows.UTF16PtrFromString(rootPath)
		if err != nil {
			return err
		}

		info := shQueryRBInfo{
			cbSize: uint32(unsafe.Sizeof(shQueryRBInfo{})),
		}

		ret := shQueryRecycleBinAPI(u16, &info)
		// 如果 ret != 0 (S_OK)，表示获取回收站信息失败，说明不支持回收站或不是有效驱动器
		if ret != 0 {
			return fmt.Errorf("the volume %s for path %s does not support system recycle bin (or is invalid). please set IMAGE_FUNNEL_USE_SYSTEM_RECYCLE_BIN=false to enable direct physical deletion", vol, p)
		}
	}
	return nil
}

// trashOrDelete 将一系列物理路径对应的文件移入 Windows 系统回收站。
// 如果 useSystemRecycleBin 为 false，则退化为直接删除物理文件。
// 如果 useSystemRecycleBin 为 true，但文件所在盘符不支持回收站，则返回错误。
func trashOrDelete(paths []string, useSystemRecycleBin bool) error {
	if len(paths) == 0 {
		return nil
	}

	if !useSystemRecycleBin {
		// 物理删除直接使用 Go 语言自带的跨平台 os.RemoveAll，更轻量且彻底避免 Windows Shell API 的副作用
		for _, p := range paths {
			if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}

	// 校验回收站支持
	if err := checkRecycleBinSupport(paths); err != nil {
		return err
	}

	// PFrom 必须是以双 NULL (即 \x00\x00) 结尾的多个 NULL 分隔路径
	var utf16Paths []uint16
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			absPath = p
		}
		cleanPath := filepath.Clean(absPath)
		u16, err := windows.UTF16FromString(cleanPath)
		if err != nil {
			return err
		}
		utf16Paths = append(utf16Paths, u16...)
	}
	utf16Paths = append(utf16Paths, 0) // 添加第二个 null 终止符

	fileOp := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &utf16Paths[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}

	if err := shFileOperationAPI(&fileOp); err != nil {
		return err
	}
	if fileOp.fAnyOperationsAborted != 0 {
		return fmt.Errorf("SHFileOperationW was aborted by user")
	}

	return nil
}
