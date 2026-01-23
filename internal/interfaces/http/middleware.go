package http

import (
	"net/url"
	"strings"
)

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
