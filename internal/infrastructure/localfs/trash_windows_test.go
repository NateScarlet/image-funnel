//go:build windows

package localfs

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// 在测试中 Mock 掉不稳定的 Windows 外部回收站系统 API
	// 避免在无 GUI 测试环境中引发 COM apartment 挂起或临时目录无法回收产生的弹窗阻塞
	// 同时对参数传递、结构体装配与 flags 选项进行严谨的断言测试
	shFileOperationAPI = func(fileOp *shFileOpStructW) error {
		if fileOp.wFunc != foDelete {
			panic("invalid wFunc passed to SHFileOperationW mock")
		}
		if fileOp.pFrom == nil {
			panic("nil pFrom passed to SHFileOperationW mock")
		}
		hasUndo := (fileOp.fFlags & fofAllowUndo) != 0
		if hasUndo != expectedAllowUndo {
			panic("mismatch fofAllowUndo flag in SHFileOperationW mock")
		}
		expectedBaseFlags := uint16(fofNoConfirmation | fofSilent | fofNoErrorUI)
		if (fileOp.fFlags & expectedBaseFlags) != expectedBaseFlags {
			panic("invalid base flags passed to SHFileOperationW mock")
		}
		fileOp.fAnyOperationsAborted = 0
		return nil
	}

	shQueryRecycleBinAPI = func(rootPath *uint16, info *shQueryRBInfo) uintptr {
		if mockRecycleBinSupported {
			return 0 // S_OK, 表示支持回收站
		}
		return 0x80070002 // 模拟错误代码，表示不支持回收站
	}

	os.Exit(m.Run())
}
