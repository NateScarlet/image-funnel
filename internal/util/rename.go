package util

import (
	"time"
)

// RenameWithRetry 重命名文件，遇到 Windows 瞬时占用错误时按退避间隔重试。
//
// 动机：快照写盘（覆盖既有笔记）与附件 CAS 落盘都靠 rename 完成，而目标文件可能正被
// 外部程序短暂持有——目录监视器对新文件做 stat、杀毒软件扫描刚写出的字节等。这类占用
// 在毫秒级释放，直接失败会让整条采集记录白白重试一轮。
//
// 只对「等待后可能自行消失」的错误码重试（见 isTransientRenameError）；其余错误
// （磁盘满、路径不存在、只读介质等）立即上抛，不把可诊断的失败拖成延迟失败。
// 持久占用（对方长期持有句柄）同样会在耗尽尝试后如实报错。
func RenameWithRetry(oldPath, newPath string, options ...RenameOption) error {
	return renameWithRetry(oldPath, newPath, renameFile, options...)
}

// renameWithRetry 是 RenameWithRetry 的实现，rename 作为参数注入以便测试模拟占用，
// 生产调用方一律经 RenameWithRetry（它固定注入平台实现）。
func renameWithRetry(oldPath, newPath string, rename func(oldPath, newPath string) error, options ...RenameOption) error {
	opts := newRenameOptions(options...)
	var err error
	for attempt := 0; ; attempt++ {
		err = rename(oldPath, newPath)
		if err == nil {
			return nil
		}
		if !isTransientRenameError(err) {
			// 非瞬时错误：重试无意义，保持快速失败
			return err
		}
		if attempt >= opts.maxRetries {
			return err
		}
		// 按退避表等待；表用尽后继续用最后一项间隔，空表则立即重试
		var delay time.Duration
		if len(opts.backoff) > 0 {
			delay = opts.backoff[len(opts.backoff)-1]
			if attempt < len(opts.backoff) {
				delay = opts.backoff[attempt]
			}
		}
		time.Sleep(delay)
	}
}

// newRenameOptions 构造重命名重试配置并套用可选覆盖。
// 默认 3 次重试（共 4 次尝试）配 20/50/100ms 递增退避：总时长上限约 170ms，
// 足以跨过目录监视器与杀毒扫描的占用窗口，又不至于让真正的持久占用长时间挂起。
func newRenameOptions(options ...RenameOption) *RenameOptions {
	opts := &RenameOptions{
		maxRetries: 3,
		backoff:    []time.Duration{20 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond},
	}
	for _, apply := range options {
		apply(opts)
	}
	return opts
}

// RenameOptions 重命名重试的可选配置（字段私有，经函数式 Options 覆盖）。
type RenameOptions struct {
	maxRetries int
	backoff    []time.Duration
}

// RenameOption 重命名重试的可选配置项。
type RenameOption func(*RenameOptions)

// WithRenameMaxRetries 设置最大重试次数（不含首次尝试）；0 表示不重试。
func WithRenameMaxRetries(n int) RenameOption {
	return func(opts *RenameOptions) { opts.maxRetries = n }
}

// WithRenameBackoff 设置各次重试前的等待间隔；空切片表示立即重试。
func WithRenameBackoff(backoff []time.Duration) RenameOption {
	return func(opts *RenameOptions) { opts.backoff = backoff }
}
