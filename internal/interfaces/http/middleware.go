package http

import (
	"net/url"
	"strings"
)

// isDevOriginAllowed 让开发模式下前端 dev server（localhost 各端口）能直接向
// 自托管 envelope 端点上报；生产模式不启用此例外。
func isDevOriginAllowed(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func isOriginAllowed(origin string, requestHost string, corsHosts []string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if requestHost != "" && strings.EqualFold(u.Host, requestHost) {
		return true
	}
	for _, i := range corsHosts {
		if i != "" && strings.EqualFold(i, u.Host) {
			return true
		}
	}
	return false
}
