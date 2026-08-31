//go:build windows

package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard           = user32.NewProc("OpenClipboard")
	closeClipboard          = user32.NewProc("CloseClipboard")
	getClipboardData        = user32.NewProc("GetClipboardData")
	registerClipboardFormat = user32.NewProc("RegisterClipboardFormatW")
	enumClipboardFormats    = user32.NewProc("EnumClipboardFormats")
	getClipboardFormatNameW = user32.NewProc("GetClipboardFormatNameW")
	globalAlloc             = kernel32.NewProc("GlobalAlloc")
	globalLock              = kernel32.NewProc("GlobalLock")
	globalUnlock            = kernel32.NewProc("GlobalUnlock")
	globalSize              = kernel32.NewProc("GlobalSize")
)

const (
	cfDropFiles   = 15
	cfText        = 1
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
	gmemZeroinit  = 0x0040

	// clipboardWriteRetryTimes / clipboardWriteRetryDelayMs SetDataObject 四参重载的写入重试参数：
	// 剪贴板是系统全局共享资源，可能被其他进程瞬时占用导致写入失败，按此参数重试后仍失败才报错
	clipboardWriteRetryTimes   = 10
	clipboardWriteRetryDelayMs = 100
)

// Clipboard Windows 剪贴板实现
type Clipboard struct {
	// mu 串行化本进程内的剪贴板写入，避免并发请求各自启动 PowerShell 进程互相抢占剪贴板
	mu sync.Mutex
}

// NewClipboard 创建一个新的剪贴板实例
func NewClipboard() *Clipboard {
	return &Clipboard{}
}

// openClipboardWithRetry 尝试打开剪贴板，失败时重试（浏览器可能短暂占用剪贴板）
func openClipboardWithRetry() bool {
	for i := 0; i < 10; i++ {
		ret, _, _ := openClipboard.Call(0)
		if ret != 0 {
			return true
		}
		if i < 9 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return false
}

// ReadCustomFormat 读取剪贴板中自定义格式的数据
func (c *Clipboard) ReadCustomFormat(formatName string) (string, error) {
	format, err := registerClipboardFormatW(formatName)
	if err != nil {
		return "", err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if !openClipboardWithRetry() {
		return "", fmt.Errorf("OpenClipboard failed")
	}
	defer closeClipboard.Call()

	hData, _, _ := getClipboardData.Call(uintptr(format))
	if hData == 0 {
		return "", nil
	}

	pData, _, _ := globalLock.Call(hData)
	if pData == 0 {
		return "", fmt.Errorf("GlobalLock failed")
	}
	defer globalUnlock.Call(hData)

	size, _, _ := globalSize.Call(hData)
	if size == 0 {
		return "", nil
	}

	buf := make([]byte, size)
	copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(pData)), size))

	// 假设以 null 结尾
	if len(buf) > 0 && buf[len(buf)-1] == 0 {
		buf = buf[:len(buf)-1]
	}

	return string(buf), nil
}

// ReadHTMLFormat 读取剪贴板中 HTML Format (CF_HTML) 的内容
func (c *Clipboard) ReadHTMLFormat() (string, error) {
	raw, err := c.ReadCustomFormat("HTML Format")
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", nil
	}

	// 解析 CF_HTML 头部，提取 StartHTML/EndHTML 偏移量
	startHTML := findCFHTMLOffset(raw, "StartHTML:")
	endHTML := findCFHTMLOffset(raw, "EndHTML:")
	if startHTML < 0 || endHTML < 0 || startHTML >= endHTML || endHTML > len(raw) {
		return "", fmt.Errorf("invalid CF_HTML header")
	}

	return raw[startHTML:endHTML], nil
}

// findCFHTMLOffset 在 CF_HTML 头部中查找指定键的偏移量值
func findCFHTMLOffset(raw, key string) int {
	idx := strings.Index(raw, key)
	if idx < 0 {
		return -1
	}
	rest := raw[idx+len(key):]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	var offset int
	for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		offset = offset*10 + int(rest[0]-'0')
		rest = rest[1:]
	}
	return offset
}

// ListFormats 枚举剪贴板中所有格式名称，用于调试
func (c *Clipboard) ListFormats() []string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if !openClipboardWithRetry() {
		return nil
	}
	defer closeClipboard.Call()

	var formats []string
	format, _, _ := enumClipboardFormats.Call(0)
	for format != 0 {
		name := formatName(uint32(format))
		formats = append(formats, name)
		format, _, _ = enumClipboardFormats.Call(format)
	}
	return formats
}

// formatName 根据格式 ID 获取名称
func formatName(id uint32) string {
	switch id {
	case 1:
		return "CF_TEXT"
	case 2:
		return "CF_BITMAP"
	case 7:
		return "CF_OEMTEXT"
	case 8:
		return "CF_DIB"
	case 13:
		return "CF_UNICODETEXT"
	case 15:
		return "CF_HDROP"
	}
	buf := make([]uint16, 256)
	ret, _, _ := getClipboardFormatNameW.Call(uintptr(id), uintptr(unsafe.Pointer(&buf[0])), 256)
	if ret > 0 {
		return windows.UTF16ToString(buf[:ret])
	}
	return fmt.Sprintf("CF_#%d", id)
}

// AddFiles 通过 PowerShell 调用 .NET Clipboard API 向剪贴板附加文件
func (c *Clipboard) AddFiles(filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	// 串行化写入：剪贴板是全局共享资源，本进程内并发写入会互相抢占而导致偶发失败
	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", buildAddFilesScript(filePaths))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("剪贴板操作失败: %w\n%s", err, string(output))
	}
	return nil
}

// buildAddFilesScript 构建向剪贴板附加文件列表的 PowerShell 脚本：
// 1. 获取当前剪贴板 DataObject，逐个格式复制到新 DataObject（保留所有已有格式）
// 2. 附加文件路径
// 3. 通过四参重载 SetDataObject 写回，剪贴板被其他进程瞬时占用时自动重试
func buildAddFilesScript(filePaths []string) string {
	// 转义路径中的单引号（PowerShell 单引号字符串中唯一需转义的字符）
	var addCmds strings.Builder
	for _, p := range filePaths {
		addCmds.WriteString("$files.Add('")
		addCmds.WriteString(strings.ReplaceAll(p, "'", "''"))
		addCmds.WriteString("');")
	}

	// 注意：不能用 New-Object DataObject($orig) 复制格式——这会把原 DataObject 整体当作一个对象存储，
	// 导致 CF_HDROP 等格式丢失。必须逐个格式取出数据再 SetData 到新 DataObject。
	// SetDataObject 第二个参数 $true 表示复制数据（而非引用），确保剪贴板格式可被 Win32 API 枚举；
	// 第三个/第四个参数为重试次数与重试间隔（毫秒），应对剪贴板被其他进程瞬时占用，
	// 重试耗尽后仍失败则由 catch 向上抛出错误
	return `$ErrorActionPreference = 'Stop';` +
		`try {` +
		`Add-Type -AssemblyName System.Windows.Forms;` +
		`try{$orig=[System.Windows.Forms.Clipboard]::GetDataObject()}catch{$orig=$null};` +
		`$new=New-Object System.Windows.Forms.DataObject;` +
		`if($orig){foreach($fmt in $orig.GetFormats()){$data=$orig.GetData($fmt);if($data){$new.SetData($fmt,$data)}}};` +
		`$files=New-Object System.Collections.Specialized.StringCollection;` +
		addCmds.String() +
		`$new.SetFileDropList($files);` +
		`[System.Windows.Forms.Clipboard]::SetDataObject($new,$true,` +
		fmt.Sprintf("%d,%d", clipboardWriteRetryTimes, clipboardWriteRetryDelayMs) + `)` +
		`} catch {` +
		`Write-Error $_.Exception.Message;` +
		`exit 1;` +
		`}`
}

// registerClipboardFormatW 注册自定义剪贴板格式
func registerClipboardFormatW(name string) (uint32, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	ret, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(namePtr)))
	if ret == 0 {
		return 0, fmt.Errorf("RegisterClipboardFormatW failed for %q", name)
	}
	return uint32(ret), nil
}
