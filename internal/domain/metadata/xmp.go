package metadata

import (
	"path/filepath"
	"strings"
	"time"
)

type XMPData struct {
	rating    int
	action    string
	timestamp time.Time
	label     string // XMP 标签（如颜色标签或自定义文本标签）
}

// Rating 返回 XMP 评分星级
func (d *XMPData) Rating() (_ int) {
	if d == nil {
		return
	}
	return d.rating
}

// Action 返回筛选操作动作
func (d *XMPData) Action() (_ string) {
	if d == nil {
		return
	}
	return d.action
}

// Timestamp 返回动作时间戳
func (d *XMPData) Timestamp() (_ time.Time) {
	if d == nil {
		return
	}
	return d.timestamp
}

// Label 返回 XMP 标签文本
func (d *XMPData) Label() (_ string) {
	if d == nil {
		return
	}
	return d.label
}

// NewXMPData 创建一个新的 XMP 伴随元数据对象
func NewXMPData(rating int, action string, timestamp time.Time, label string) *XMPData {
	return &XMPData{
		rating:    rating,
		action:    action,
		timestamp: timestamp,
		label:     label,
	}
}

// IsSupportedImage 判断文件名是否为支持的图片格式
func IsSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}
