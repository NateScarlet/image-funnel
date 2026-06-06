package util

import (
	"fmt"

	"github.com/mssola/user_agent"
)

// ParseUserAgent 解析 User-Agent 字符串，返回有意义的友好设备名称
func ParseUserAgent(uaStr string) string {
	if uaStr == "" {
		return "未知设备"
	}

	ua := user_agent.New(uaStr)
	osInfo := ua.OSInfo()
	browserName, _ := ua.Browser()

	osName := osInfo.Name
	if osName == "" {
		osName = "未知系统"
	}

	if browserName != "" {
		return fmt.Sprintf("%s (%s)", osName, browserName)
	}
	return osName
}
