package transcode

import "io"

// File 是可重复打开的转码/缓存结果句柄。它本身不是可读流，
// 必须调用 Open() 获得独立的 io.ReadSeekCloser 进行读取。
type File interface {
	// Open 返回一个独立的读取器（从文件开头开始）。
	// 每个调用者获得的 Reader 有自己的偏移量，可以安全并发使用。
	// 调用方负责 Close 返回的 Reader。
	Open() (io.ReadSeekCloser, error)
}
