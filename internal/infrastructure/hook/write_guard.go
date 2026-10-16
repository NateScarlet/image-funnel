package hook

import (
	"os"
	"sync"
	"time"

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

// Write 写入文件前登记内容哈希，使由这次写入触发的文件变更事件被识别为自身回环
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

	return os.WriteFile(absPath, content, perm)
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
