//go:build windows

package localfs

import (
	"fmt"
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
	fAnyOperationsAborted bool
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

// moveToRecycleBin 将一系列物理路径对应的文件移入 Windows 系统回收站
func moveToRecycleBin(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shFileOperation := shell32.NewProc("SHFileOperationW")

	// PFrom 必须是以双 NULL (即 \x00\x00) 结尾的多个 NULL 分隔路径
	var utf16Paths []uint16
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			absPath = p
		}
		u16, err := windows.UTF16FromString(absPath)
		if err != nil {
			return err
		}
		// UTF16FromString 返回的切片已包含单 null 终止符
		utf16Paths = append(utf16Paths, u16...)
	}
	utf16Paths = append(utf16Paths, 0) // 添加第二个 null 终止符

	fileOp := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &utf16Paths[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}

	ret, _, _ := shFileOperation.Call(uintptr(unsafe.Pointer(&fileOp)))
	if ret != 0 {
		return fmt.Errorf("SHFileOperationW failed with return code %d", ret)
	}
	if fileOp.fAnyOperationsAborted {
		return fmt.Errorf("SHFileOperationW was aborted by user")
	}

	return nil
}
