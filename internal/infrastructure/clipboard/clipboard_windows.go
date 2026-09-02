//go:build windows

package clipboard

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard           = user32.NewProc("OpenClipboard")
	closeClipboard          = user32.NewProc("CloseClipboard")
	emptyClipboard          = user32.NewProc("EmptyClipboard")
	getClipboardData        = user32.NewProc("GetClipboardData")
	setClipboardData        = user32.NewProc("SetClipboardData")
	registerClipboardFormat = user32.NewProc("RegisterClipboardFormatW")
	enumClipboardFormats    = user32.NewProc("EnumClipboardFormats")
	getClipboardFormatNameW = user32.NewProc("GetClipboardFormatNameW")
	globalAlloc             = kernel32.NewProc("GlobalAlloc")
	globalLock              = kernel32.NewProc("GlobalLock")
	globalUnlock            = kernel32.NewProc("GlobalUnlock")
	globalFree              = kernel32.NewProc("GlobalFree")
	globalSize              = kernel32.NewProc("GlobalSize")
)

const (
	cfDropFiles      = 15
	cfText           = 1
	cfUnicodeText    = 13
	gmemMoveable     = 0x0002
	gmemZeroinit     = 0x0040
	dropFilesHeadLen = 20 // DROPFILES 结构头长度：pFiles(4)+pt(8)+fNC(4)+fWide(4)

	// clipboardWriteRetryTimes / clipboardWriteRetryDelayMs 写入重试参数：
	// 剪贴板是系统全局共享资源，MuMu 等模拟器进程会高频抢占剪贴板（尤其是 OLE 路径，
	// 表现为 .NET/WPF SetDataObject 频繁 CLIPBRD_E_CANT_OPEN）。本实现绕过 OLE，
	// 直接使用 Win32 原始 API，并对 OpenClipboard 与 EmptyClipboard 两个可失败
	// 步骤分别重试；实测竞争下平均 1 轮成功，30 轮上限留足余量。
	clipboardWriteRetryTimes   = 30
	clipboardWriteRetryDelayMs = 100
)

// Clipboard Windows 剪贴板实现
type Clipboard struct {
	// mu 串行化本进程内的剪贴板写入，避免并发请求各自抢占剪贴板
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

// AddFiles 将文件列表以 CF_HDROP 写入剪贴板，同时写回纯文本与 HTML 格式。
//
// MuMu 等模拟器进程会高频抢占剪贴板，.NET/WPF 的 SetDataObject（OLE 路径）会因此
// 频繁报 CLIPBRD_E_CANT_OPEN；本实现绕过 OLE，直接使用 Win32 原始 API
// （OpenClipboard/EmptyClipboard/SetClipboardData），并对 OpenClipboard 与
// EmptyClipboard 两个可失败步骤分别重试。所有调用必须在同一 OS 线程上执行。
func (c *Clipboard) AddFiles(filePaths []string, html string) error {
	if len(filePaths) == 0 {
		return nil
	}

	// 串行化写入：剪贴板是全局共享资源，本进程内并发写入会互相抢占而导致偶发失败
	c.mu.Lock()
	defer c.mu.Unlock()

	// 剪贴板 API 要求 Open/Close 在同一 OS 线程上配对调用
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 与前端 text/plain 内容保持一致：绝对路径每行一个
	text := strings.Join(filePaths, "\r\n")

	// 预构建各格式负载（含 App 写入时带 nonce 的 HTML；html 为空则不写 HTML 格式）
	textBlob := utf16Bytes(text + "\x00") // CF_UNICODETEXT：UTF-16LE + 结尾 NULL
	dropBlob := buildDropFiles(filePaths)

	var htmlFormat uint32
	var htmlBlob []byte
	if html != "" {
		format, err := registerClipboardFormatW("HTML Format")
		if err != nil {
			return err
		}
		htmlFormat = format
		htmlBlob = buildCFHTML(html)
	}

	var lastErr error
	for attempt := 0; attempt < clipboardWriteRetryTimes; attempt++ {
		// 1. 打开剪贴板；失败说明被其他进程占用，等待后重试
		ret, _, _ := openClipboard.Call(0)
		if ret == 0 {
			lastErr = fmt.Errorf("OpenClipboard failed")
			time.Sleep(clipboardWriteRetryDelayMs * time.Millisecond)
			continue
		}

		// 2. 清空旧数据；失败通常因前一个拥有者的延迟渲染未及时响应，
		//    先关闭本次打开再等待后重试（否则自己占着锁永远重试不出去）
		ret, _, _ = emptyClipboard.Call()
		if ret == 0 {
			closeClipboard.Call()
			lastErr = fmt.Errorf("EmptyClipboard failed")
			time.Sleep(clipboardWriteRetryDelayMs * time.Millisecond)
			continue
		}

		// 3. 写入三种格式（SetClipboardData 成功后内存归系统所有，不可释放）。
		//    EmptyClipboard 成功后旧数据已被清空，此处失败属于不可恢复错误，
		//    直接快速失败返回，避免重试留下半空剪贴板
		ok := setClipboardDataFormat(cfUnicodeText, textBlob)
		if ok && html != "" {
			ok = setClipboardDataFormat(htmlFormat, htmlBlob)
		}
		if ok {
			ok = setClipboardDataFormat(cfDropFiles, dropBlob)
		}
		closeClipboard.Call()
		if !ok {
			return fmt.Errorf("SetClipboardData failed")
		}
		return nil
	}
	return lastErr
}

// setClipboardDataFormat 分配全局内存并写入指定剪贴板格式。
// 成功后 HGLOBAL 所有权移交系统，调用方不得释放；失败时负责释放并返回 false。
func setClipboardDataFormat(format uint32, data []byte) bool {
	h, _, _ := globalAlloc.Call(gmemMoveable|gmemZeroinit, uintptr(len(data)))
	if h == 0 {
		return false
	}
	p, _, _ := globalLock.Call(h)
	if p == 0 {
		globalFree.Call(h)
		return false
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	globalUnlock.Call(h)

	if ret, _, _ := setClipboardData.Call(uintptr(format), h); ret == 0 {
		globalFree.Call(h)
		return false
	}
	return true
}

// buildDropFiles 构造 CF_HDROP 剪贴板数据：
// DROPFILES 头（pFiles=20 指向文件列表偏移，fWide=1 表示宽字符路径）+
// UTF-16LE 空终止的文件路径列表，列表末尾再补一个 NULL 形成双 NULL 结尾。
func buildDropFiles(filePaths []string) []byte {
	buf := make([]byte, 0, 64)

	// DROPFILES 头：pFiles=20（路径列表相对结构体起点的偏移），fWide=1
	head := make([]byte, dropFilesHeadLen)
	binary.LittleEndian.PutUint32(head[0:4], dropFilesHeadLen)
	binary.LittleEndian.PutUint32(head[16:20], 1)
	buf = append(buf, head...)

	for _, p := range filePaths {
		buf = append(buf, utf16Bytes(p)...)
		buf = append(buf, 0, 0) // 每个路径 NULL 终止
	}
	buf = append(buf, 0, 0) // 列表结尾额外 NULL，双 NULL 结尾
	return buf
}

// cfHTMLHeaderFmt CF_HTML 头部模板：
// 偏移字段固定为 10 位十进制数字（%010d），保证头部长度恒定，便于计算字节偏移。
const cfHTMLHeaderFmt = "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"

// buildCFHTML 构造 CF_HTML 剪贴板数据：头部 + HTML 内容。
// StartHTML/EndHTML 指向 HTML 内容的起止字节偏移，使读取端可 round-trip 还原；
// Start/EndFragment 与 Start/EndHTML 相同（内容整体即片段）。
func buildCFHTML(html string) []byte {
	headerLen := len(fmt.Sprintf(cfHTMLHeaderFmt, 0, 0, 0, 0))
	startHTML := headerLen
	endHTML := headerLen + len(html)
	header := fmt.Sprintf(cfHTMLHeaderFmt, startHTML, endHTML, startHTML, endHTML)
	blob := make([]byte, 0, headerLen+len(html))
	blob = append(blob, header...)
	blob = append(blob, html...)
	return blob
}

// utf16Bytes 将字符串编码为 UTF-16LE 字节序列（不追加结尾 NULL）
func utf16Bytes(s string) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, 0, len(units)*2)
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
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
