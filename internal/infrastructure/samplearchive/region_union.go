package samplearchive

import "sync"

// imageRect 是原图绝对坐标系下的矩形。W/H 为零表示无差异区域。
type imageRect struct {
	X, Y, W, H int
}

// RegionUnion 保存单次 Commit 内的差异区域并集，供后续候选复用坐标并提前终止 diff。
// 每次 Commit 新建、初始为空，不跨 Commit 共享。
type RegionUnion struct {
	mu   sync.Mutex
	rect imageRect
}

func NewRegionUnion() *RegionUnion {
	return &RegionUnion{}
}

func (u *RegionUnion) Rect() imageRect {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.rect
}

// Contains 判断 rect 是否被并集完全包含（四条边都在并集内）。
// 空并集不包含任何矩形，保证每个候选至少执行一次真实 diff。
func (u *RegionUnion) Contains(rect imageRect) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.rect.W <= 0 || u.rect.H <= 0 || rect.W <= 0 || rect.H <= 0 {
		return false
	}
	return rect.X >= u.rect.X && rect.Y >= u.rect.Y &&
		rect.X+rect.W <= u.rect.X+u.rect.W &&
		rect.Y+rect.H <= u.rect.Y+u.rect.H
}

func (u *RegionUnion) Expand(rect imageRect) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.rect = unionRect(u.rect, rect)
}

func unionRect(a, b imageRect) imageRect {
	if a.W <= 0 || a.H <= 0 {
		return b
	}
	if b.W <= 0 || b.H <= 0 {
		return a
	}
	x1 := min(a.X, b.X)
	y1 := min(a.Y, b.Y)
	x2 := max(a.X+a.W, b.X+b.W)
	y2 := max(a.Y+a.H, b.Y+b.H)
	return imageRect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
}
