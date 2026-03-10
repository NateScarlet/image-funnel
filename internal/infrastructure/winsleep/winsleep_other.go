//go:build !windows

// Package winsleep 在非 Windows 平台上提供空实现，不做任何操作。
package winsleep

import (
	"time"

	"go.uber.org/zap"
)

// Guard 在非 Windows 平台上是空操作
type Guard struct{}

// NewGuard 创建防休眠守卫（非 Windows 平台为空操作）
// 返回守卫实例和空的停止函数
func NewGuard(_ time.Duration, _ *zap.Logger) (*Guard, func()) {
	return &Guard{}, func() {}
}

// RecordActivity 在非 Windows 平台上不做任何操作
func (g *Guard) RecordActivity() {}
