package hook

import (
	"os"
	"sync"
	"time"

	"main/internal/util"

	"github.com/cespare/xxhash/v2"
)

// writeGuard 防重入守卫：抑制「自身写入 → 文件变更事件回环」。
// 改写侧（写）与事件侧（读）共享同一个守卫实例——只有同一个实例才能识别出
// 事件对应的正是自己刚写下的内容。
type writeGuard struct {
	mu      sync.Mutex
	entries map[string]writeIgnoreItem
}

// writeIgnoreItem 一次自身写入的登记，带过期时间
type writeIgnoreItem struct {
	contentHash uint64
	expireTime  time.Time
}

// writeIgnoreTTL 自身写入登记的有效期：需覆盖文件系统事件上报的延迟窗口
const writeIgnoreTTL = 10 * time.Second

func newWriteGuard() *writeGuard {
	return &writeGuard{entries: make(map[string]writeIgnoreItem)}
}

// Write 登记内容哈希后再原子落位写入。
// 登记使由这次写入触发的变更事件被识别为自身回环：原子落位最终发生在目标路径上，
// 其内容与登记哈希一致而被抑制。落位过程还会产生另外两个路径的变更事件——
// 临时文件（带 .tmp 后缀）与备份硬链接（目标名加 ~），前者已被文件监听层过滤，
// 后者扩展名不再是 .md 而被笔记变更路径忽略，因此都不会重新进入本守卫。
// 原子落位同时保证写入期间同一路径的并发读者只会读到旧全文或新全文。
func (g *writeGuard) Write(absPath string, content []byte, perm os.FileMode) error {
	g.mu.Lock()
	g.entries[absPath] = writeIgnoreItem{
		contentHash: xxhash.Sum64(content),
		expireTime:  time.Now().Add(writeIgnoreTTL),
	}
	now := time.Now()
	// 轻量清理过期的登记项
	for path, item := range g.entries {
		if now.After(item.expireTime) {
			delete(g.entries, path)
		}
	}
	g.mu.Unlock()

	return util.AtomicSave(absPath, func(file *os.File) error {
		_, err := file.Write(content)
		return err
	}, util.AtomicSaveWithPerm(perm))
}

// ShouldIgnore 事件侧查询：内容与最近一次自身写入完全一致则忽略该事件
func (g *writeGuard) ShouldIgnore(absPath string, content []byte) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	item, exists := g.entries[absPath]
	if !exists {
		return false
	}
	if time.Now().After(item.expireTime) {
		delete(g.entries, absPath)
		return false
	}

	return item.contentHash == xxhash.Sum64(content)
}
