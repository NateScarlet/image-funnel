//go:build windows

// Package winsleep 在 Windows 上通过 SetThreadExecutionState API 控制系统休眠行为。
// 当收到活跃信号时阻止系统休眠，空闲一段时间后恢复正常策略。
package winsleep

import (
	"runtime"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

// SetThreadExecutionState API 标志位
// ES_CONTINUOUS      保持当前状态直到再次调用并清除该标志
// ES_SYSTEM_REQUIRED 阻止系统进入休眠（不影响屏幕关闭）
const (
	esContinuous     uintptr = 0x80000000
	esSystemRequired uintptr = 0x00000001
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	setThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

// Guard 接收活跃状态更新，在收到状态时阻止系统休眠
type Guard struct {
	idleThreshold time.Duration
	logger        *zap.Logger

	activityChan chan struct{} // 接收活跃信号
	stopChan     chan struct{} // 通知 loop 退出
}

// NewGuard 创建防休眠守卫
// idleThreshold 为多久未收到活跃信号后恢复休眠
// 返回守卫实例和停止函数，停止函数会停止循环并恢复系统默认休眠策略
func NewGuard(idleThreshold time.Duration, logger *zap.Logger) (*Guard, func()) {
	g := &Guard{
		idleThreshold: idleThreshold,
		logger:        logger,
		activityChan:  make(chan struct{}, 1),
		stopChan:      make(chan struct{}),
	}

	go g.loop()

	return g, func() {
		close(g.stopChan)
	}
}

// loop 在固定的 OS 线程上执行 SetThreadExecutionState
// 针对 SetThreadExecutionState 这种具有线程相关性的 API：
// 标志位是针对调用线程设置的，若在不同线程调用 release 无法清除该 API 在先前线程上设置的标志。
func (g *Guard) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var (
		active = false
		timer  = time.NewTimer(g.idleThreshold)
	)
	// Go 1.23+ 保证 Stop 后 C 中不会残留陈旧值，且后续接收会阻塞
	timer.Stop()

	for {
		select {
		case <-g.activityChan:
			if !active {
				g.prevent()
				active = true
				g.logger.Info("preventing system sleep due to recent activity")
			}
			timer.Reset(g.idleThreshold)
		case <-timer.C:
			g.release()
			active = false
			g.logger.Info("allowing system sleep, no recent activity")
		case <-g.stopChan:
			if active {
				g.release()
			}
			timer.Stop()
			return
		}
	}
}

// RecordActivity 记录一次活跃行为，触发或重置空闲计时器
// 调用时阻止休眠，直到超过 idleThreshold 没有再次调用。
// 使用非阻塞发送，因为并发的多次记录在逻辑上是等效的，只需处理最近的一次。
func (g *Guard) RecordActivity() {
	select {
	case g.activityChan <- struct{}{}:
	default:
		// 通道已满（已有待处理信号），忽略本次。
	}
}

// prevent 调用 SetThreadExecutionState 阻止系统休眠
func (g *Guard) prevent() {
	ret, _, _ := setThreadExecutionState.Call(esContinuous | esSystemRequired)
	if ret == 0 {
		g.logger.Warn("SetThreadExecutionState (prevent) returned 0")
	}
}

// release 调用 SetThreadExecutionState 恢复默认休眠策略
func (g *Guard) release() {
	ret, _, _ := setThreadExecutionState.Call(esContinuous)
	if ret == 0 {
		g.logger.Warn("SetThreadExecutionState (release) returned 0")
	}
}
