//go:build !windows

package clipboard

import "errors"

// Clipboard 非 Windows 平台的剪贴板实现
type Clipboard struct{}

// NewClipboard 创建一个新的剪贴板实例
func NewClipboard() *Clipboard {
	return &Clipboard{}
}

// AddFiles 向剪贴板附加文件（非 Windows 平台不支持）
func (c *Clipboard) AddFiles(filePaths []string, html string) error {
	return errors.New("not supported on this platform")
}

// ReadHTMLFormat 读取剪贴板中 HTML 格式的内容（非 Windows 平台不支持）
func (c *Clipboard) ReadHTMLFormat() (string, error) {
	return "", errors.New("not supported on this platform")
}
