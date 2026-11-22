// Package imageversion 集中定义「图片版本」：一次性读取文件元数据后，
// 由这里唯一决定用哪些字段、什么精度判定同路径文件是否仍是同一版本。
// 签名 URL 的身份核对与变体缓存 key 都从此处派生，避免各自 stat 取不同字段/精度而漂移。
package imageversion

import "time"

// Meta 是构造图片版本所需的最小文件元数据视图，os.Stat 返回的 os.FileInfo 满足此接口。
// 只暴露这两个方法，调用方不得自行从更丰富的元数据里挑字段
type Meta interface {
	ModTime() time.Time
	Size() int64
}

// Version 由一次 stat 的元数据构造，对外只提供秒级与纳秒级两种身份视图。
// 字段私有：外部无法自行拼装或改写精度
type Version struct {
	modTime time.Time
	size    int64
}

// New 从一次 stat 得到的文件元数据构造图片版本
func New(meta Meta) Version {
	return Version{modTime: meta.ModTime(), size: meta.Size()}
}

// SecondView 秒级身份视图：秒级修改时间 + 字节大小。
// 签名 URL 的 t/s 声明与校验使用此视图（ADR 0004）
type SecondView struct {
	ModUnix int64
	Size    int64
}

// NanoView 纳秒级身份视图：纳秒级修改时间 + 字节大小。
// 变体缓存 key 使用此视图
type NanoView struct {
	ModNano int64
	Size    int64
}

// Second 返回秒级+大小视图
func (v Version) Second() SecondView {
	return SecondView{ModUnix: v.modTime.Unix(), Size: v.size}
}

// Nano 返回纳秒级+大小视图
func (v Version) Nano() NanoView {
	return NanoView{ModNano: v.modTime.UnixNano(), Size: v.size}
}
