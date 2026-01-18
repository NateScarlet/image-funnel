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
	preset    string
}

func (d *XMPData) Rating() (_ int) {
	if d == nil {
		return
	}
	return d.rating
}

func (d *XMPData) Action() (_ string) {
	if d == nil {
		return
	}
	return d.action
}

func (d *XMPData) Timestamp() (_ time.Time) {
	if d == nil {
		return
	}
	return d.timestamp
}

func (d *XMPData) Preset() (_ string) {
	if d == nil {
		return
	}
	return d.preset
}

func NewXMPData(rating int, action string, timestamp time.Time, preset string) *XMPData {
	return &XMPData{
		rating:    rating,
		action:    action,
		timestamp: timestamp,
		preset:    preset,
	}
}

func IsSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}
