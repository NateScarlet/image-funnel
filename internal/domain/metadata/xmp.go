package metadata

import (
	"path/filepath"
	"strings"
	"time"
)

type XMPData struct {
	rating    int
	action    string
	sessionID string
	timestamp time.Time
	preset    string
}

func NewXMPData(rating int, action, sessionID string, timestamp time.Time, preset string) *XMPData {
	return &XMPData{
		rating:    rating,
		action:    action,
		sessionID: sessionID,
		timestamp: timestamp,
		preset:    preset,
	}
}

func (x *XMPData) Rating() int {
	return x.rating
}

func (x *XMPData) Action() string {
	return x.action
}

func (x *XMPData) SessionID() string {
	return x.sessionID
}

func (x *XMPData) Timestamp() time.Time {
	return x.timestamp
}

func (x *XMPData) Preset() string {
	return x.preset
}

func IsSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}
