//go:build !windows

package clipboard

import "errors"

// Clipboard 非 Windows 平台的剪贴板实现
type Clipboard struct{}

// NewClipboard 创建一个新的剪贴板实例
func NewClipboard() *Clipboard {
	return &Clipboard{}
}

// ReadCustomFormat 读取剪贴板中自定义格式的数据（非 Windows 平台不支持）
func (c *Clipboard) ReadCustomFormat(formatName string) (string, error) {
	return "", errors.New("not supported on this platform")
}

// AddFiles 向剪贴板附加文件（非 Windows 平台不支持）
func (c *Clipboard) AddFiles(filePaths []string) error {
	return errors.New("not supported on this platform")
}

// ListFormats 枚举剪贴板中所有格式（非 Windows 平台不支持）
func (c *Clipboard) ListFormats() []string {
	return nil
}

// ReadHTMLFormat 读取剪贴板中 HTML 格式的内容（非 Windows 平台不支持）
func (c *Clipboard) ReadHTMLFormat() (string, error) {
	return "", errors.New("not supported on this platform")
}
